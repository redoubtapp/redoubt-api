package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/redoubtapp/redoubt-api/internal/db/generated"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
)

const (
	// MaxFileSize is the maximum allowed file size (5 MB).
	MaxFileSize = 5 * 1024 * 1024

	// Avatar directory prefix in S3.
	AvatarPrefix = "avatars"
)

// AllowedMIMETypes for avatar uploads.
var AllowedMIMETypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
	"image/gif":  true,
}

// Service handles media file operations.
type Service struct {
	s3Client  *S3Client
	encryptor *Encryptor
	queries   *generated.Queries
	logger    *slog.Logger
}

// NewService creates a new storage service.
func NewService(s3Client *S3Client, encryptor *Encryptor, queries *generated.Queries, logger *slog.Logger) *Service {
	return &Service{
		s3Client:  s3Client,
		encryptor: encryptor,
		queries:   queries,
		logger:    logger,
	}
}

// UploadAvatar uploads an avatar for a user.
func (s *Service) UploadAvatar(ctx context.Context, userID uuid.UUID, data []byte, filename string) (*generated.MediaFile, error) {
	// Validate file size
	if len(data) > MaxFileSize {
		return nil, apperrors.ErrFileTooLarge
	}

	// Detect and validate content type
	contentType := detectContentType(data, filename)
	if !AllowedMIMETypes[contentType] {
		return nil, apperrors.ErrInvalidFileType
	}

	// Generate encryption key for this file
	fileKey, err := s.encryptor.GenerateFileKey()
	if err != nil {
		s.logger.Error("failed to generate file key", slog.String("error", err.Error()))
		return nil, err
	}

	// Encrypt the file data
	encryptedData, err := s.encryptor.Encrypt(data, fileKey)
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

	// Generate S3 key
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = extensionFromMIME(contentType)
	}
	s3Key := fmt.Sprintf("%s/%s%s", AvatarPrefix, userID.String(), ext)

	// Delete existing avatar if any
	existingFiles, err := s.queries.GetMediaFilesByOwner(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err == nil {
		for _, f := range existingFiles {
			// Check if it's an avatar
			if strings.HasPrefix(f.S3Key, AvatarPrefix+"/") {
				// Delete from S3
				if err := s.s3Client.Delete(ctx, f.S3Key); err != nil {
					s.logger.Warn("failed to delete old avatar from S3", slog.String("key", f.S3Key), slog.String("error", err.Error()))
				}
				// Delete from database
				if err := s.queries.DeleteMediaFile(ctx, f.ID); err != nil {
					s.logger.Warn("failed to delete old avatar record", slog.String("error", err.Error()))
				}
			}
		}
	}

	// Upload to S3
	if err := s.s3Client.Upload(ctx, s3Key, bytes.NewReader(encryptedData), "application/octet-stream", int64(len(encryptedData))); err != nil {
		s.logger.Error("failed to upload to S3", slog.String("error", err.Error()))
		return nil, err
	}

	// Store metadata in database
	mediaFile, err := s.queries.CreateMediaFile(ctx, generated.CreateMediaFileParams{
		OwnerID:       pgtype.UUID{Bytes: userID, Valid: true},
		S3Key:         s3Key,
		EncryptionKey: encryptedKey,
		EncryptionIv:  iv,
		ContentType:   contentType,
		SizeBytes:     int64(len(data)),
	})
	if err != nil {
		// Try to clean up S3 object
		if delErr := s.s3Client.Delete(ctx, s3Key); delErr != nil {
			s.logger.Error("failed to clean up S3 after DB error", slog.String("error", delErr.Error()))
		}
		s.logger.Error("failed to create media file record", slog.String("error", err.Error()))
		return nil, err
	}

	return &mediaFile, nil
}

// GetAvatar retrieves a user's avatar.
func (s *Service) GetAvatar(ctx context.Context, userID uuid.UUID) ([]byte, string, error) {
	// Find avatar file for user
	files, err := s.queries.GetMediaFilesByOwner(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		return nil, "", err
	}

	var avatarFile *generated.MediaFile
	for _, f := range files {
		if strings.HasPrefix(f.S3Key, AvatarPrefix+"/") {
			avatarFile = &f
			break
		}
	}

	if avatarFile == nil {
		return nil, "", apperrors.ErrAvatarNotFound
	}

	// Download from S3
	reader, _, err := s.s3Client.Download(ctx, avatarFile.S3Key)
	if err != nil {
		// If the S3 object doesn't exist, treat it as avatar not found
		if errors.Is(err, ErrObjectNotFound) {
			return nil, "", apperrors.ErrAvatarNotFound
		}
		s.logger.Error("failed to download from S3", slog.String("error", err.Error()))
		return nil, "", err
	}
	defer func() { _ = reader.Close() }()

	encryptedData, err := io.ReadAll(reader)
	if err != nil {
		s.logger.Error("failed to read S3 data", slog.String("error", err.Error()))
		return nil, "", err
	}

	// Decrypt file key
	fileKey, err := s.encryptor.DecryptFileKey(avatarFile.EncryptionKey, avatarFile.EncryptionIv)
	if err != nil {
		s.logger.Error("failed to decrypt file key", slog.String("error", err.Error()))
		return nil, "", err
	}

	// Decrypt file data
	data, err := s.encryptor.Decrypt(encryptedData, fileKey)
	if err != nil {
		s.logger.Error("failed to decrypt file", slog.String("error", err.Error()))
		return nil, "", err
	}

	return data, avatarFile.ContentType, nil
}

// DeleteAvatar deletes a user's avatar.
func (s *Service) DeleteAvatar(ctx context.Context, userID uuid.UUID) error {
	// Find avatar file for user
	files, err := s.queries.GetMediaFilesByOwner(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		return err
	}

	for _, f := range files {
		if strings.HasPrefix(f.S3Key, AvatarPrefix+"/") {
			// Delete from S3
			if err := s.s3Client.Delete(ctx, f.S3Key); err != nil {
				s.logger.Error("failed to delete from S3", slog.String("error", err.Error()))
				return err
			}

			// Delete from database
			if err := s.queries.DeleteMediaFile(ctx, f.ID); err != nil {
				s.logger.Error("failed to delete media file record", slog.String("error", err.Error()))
				return err
			}

			return nil
		}
	}

	return apperrors.ErrAvatarNotFound
}

// GetAvatarURL returns the API URL for a user's avatar.
func (s *Service) GetAvatarURL(ctx context.Context, userID uuid.UUID) (string, error) {
	files, err := s.queries.GetMediaFilesByOwner(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		return "", err
	}

	for _, f := range files {
		if strings.HasPrefix(f.S3Key, AvatarPrefix+"/") {
			// Return API URL (not direct S3 URL since data is encrypted)
			return fmt.Sprintf("/api/v1/users/%s/avatar", userID.String()), nil
		}
	}

	return "", nil
}

// detectContentType detects the MIME type from file data and filename.
func detectContentType(data []byte, filename string) string {
	// First try to detect from magic bytes
	contentType := detectFromMagicBytes(data)
	if contentType != "" {
		return contentType
	}

	// Fall back to extension
	ext := strings.ToLower(filepath.Ext(filename))
	return mime.TypeByExtension(ext)
}

// detectFromMagicBytes detects content type from file header bytes.
func detectFromMagicBytes(data []byte) string {
	if len(data) < 8 {
		return ""
	}

	// PNG: 89 50 4E 47 0D 0A 1A 0A
	if len(data) >= 8 && data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "image/png"
	}

	// JPEG: FF D8 FF
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg"
	}

	// GIF: 47 49 46 38
	if len(data) >= 4 && data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 {
		return "image/gif"
	}

	// WebP: 52 49 46 46 ... 57 45 42 50
	if len(data) >= 12 && data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
		return "image/webp"
	}

	return ""
}

// extensionFromMIME returns a file extension for a MIME type.
func extensionFromMIME(mimeType string) string {
	switch mimeType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}
