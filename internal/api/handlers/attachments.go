package handlers

import (
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/redoubtapp/redoubt-api/internal/api/middleware"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
	"github.com/redoubtapp/redoubt-api/internal/messages"
	"github.com/redoubtapp/redoubt-api/internal/storage"
)

// AttachmentHandler handles attachment endpoints.
type AttachmentHandler struct {
	storageService *storage.Service
	messageService *messages.Service
}

// NewAttachmentHandler creates a new attachment handler.
func NewAttachmentHandler(storageService *storage.Service, messageService *messages.Service) *AttachmentHandler {
	return &AttachmentHandler{
		storageService: storageService,
		messageService: messageService,
	}
}

// AttachmentResponse is the response for an attachment.
type AttachmentResponse struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	URL         string `json:"url"`
	IsImage     bool   `json:"is_image"`
}

// UploadAttachment handles uploading an attachment to a message.
// POST /messages/:id/attachments
func (h *AttachmentHandler) UploadAttachment(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	messageID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid message ID")
		return
	}

	// Verify user owns the message
	isAdmin := middleware.GetIsAdmin(r.Context())
	msg, err := h.messageService.GetMessage(r.Context(), messageID, userID, isAdmin)
	if err != nil {
		handleAttachmentError(w, r, err)
		return
	}

	if msg.Author.ID != userID && !isAdmin {
		apperrors.Forbidden(w, r)
		return
	}

	// Parse multipart form (max 25MB)
	if err := r.ParseMultipartForm(storage.MaxAttachmentSize); err != nil {
		apperrors.BadRequest(w, r, "Invalid multipart form or file too large")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		apperrors.BadRequest(w, r, "No file provided")
		return
	}
	defer file.Close()

	// Read file data
	data, err := io.ReadAll(io.LimitReader(file, storage.MaxAttachmentSize+1))
	if err != nil {
		apperrors.InternalError(w, r)
		return
	}

	if len(data) > storage.MaxAttachmentSize {
		apperrors.BadRequest(w, r, "File too large (max 25MB)")
		return
	}

	// Get display order from form
	order := 0
	if orderStr := r.FormValue("order"); orderStr != "" {
		if o, err := strconv.Atoi(orderStr); err == nil {
			order = o
		}
	}

	// Upload attachment
	attachment, err := h.storageService.UploadAttachment(r.Context(), storage.UploadAttachmentRequest{
		MessageID: messageID,
		UserID:    userID,
		Filename:  header.Filename,
		Data:      data,
		Order:     order,
	})
	if err != nil {
		handleAttachmentError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, AttachmentResponse{
		ID:          attachment.ID.String(),
		Filename:    attachment.Filename,
		ContentType: attachment.ContentType,
		SizeBytes:   attachment.SizeBytes,
		URL:         attachment.URL,
		IsImage:     attachment.IsImage,
	})
}

// GetAttachment retrieves an attachment's data.
// GET /attachments/:id
func (h *AttachmentHandler) GetAttachment(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	attachmentID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid attachment ID")
		return
	}

	data, contentType, filename, err := h.storageService.GetAttachment(r.Context(), attachmentID)
	if err != nil {
		handleAttachmentError(w, r, err)
		return
	}

	// Set content disposition for downloads
	w.Header().Set("Content-Disposition", "inline; filename=\""+filename+"\"")
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "private, max-age=3600")

	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// DownloadAttachment retrieves an attachment as a download.
// GET /attachments/:id/download
func (h *AttachmentHandler) DownloadAttachment(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	attachmentID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid attachment ID")
		return
	}

	data, contentType, filename, err := h.storageService.GetAttachment(r.Context(), attachmentID)
	if err != nil {
		handleAttachmentError(w, r, err)
		return
	}

	// Force download with attachment disposition
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))

	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// DeleteAttachment deletes an attachment.
// DELETE /attachments/:id
func (h *AttachmentHandler) DeleteAttachment(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	attachmentID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid attachment ID")
		return
	}

	// Verify ownership: look up the attachment's message and check the author
	isAdmin := middleware.GetIsAdmin(r.Context())
	attachment, err := h.storageService.GetAttachmentInfo(r.Context(), attachmentID)
	if err != nil {
		handleAttachmentError(w, r, err)
		return
	}

	msg, err := h.messageService.GetMessage(r.Context(), attachment.MessageID, userID, isAdmin)
	if err != nil {
		handleAttachmentError(w, r, err)
		return
	}

	if msg.Author.ID != userID && !isAdmin {
		apperrors.Forbidden(w, r)
		return
	}

	if err := h.storageService.DeleteAttachment(r.Context(), attachmentID); err != nil {
		handleAttachmentError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetMessageAttachments lists attachments for a message.
// GET /messages/:id/attachments
func (h *AttachmentHandler) GetMessageAttachments(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	messageID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid message ID")
		return
	}

	// Verify user can access the message
	isAdmin := middleware.GetIsAdmin(r.Context())
	if _, err := h.messageService.GetMessage(r.Context(), messageID, userID, isAdmin); err != nil {
		handleAttachmentError(w, r, err)
		return
	}

	attachments, err := h.storageService.GetMessageAttachments(r.Context(), messageID)
	if err != nil {
		handleAttachmentError(w, r, err)
		return
	}

	response := make([]AttachmentResponse, len(attachments))
	for i, a := range attachments {
		response[i] = AttachmentResponse{
			ID:          a.ID.String(),
			Filename:    a.Filename,
			ContentType: a.ContentType,
			SizeBytes:   a.SizeBytes,
			URL:         a.URL,
			IsImage:     a.IsImage,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"attachments": response})
}

// handleAttachmentError maps attachment errors to HTTP responses.
func handleAttachmentError(w http.ResponseWriter, r *http.Request, err error) {
	switch err {
	case apperrors.ErrAttachmentNotFound:
		apperrors.NotFound(w, r, "Attachment")
	case apperrors.ErrMessageNotFound:
		apperrors.NotFound(w, r, "Message")
	case apperrors.ErrFileTooLarge:
		apperrors.BadRequest(w, r, "File too large (max 25MB)")
	case apperrors.ErrInvalidFileType:
		apperrors.BadRequest(w, r, "Invalid file type")
	case apperrors.ErrTooManyAttachments:
		apperrors.BadRequest(w, r, "Too many attachments (max 10 per message)")
	case apperrors.ErrForbidden, apperrors.ErrInsufficientRole:
		apperrors.Forbidden(w, r)
	default:
		apperrors.InternalError(w, r)
	}
}
