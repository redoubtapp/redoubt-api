package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/redoubtapp/redoubt-api/internal/api/middleware"
	"github.com/redoubtapp/redoubt-api/internal/audit"
	"github.com/redoubtapp/redoubt-api/internal/channels"
	"github.com/redoubtapp/redoubt-api/internal/db/generated"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
)

// ChannelHandler handles channel endpoints.
type ChannelHandler struct {
	channelService *channels.Service
	auditService   *audit.Service
	validate       *validator.Validate
}

// NewChannelHandler creates a new channel handler.
func NewChannelHandler(channelService *channels.Service, auditService *audit.Service) *ChannelHandler {
	return &ChannelHandler{
		channelService: channelService,
		auditService:   auditService,
		validate:       validator.New(),
	}
}

// ListChannelsResponse is the response for listing channels.
type ListChannelsResponse struct {
	Channels []ChannelResponse `json:"channels"`
}

// CreateChannelRequest is the request body for creating a channel.
type CreateChannelRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
	Type string `json:"type" validate:"required,oneof=text voice"`
}

// UpdateChannelRequest is the request body for updating a channel.
type UpdateChannelRequest struct {
	Name *string `json:"name" validate:"omitempty,min=1,max=100"`
	Type *string `json:"type" validate:"omitempty,oneof=text voice"`
}

// ReorderChannelsRequest is the request body for reordering channels.
type ReorderChannelsRequest struct {
	ChannelIDs []string `json:"channel_ids" validate:"required,min=1,dive,uuid"`
}

// ListChannels lists all channels in a space.
func (h *ChannelHandler) ListChannels(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	spaceID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid space ID")
		return
	}

	isAdmin := middleware.GetIsAdmin(r.Context())
	spaceChannels, err := h.channelService.ListSpaceChannels(r.Context(), spaceID, userID, isAdmin)
	if err != nil {
		handleChannelError(w, r, err)
		return
	}

	response := ListChannelsResponse{
		Channels: make([]ChannelResponse, 0, len(spaceChannels)),
	}

	for _, ch := range spaceChannels {
		response.Channels = append(response.Channels, channelServiceToResponse(ch))
	}

	writeJSON(w, http.StatusOK, response)
}

// CreateChannel creates a new channel in a space.
func (h *ChannelHandler) CreateChannel(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	spaceID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid space ID")
		return
	}

	var req CreateChannelRequest
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
	channel, err := h.channelService.CreateChannel(r.Context(), channels.CreateChannelRequest{
		SpaceID: spaceID,
		Name:    req.Name,
		Type:    generated.ChannelType(req.Type),
	}, userID, isAdmin)
	if err != nil {
		handleChannelError(w, r, err)
		return
	}

	// Audit log
	if h.auditService != nil {
		channelID := channels.UUIDFromPgtype(channel.ID)
		_ = h.auditService.LogChannelCreate(r.Context(), userID, channelID, spaceID, channel.Name, getClientIP(r))
	}

	writeJSON(w, http.StatusCreated, channelServiceToResponse(*channel))
}

// GetChannel returns a channel by ID.
func (h *ChannelHandler) GetChannel(w http.ResponseWriter, r *http.Request) {
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
	channel, err := h.channelService.GetChannel(r.Context(), channelID, userID, isAdmin)
	if err != nil {
		handleChannelError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, channelServiceToResponse(*channel))
}

// UpdateChannel updates a channel.
func (h *ChannelHandler) UpdateChannel(w http.ResponseWriter, r *http.Request) {
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

	var req UpdateChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.BadRequest(w, r, "Invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		fieldErrors := validationErrors(err)
		apperrors.ValidationError(w, r, fieldErrors)
		return
	}

	var channelType *generated.ChannelType
	if req.Type != nil {
		ct := generated.ChannelType(*req.Type)
		channelType = &ct
	}

	isAdmin := middleware.GetIsAdmin(r.Context())
	channel, err := h.channelService.UpdateChannel(r.Context(), channels.UpdateChannelRequest{
		ChannelID: channelID,
		Name:      req.Name,
		Type:      channelType,
	}, userID, isAdmin)
	if err != nil {
		handleChannelError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, channelServiceToResponse(*channel))
}

// DeleteChannel soft-deletes a channel.
func (h *ChannelHandler) DeleteChannel(w http.ResponseWriter, r *http.Request) {
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

	// Get channel info for audit log before deleting
	var channelName string
	var spaceID uuid.UUID
	if h.auditService != nil {
		if channel, err := h.channelService.GetChannel(r.Context(), channelID, userID, isAdmin); err == nil {
			channelName = channel.Name
			spaceID = channels.UUIDFromPgtype(channel.SpaceID)
		}
	}

	if err := h.channelService.DeleteChannel(r.Context(), channelID, userID, isAdmin); err != nil {
		handleChannelError(w, r, err)
		return
	}

	// Audit log
	if h.auditService != nil {
		_ = h.auditService.LogChannelDelete(r.Context(), userID, channelID, spaceID, channelName, getClientIP(r))
	}

	w.WriteHeader(http.StatusNoContent)
}

// ReorderChannels reorders channels in a space.
func (h *ChannelHandler) ReorderChannels(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	spaceID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid space ID")
		return
	}

	var req ReorderChannelsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.BadRequest(w, r, "Invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		fieldErrors := validationErrors(err)
		apperrors.ValidationError(w, r, fieldErrors)
		return
	}

	channelIDs := make([]uuid.UUID, 0, len(req.ChannelIDs))
	for _, idStr := range req.ChannelIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			apperrors.BadRequest(w, r, "Invalid channel ID in list")
			return
		}
		channelIDs = append(channelIDs, id)
	}

	isAdmin := middleware.GetIsAdmin(r.Context())
	if err := h.channelService.ReorderChannels(r.Context(), channels.ReorderChannelsRequest{
		SpaceID:    spaceID,
		ChannelIDs: channelIDs,
	}, userID, isAdmin); err != nil {
		handleChannelError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Channels reordered successfully"})
}

// channelServiceToResponse converts a Channel to ChannelResponse.
func channelServiceToResponse(channel generated.Channel) ChannelResponse {
	return ChannelResponse{
		ID:        channels.UUIDFromPgtype(channel.ID).String(),
		SpaceID:   channels.UUIDFromPgtype(channel.SpaceID).String(),
		Name:      channel.Name,
		Type:      string(channel.Type),
		Position:  channel.Position,
		CreatedAt: channel.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: channel.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}
}

// handleChannelError maps channel errors to HTTP responses.
func handleChannelError(w http.ResponseWriter, r *http.Request, err error) {
	switch err {
	case apperrors.ErrChannelNotFound:
		apperrors.NotFound(w, r, "Channel")
	case apperrors.ErrSpaceNotFound:
		apperrors.NotFound(w, r, "Space")
	case apperrors.ErrForbidden, apperrors.ErrInsufficientRole:
		apperrors.Forbidden(w, r)
	default:
		apperrors.InternalError(w, r)
	}
}
