package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/redoubtapp/redoubt-api/internal/db/generated"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
)

const (
	// MaxAttachmentSize is the maximum allowed attachment size (25 MB).
	MaxAttachmentSize = 25 * 1024 * 1024

	// MaxAttachmentsPerMessage is the maximum number of attachments per message.
	MaxAttachmentsPerMessage = 10

	// AttachmentPrefix is the S3 directory prefix for attachments.
	AttachmentPrefix = "attachments"
)

// AllowedAttachmentTypes defines allowed MIME types for attachments.
var AllowedAttachmentTypes = map[string]bool{
	// Images
	"image/png":     true,
	"image/jpeg":    true,
	"image/webp":    true,
	"image/gif":     true,
	"image/svg+xml": true,
	// Documents
	"application/pdf":  true,
	"text/plain":       true,
	"text/markdown":    true,
	"text/csv":         true,
	"application/json": true,
	"application/xml":  true,
	// Archives
	"application/zip":             true,
	"application/gzip":            true,
	"application/x-tar":           true,
	"application/x-7z-compressed": true,
	// Code
	"text/javascript":    true,
	"text/css":           true,
	"text/html":          true,
	"application/x-yaml": true,
	"text/x-python":      true,
	"text/x-go":          true,
	"text/x-rust":        true,
	"text/x-typescript":  true,
}

// AttachmentInfo represents attachment metadata returned to clients.
type AttachmentInfo struct {
	ID          uuid.UUID `json:"id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	URL         string    `json:"url"`
	IsImage     bool      `json:"is_image"`
}

// AttachmentMeta contains attachment metadata including its parent message.
type AttachmentMeta struct {
	ID        uuid.UUID
	MessageID uuid.UUID
	Filename  string
}

// GetAttachmentInfo retrieves attachment metadata without downloading file data.
func (s *Service) GetAttachmentInfo(ctx context.Context, attachmentID uuid.UUID) (*AttachmentMeta, error) {
	attachment, err := s.queries.GetAttachmentByID(ctx, pgtype.UUID{Bytes: attachmentID, Valid: true})
	if err != nil {
		return nil, apperrors.ErrAttachmentNotFound
	}

	return &AttachmentMeta{
		ID:        attachment.ID.Bytes,
		MessageID: attachment.MessageID.Bytes,
		Filename:  attachment.Filename,
	}, nil
}

// UploadAttachmentRequest contains data for uploading an attachment.
type UploadAttachmentRequest struct {
	MessageID uuid.UUID
	UserID    uuid.UUID
	Filename  string
	Data      []byte
	Order     int
}

// UploadAttachment uploads a file attachment for a message.
func (s *Service) UploadAttachment(ctx context.Context, req UploadAttachmentRequest) (*AttachmentInfo, error) {
	// Validate file size
	if len(req.Data) > MaxAttachmentSize {
		return nil, apperrors.ErrFileTooLarge
	}

	if len(req.Data) == 0 {
		return nil, apperrors.ErrInvalidInput
	}

	// Check attachment count
	count, err := s.queries.CountMessageAttachments(ctx, pgtype.UUID{Bytes: req.MessageID, Valid: true})
	if err != nil {
		s.logger.Error("failed to count attachments", slog.String("error", err.Error()))
		return nil, err
	}
	if count >= MaxAttachmentsPerMessage {
		return nil, apperrors.ErrTooManyAttachments
	}

	// Detect and validate content type
	contentType := detectContentType(req.Data, req.Filename)
	if !AllowedAttachmentTypes[contentType] {
		// Allow any content type for now but sanitize
		contentType = "application/octet-stream"
	}

	// Generate encryption key for this file
	fileKey, err := s.encryptor.GenerateFileKey()
	if err != nil {
		s.logger.Error("failed to generate file key", slog.String("error", err.Error()))
		return nil, err
	}

	// Encrypt the file data
	encryptedData, err := s.encryptor.Encrypt(req.Data, fileKey)
	if err != nil {
		s.logger.Error("failed to encrypt file", slog.String("error", err.Error()))
		return nil, err
	}

	// Encrypt the file key with the master key
	encryptedKey, iv, err := s.encryptor.EncryptFileKey(fileKey)
	if err != nil {
		s.logger.Error("failed to encrypt file key", slog.String("error", err.Error()))
		return nil, err
	}

	// Generate unique S3 key
	fileID := uuid.New()
	ext := filepath.Ext(req.Filename)
	s3Key := fmt.Sprintf("%s/%s/%s%s", AttachmentPrefix, req.MessageID.String(), fileID.String(), ext)

	// Upload to S3
	if err := s.s3Client.Upload(ctx, s3Key, bytes.NewReader(encryptedData), "application/octet-stream", int64(len(encryptedData))); err != nil {
		s.logger.Error("failed to upload to S3", slog.String("error", err.Error()))
		return nil, err
	}

	// Store media file record
	mediaFile, err := s.queries.CreateMediaFileWithMessage(ctx, generated.CreateMediaFileWithMessageParams{
		OwnerID:       pgtype.UUID{Bytes: req.UserID, Valid: true},
		MessageID:     pgtype.UUID{Bytes: req.MessageID, Valid: true},
		S3Key:         s3Key,
		EncryptionKey: encryptedKey,
		EncryptionIv:  iv,
		ContentType:   contentType,
		SizeBytes:     int64(len(req.Data)),
	})
	if err != nil {
		// Clean up S3 object
		if delErr := s.s3Client.Delete(ctx, s3Key); delErr != nil {
			s.logger.Error("failed to clean up S3 after DB error", slog.String("error", delErr.Error()))
		}
		s.logger.Error("failed to create media file record", slog.String("error", err.Error()))
		return nil, err
	}

	// Create attachment record
	attachment, err := s.queries.CreateMessageAttachment(ctx, generated.CreateMessageAttachmentParams{
		MessageID:    pgtype.UUID{Bytes: req.MessageID, Valid: true},
		MediaFileID:  mediaFile.ID,
		Filename:     sanitizeFilename(req.Filename),
		DisplayOrder: int32(min(req.Order, math.MaxInt32)), //nolint:gosec // bounded by min
	})
	if err != nil {
		// Clean up media file and S3
		if delErr := s.queries.DeleteMediaFile(ctx, mediaFile.ID); delErr != nil {
			s.logger.Error("failed to clean up media file after attachment error", slog.String("error", delErr.Error()))
		}
		if delErr := s.s3Client.Delete(ctx, s3Key); delErr != nil {
			s.logger.Error("failed to clean up S3 after attachment error", slog.String("error", delErr.Error()))
		}
		s.logger.Error("failed to create attachment record", slog.String("error", err.Error()))
		return nil, err
	}

	return &AttachmentInfo{
		ID:          uuidFromPgtype(attachment.ID),
		Filename:    attachment.Filename,
		ContentType: contentType,
		SizeBytes:   int64(len(req.Data)),
		URL:         fmt.Sprintf("/api/v1/attachments/%s", uuidFromPgtype(attachment.ID).String()),
		IsImage:     isImageType(contentType),
	}, nil
}

// GetAttachment retrieves an attachment's data.
func (s *Service) GetAttachment(ctx context.Context, attachmentID uuid.UUID) ([]byte, string, string, error) {
	// Get attachment with media file info
	mediaFile, err := s.queries.GetMediaFileForAttachment(ctx, pgtype.UUID{Bytes: attachmentID, Valid: true})
	if err != nil {
		return nil, "", "", apperrors.ErrAttachmentNotFound
	}

	// Get attachment for filename
	attachment, err := s.queries.GetAttachmentByID(ctx, pgtype.UUID{Bytes: attachmentID, Valid: true})
	if err != nil {
		return nil, "", "", apperrors.ErrAttachmentNotFound
	}

	// Download from S3
	reader, _, err := s.s3Client.Download(ctx, mediaFile.S3Key)
	if err != nil {
		s.logger.Error("failed to download from S3", slog.String("error", err.Error()))
		return nil, "", "", err
	}
	defer func() { _ = reader.Close() }()

	encryptedData, err := io.ReadAll(reader)
	if err != nil {
		s.logger.Error("failed to read S3 data", slog.String("error", err.Error()))
		return nil, "", "", err
	}

	// Decrypt file key
	fileKey, err := s.encryptor.DecryptFileKey(mediaFile.EncryptionKey, mediaFile.EncryptionIv)
	if err != nil {
		s.logger.Error("failed to decrypt file key", slog.String("error", err.Error()))
		return nil, "", "", err
	}

	// Decrypt file data
	data, err := s.encryptor.Decrypt(encryptedData, fileKey)
	if err != nil {
		s.logger.Error("failed to decrypt file", slog.String("error", err.Error()))
		return nil, "", "", err
	}

	return data, mediaFile.ContentType, attachment.Filename, nil
}

// GetMessageAttachments retrieves all attachments for a message.
func (s *Service) GetMessageAttachments(ctx context.Context, messageID uuid.UUID) ([]AttachmentInfo, error) {
	attachments, err := s.queries.GetMessageAttachments(ctx, pgtype.UUID{Bytes: messageID, Valid: true})
	if err != nil {
		return nil, err
	}

	result := make([]AttachmentInfo, len(attachments))
	for i, a := range attachments {
		result[i] = AttachmentInfo{
			ID:          uuidFromPgtype(a.ID),
			Filename:    a.Filename,
			ContentType: a.ContentType,
			SizeBytes:   a.SizeBytes,
			URL:         fmt.Sprintf("/api/v1/attachments/%s", uuidFromPgtype(a.ID).String()),
			IsImage:     isImageType(a.ContentType),
		}
	}

	return result, nil
}

// DeleteAttachment deletes an attachment.
func (s *Service) DeleteAttachment(ctx context.Context, attachmentID uuid.UUID) error {
	// Get media file info first
	mediaFile, err := s.queries.GetMediaFileForAttachment(ctx, pgtype.UUID{Bytes: attachmentID, Valid: true})
	if err != nil {
		return apperrors.ErrAttachmentNotFound
	}

	// Delete from S3
	if err := s.s3Client.Delete(ctx, mediaFile.S3Key); err != nil {
		s.logger.Error("failed to delete from S3", slog.String("error", err.Error()))
		return err
	}

	// Delete attachment record (cascades to media file cleanup handled separately)
	if err := s.queries.DeleteMessageAttachment(ctx, pgtype.UUID{Bytes: attachmentID, Valid: true}); err != nil {
		s.logger.Error("failed to delete attachment record", slog.String("error", err.Error()))
		return err
	}

	// Delete media file record
	if err := s.queries.DeleteMediaFile(ctx, mediaFile.ID); err != nil {
		s.logger.Error("failed to delete media file record", slog.String("error", err.Error()))
		return err
	}

	return nil
}

// DeleteMessageAttachments deletes all attachments for a message.
func (s *Service) DeleteMessageAttachments(ctx context.Context, messageID uuid.UUID) error {
	attachments, err := s.queries.GetMessageAttachments(ctx, pgtype.UUID{Bytes: messageID, Valid: true})
	if err != nil {
		return err
	}

	for _, a := range attachments {
		// Delete from S3
		if err := s.s3Client.Delete(ctx, a.S3Key); err != nil {
			s.logger.Warn("failed to delete attachment from S3", slog.String("key", a.S3Key), slog.String("error", err.Error()))
		}

		// Delete media file record
		if err := s.queries.DeleteMediaFile(ctx, a.MediaFileID); err != nil {
			s.logger.Warn("failed to delete media file record", slog.String("error", err.Error()))
		}
	}

	// Delete all attachment records
	if err := s.queries.DeleteMessageAttachments(ctx, pgtype.UUID{Bytes: messageID, Valid: true}); err != nil {
		return err
	}

	return nil
}

// isImageType checks if a content type is an image.
func isImageType(contentType string) bool {
	return strings.HasPrefix(contentType, "image/")
}

// sanitizeFilename removes potentially dangerous characters from filenames.
func sanitizeFilename(filename string) string {
	// Get just the base name
	filename = filepath.Base(filename)

	// Remove null bytes and control characters
	filename = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, filename)

	// Limit length
	if len(filename) > 255 {
		ext := filepath.Ext(filename)
		name := strings.TrimSuffix(filename, ext)
		maxNameLen := 255 - len(ext)
		if maxNameLen > 0 && len(name) > maxNameLen {
			name = name[:maxNameLen]
		}
		filename = name + ext
	}

	if filename == "" {
		filename = "attachment"
	}

	return filename
}

// uuidFromPgtype converts a pgtype.UUID to a uuid.UUID.
func uuidFromPgtype(p pgtype.UUID) uuid.UUID {
	if !p.Valid {
		return uuid.Nil
	}
	return p.Bytes
}
