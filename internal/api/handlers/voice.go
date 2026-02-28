package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/redoubtapp/redoubt-api/internal/api/middleware"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
	"github.com/redoubtapp/redoubt-api/internal/voice"
)

// VoiceHandler handles voice channel endpoints.
type VoiceHandler struct {
	voiceService *voice.Service
	validate     *validator.Validate
}

// NewVoiceHandler creates a new voice handler.
func NewVoiceHandler(voiceService *voice.Service) *VoiceHandler {
	return &VoiceHandler{
		voiceService: voiceService,
		validate:     validator.New(),
	}
}

// JoinVoiceChannelResponse is the response for joining a voice channel.
type JoinVoiceChannelResponse struct {
	Token        string `json:"token"`
	WebSocketURL string `json:"ws_url"`
	RoomName     string `json:"room_name"`
}

// UpdateMuteStateRequest is the request body for updating mute state.
type UpdateMuteStateRequest struct {
	SelfMuted    *bool `json:"self_muted"`
	SelfDeafened *bool `json:"self_deafened"`
}

// ServerMuteRequest is the request body for server-muting a user.
type ServerMuteRequest struct {
	Muted bool `json:"muted"`
}

// VoiceParticipantResponse is the response for a voice participant.
type VoiceParticipantResponse struct {
	UserID       string  `json:"user_id"`
	Username     string  `json:"username"`
	AvatarURL    *string `json:"avatar_url,omitempty"`
	SelfMuted    bool    `json:"self_muted"`
	SelfDeafened bool    `json:"self_deafened"`
	ServerMuted  bool    `json:"server_muted"`
	ConnectedAt  string  `json:"connected_at"`
}

// VoiceStateResponse is the response for voice state.
type VoiceStateResponse struct {
	ChannelID    string `json:"channel_id"`
	SpaceID      string `json:"space_id"`
	SelfMuted    bool   `json:"self_muted"`
	SelfDeafened bool   `json:"self_deafened"`
	ServerMuted  bool   `json:"server_muted"`
}

// JoinVoiceChannel allows a user to join a voice channel.
func (h *VoiceHandler) JoinVoiceChannel(w http.ResponseWriter, r *http.Request) {
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
	result, err := h.voiceService.Join(r.Context(), channelID, userID, isAdmin)
	if err != nil {
		handleVoiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, JoinVoiceChannelResponse{
		Token:        result.Token,
		WebSocketURL: result.WebSocketURL,
		RoomName:     result.RoomName,
	})
}

// LeaveVoiceChannel allows a user to leave their current voice channel.
func (h *VoiceHandler) LeaveVoiceChannel(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	if err := h.voiceService.Leave(r.Context(), userID); err != nil {
		handleVoiceError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpdateMuteState updates the user's self-mute/deafen state.
func (h *VoiceHandler) UpdateMuteState(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	var req UpdateMuteStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.BadRequest(w, r, "Invalid request body")
		return
	}

	if req.SelfMuted == nil && req.SelfDeafened == nil {
		apperrors.BadRequest(w, r, "At least one of self_muted or self_deafened must be provided")
		return
	}

	if err := h.voiceService.UpdateMuteState(r.Context(), userID, voice.UpdateMuteStateRequest{
		SelfMuted:    req.SelfMuted,
		SelfDeafened: req.SelfDeafened,
	}); err != nil {
		handleVoiceError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ServerMute allows an admin to server-mute another user.
func (h *VoiceHandler) ServerMute(w http.ResponseWriter, r *http.Request) {
	actorUserID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	targetUserID, err := uuid.Parse(mux.Vars(r)["userId"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid user ID")
		return
	}

	var req ServerMuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.BadRequest(w, r, "Invalid request body")
		return
	}

	isAdmin := middleware.GetIsAdmin(r.Context())
	if err := h.voiceService.ServerMute(r.Context(), targetUserID, actorUserID, req.Muted, isAdmin); err != nil {
		handleVoiceError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetChannelParticipants returns all participants in a voice channel.
func (h *VoiceHandler) GetChannelParticipants(w http.ResponseWriter, r *http.Request) {
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
	participants, err := h.voiceService.GetChannelParticipants(r.Context(), channelID, userID, isAdmin)
	if err != nil {
		handleVoiceError(w, r, err)
		return
	}

	response := make([]VoiceParticipantResponse, 0, len(participants))
	for _, p := range participants {
		response = append(response, VoiceParticipantResponse{
			UserID:       p.UserID.String(),
			Username:     p.Username,
			AvatarURL:    p.AvatarURL,
			SelfMuted:    p.SelfMuted,
			SelfDeafened: p.SelfDeafened,
			ServerMuted:  p.ServerMuted,
			ConnectedAt:  p.ConnectedAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"participants": response})
}

// GetVoiceState returns the current voice state for the authenticated user.
func (h *VoiceHandler) GetVoiceState(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	state, err := h.voiceService.GetUserVoiceState(r.Context(), userID)
	if err != nil {
		handleVoiceError(w, r, err)
		return
	}

	if state == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"voice_state": nil})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"voice_state": VoiceStateResponse{
			ChannelID:    state.ChannelID.String(),
			SpaceID:      state.SpaceID.String(),
			SelfMuted:    state.SelfMuted,
			SelfDeafened: state.SelfDeafened,
			ServerMuted:  state.ServerMuted,
		},
	})
}

// handleVoiceError maps voice errors to HTTP responses.
func handleVoiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch err {
	case apperrors.ErrUserNotFound:
		apperrors.NotFound(w, r, "User")
	case apperrors.ErrChannelNotFound:
		apperrors.NotFound(w, r, "Channel")
	case apperrors.ErrNotVoiceChannel:
		apperrors.BadRequest(w, r, "This is not a voice channel")
	case apperrors.ErrAlreadyInVoice:
		apperrors.Conflict(w, r, "Already in a voice channel")
	case apperrors.ErrVoiceChannelFull:
		apperrors.Conflict(w, r, "Voice channel is full")
	case apperrors.ErrNotInVoiceChannel:
		apperrors.BadRequest(w, r, "Not in a voice channel")
	case apperrors.ErrLiveKitUnavailable:
		apperrors.ServiceUnavailable(w, r, "Voice service is unavailable")
	case apperrors.ErrCannotMuteSelf:
		apperrors.BadRequest(w, r, "Cannot server-mute yourself")
	case apperrors.ErrCannotMuteHigherRole:
		apperrors.Forbidden(w, r)
	case apperrors.ErrForbidden:
		apperrors.Forbidden(w, r)
	default:
		apperrors.InternalError(w, r)
	}
}
