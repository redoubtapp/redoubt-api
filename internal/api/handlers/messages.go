package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/redoubtapp/redoubt-api/internal/api/middleware"
	"github.com/redoubtapp/redoubt-api/internal/audit"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
	"github.com/redoubtapp/redoubt-api/internal/messages"
)

// MessageHandler handles message endpoints.
type MessageHandler struct {
	messageService   *messages.Service
	readStateService *messages.ReadStateService
	auditService     *audit.Service
	validate         *validator.Validate
}

// NewMessageHandler creates a new message handler.
func NewMessageHandler(
	messageService *messages.Service,
	readStateService *messages.ReadStateService,
	auditService *audit.Service,
) *MessageHandler {
	return &MessageHandler{
		messageService:   messageService,
		readStateService: readStateService,
		auditService:     auditService,
		validate:         validator.New(),
	}
}

// SendMessageRequest is the request body for sending a message.
type SendMessageRequest struct {
	Content  string  `json:"content" validate:"required,min=1,max=2000"`
	ThreadID *string `json:"thread_id" validate:"omitempty,uuid"`
	Nonce    string  `json:"nonce" validate:"omitempty,max=64"`
}

// EditMessageRequest is the request body for editing a message.
type EditMessageRequest struct {
	Content string `json:"content" validate:"required,min=1,max=2000"`
}

// MarkAsReadRequest is the request body for marking a channel as read.
type MarkAsReadRequest struct {
	MessageID *string `json:"message_id" validate:"omitempty,uuid"`
}

// MessageResponse is the response for a message.
type MessageResponse struct {
	ID           string               `json:"id"`
	ChannelID    string               `json:"channel_id"`
	Author       AuthorResponse       `json:"author"`
	Content      string               `json:"content"`
	ThreadID     *string              `json:"thread_id,omitempty"`
	IsThreadRoot bool                 `json:"is_thread_root"`
	ReplyCount   int32                `json:"reply_count"`
	EditedAt     *string              `json:"edited_at,omitempty"`
	CreatedAt    string               `json:"created_at"`
	Attachments  []AttachmentResponse `json:"attachments,omitempty"`
}

// AuthorResponse is the author info in a message response.
type AuthorResponse struct {
	ID        string  `json:"id"`
	Username  string  `json:"username"`
	AvatarURL *string `json:"avatar_url,omitempty"`
}

// MessageEditResponse is the response for a message edit history entry.
type MessageEditResponse struct {
	ID              string `json:"id"`
	PreviousContent string `json:"previous_content"`
	EditedAt        string `json:"edited_at"`
}

// ListMessagesResponse is the response for listing messages.
type ListMessagesResponse struct {
	Messages   []MessageResponse `json:"messages"`
	NextCursor string            `json:"next_cursor,omitempty"`
	HasMore    bool              `json:"has_more"`
}

// UnreadCountResponse is the response for unread count.
type UnreadCountResponse struct {
	UnreadCount int `json:"unread_count"`
}

// ReadStateResponse is the response for read state.
type ReadStateResponse struct {
	LastReadAt        *string `json:"last_read_at,omitempty"`
	LastReadMessageID *string `json:"last_read_message_id,omitempty"`
}

// SendMessage sends a message to a channel.
func (h *MessageHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	channelID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid channel ID")
		return
	}

	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.BadRequest(w, r, "Invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		fieldErrors := validationErrors(err)
		apperrors.ValidationError(w, r, fieldErrors)
		return
	}

	var threadID *uuid.UUID
	if req.ThreadID != nil {
		tid, err := uuid.Parse(*req.ThreadID)
		if err != nil {
			apperrors.BadRequest(w, r, "Invalid thread ID")
			return
		}
		threadID = &tid
	}

	isAdmin := middleware.GetIsAdmin(r.Context())
	msg, err := h.messageService.SendMessage(r.Context(), messages.SendMessageRequest{
		ChannelID: channelID,
		Content:   req.Content,
		ThreadID:  threadID,
		Nonce:     req.Nonce,
	}, userID, isAdmin)
	if err != nil {
		handleMessageError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, messageWithAuthorToResponse(msg))
}

// ListMessages lists messages in a channel.
func (h *MessageHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	channelID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid channel ID")
		return
	}

	cursor := r.URL.Query().Get("cursor")
	limitStr := r.URL.Query().Get("limit")
	var limit int32 = 50
	if limitStr != "" {
		l, err := strconv.ParseInt(limitStr, 10, 32)
		if err != nil || l <= 0 || l > 50 {
			apperrors.BadRequest(w, r, "Invalid limit parameter")
			return
		}
		limit = int32(l)
	}

	isAdmin := middleware.GetIsAdmin(r.Context())
	result, err := h.messageService.ListChannelMessages(r.Context(), channelID, cursor, limit, userID, isAdmin)
	if err != nil {
		handleMessageError(w, r, err)
		return
	}

	response := ListMessagesResponse{
		Messages:   make([]MessageResponse, len(result.Messages)),
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}

	for i, msg := range result.Messages {
		response.Messages[i] = messageWithAuthorToResponse(&msg)
	}

	writeJSON(w, http.StatusOK, response)
}

// GetMessage returns a single message.
func (h *MessageHandler) GetMessage(w http.ResponseWriter, r *http.Request) {
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

	isAdmin := middleware.GetIsAdmin(r.Context())
	msg, err := h.messageService.GetMessage(r.Context(), messageID, userID, isAdmin)
	if err != nil {
		handleMessageError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, messageWithAuthorToResponse(msg))
}

// EditMessage edits a message.
func (h *MessageHandler) EditMessage(w http.ResponseWriter, r *http.Request) {
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

	var req EditMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.BadRequest(w, r, "Invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		fieldErrors := validationErrors(err)
		apperrors.ValidationError(w, r, fieldErrors)
		return
	}

	isAdmin := middleware.GetIsAdmin(r.Context())
	msg, err := h.messageService.EditMessage(r.Context(), messageID, req.Content, userID, isAdmin)
	if err != nil {
		handleMessageError(w, r, err)
		return
	}

	// Return minimal response for edit
	response := map[string]any{
		"id":         messages.UUIDFromPgtype(msg.ID).String(),
		"content":    msg.Content,
		"channel_id": messages.UUIDFromPgtype(msg.ChannelID).String(),
	}
	if msg.EditedAt.Valid {
		response["edited_at"] = msg.EditedAt.Time.Format("2006-01-02T15:04:05Z")
	}

	writeJSON(w, http.StatusOK, response)
}

// DeleteMessage deletes a message.
func (h *MessageHandler) DeleteMessage(w http.ResponseWriter, r *http.Request) {
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

	isAdmin := middleware.GetIsAdmin(r.Context())
	if err := h.messageService.DeleteMessage(r.Context(), messageID, userID, isAdmin); err != nil {
		handleMessageError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetEditHistory returns the edit history for a message.
func (h *MessageHandler) GetEditHistory(w http.ResponseWriter, r *http.Request) {
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

	isAdmin := middleware.GetIsAdmin(r.Context())
	edits, err := h.messageService.GetEditHistory(r.Context(), messageID, userID, isAdmin)
	if err != nil {
		handleMessageError(w, r, err)
		return
	}

	response := make([]MessageEditResponse, len(edits))
	for i, edit := range edits {
		response[i] = MessageEditResponse{
			ID:              messages.UUIDFromPgtype(edit.ID).String(),
			PreviousContent: edit.PreviousContent,
			EditedAt:        edit.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"edits": response})
}

// GetThreadReplies returns replies in a thread.
func (h *MessageHandler) GetThreadReplies(w http.ResponseWriter, r *http.Request) {
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

	isAdmin := middleware.GetIsAdmin(r.Context())
	replies, err := h.messageService.GetThreadReplies(r.Context(), messageID, userID, isAdmin)
	if err != nil {
		handleMessageError(w, r, err)
		return
	}

	response := make([]MessageResponse, len(replies))
	for i, msg := range replies {
		response[i] = messageWithAuthorToResponse(&msg)
	}

	writeJSON(w, http.StatusOK, map[string]any{"replies": response})
}

// ReplyToThread creates a reply in a thread.
func (h *MessageHandler) ReplyToThread(w http.ResponseWriter, r *http.Request) {
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

	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.BadRequest(w, r, "Invalid request body")
		return
	}

	// Remove thread_id validation since we're using path param
	if err := h.validate.Var(req.Content, "required,min=1,max=2000"); err != nil {
		apperrors.BadRequest(w, r, "Invalid content")
		return
	}

	// Get the parent message to find the channel
	isAdmin := middleware.GetIsAdmin(r.Context())
	parentMsg, err := h.messageService.GetMessage(r.Context(), messageID, userID, isAdmin)
	if err != nil {
		handleMessageError(w, r, err)
		return
	}

	channelID := messages.UUIDFromPgtype(parentMsg.Message.ChannelID)
	msg, err := h.messageService.SendMessage(r.Context(), messages.SendMessageRequest{
		ChannelID: channelID,
		Content:   req.Content,
		ThreadID:  &messageID,
		Nonce:     req.Nonce,
	}, userID, isAdmin)
	if err != nil {
		handleMessageError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, messageWithAuthorToResponse(msg))
}

// MarkChannelAsRead marks a channel as read.
func (h *MessageHandler) MarkChannelAsRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	channelID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid channel ID")
		return
	}

	var req MarkAsReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Allow empty body - marks all as read
		req = MarkAsReadRequest{}
	}

	var messageID *uuid.UUID
	if req.MessageID != nil {
		mid, err := uuid.Parse(*req.MessageID)
		if err != nil {
			apperrors.BadRequest(w, r, "Invalid message ID")
			return
		}
		messageID = &mid
	}

	isAdmin := middleware.GetIsAdmin(r.Context())
	if err := h.readStateService.MarkChannelAsRead(r.Context(), channelID, messageID, userID, isAdmin); err != nil {
		handleMessageError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Channel marked as read"})
}

// GetUnreadCount returns the unread count for a channel.
func (h *MessageHandler) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	channelID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid channel ID")
		return
	}

	isAdmin := middleware.GetIsAdmin(r.Context())
	count, err := h.readStateService.GetUnreadCount(r.Context(), channelID, userID, isAdmin)
	if err != nil {
		handleMessageError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, UnreadCountResponse{UnreadCount: count})
}

// messageWithAuthorToResponse converts a MessageWithAuthor to MessageResponse.
func messageWithAuthorToResponse(msg *messages.MessageWithAuthor) MessageResponse {
	response := MessageResponse{
		ID:        messages.UUIDFromPgtype(msg.Message.ID).String(),
		ChannelID: messages.UUIDFromPgtype(msg.Message.ChannelID).String(),
		Author: AuthorResponse{
			ID:        msg.Author.ID.String(),
			Username:  msg.Author.Username,
			AvatarURL: msg.Author.AvatarURL,
		},
		Content:      msg.Message.Content,
		IsThreadRoot: msg.Message.IsThreadRoot,
		ReplyCount:   msg.Message.ReplyCount,
		CreatedAt:    msg.Message.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}

	if msg.Message.ThreadID.Valid {
		tid := messages.UUIDFromPgtype(msg.Message.ThreadID).String()
		response.ThreadID = &tid
	}

	if msg.Message.EditedAt.Valid {
		editedAt := msg.Message.EditedAt.Time.Format("2006-01-02T15:04:05Z")
		response.EditedAt = &editedAt
	}

	// Add attachments
	if len(msg.Attachments) > 0 {
		response.Attachments = make([]AttachmentResponse, len(msg.Attachments))
		for i, att := range msg.Attachments {
			response.Attachments[i] = AttachmentResponse{
				ID:          att.ID.String(),
				Filename:    att.Filename,
				ContentType: att.ContentType,
				SizeBytes:   att.SizeBytes,
				URL:         "/api/v1/attachments/" + att.ID.String(),
				IsImage:     att.IsImage,
			}
		}
	}

	return response
}

// handleMessageError maps message errors to HTTP responses.
func handleMessageError(w http.ResponseWriter, r *http.Request, err error) {
	switch err {
	case apperrors.ErrMessageNotFound:
		apperrors.NotFound(w, r, "Message")
	case apperrors.ErrChannelNotFound:
		apperrors.NotFound(w, r, "Channel")
	case apperrors.ErrForbidden, apperrors.ErrInsufficientRole:
		apperrors.Forbidden(w, r)
	case apperrors.ErrMessageTooLong:
		apperrors.BadRequest(w, r, "Message content is too long (max 2000 characters)")
	case apperrors.ErrMessageEmpty:
		apperrors.BadRequest(w, r, "Message content cannot be empty")
	case apperrors.ErrCodeBlockTooLong:
		apperrors.BadRequest(w, r, "Code block exceeds maximum length (1500 characters)")
	case apperrors.ErrEditWindowExpired:
		apperrors.BadRequest(w, r, "Edit window has expired")
	case apperrors.ErrCannotEditOthers:
		apperrors.Forbidden(w, r)
	case apperrors.ErrCannotDeleteOthers:
		apperrors.Forbidden(w, r)
	case apperrors.ErrRateLimited:
		apperrors.TooManyRequests(w, r)
	case apperrors.ErrInvalidInput:
		apperrors.BadRequest(w, r, "Invalid input")
	default:
		apperrors.InternalError(w, r)
	}
}
