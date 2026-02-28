package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/redoubtapp/redoubt-api/internal/api/middleware"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
	"github.com/redoubtapp/redoubt-api/internal/messages"
)

// ReactionHandler handles reaction endpoints.
type ReactionHandler struct {
	reactionService *messages.ReactionService
	validate        *validator.Validate
}

// NewReactionHandler creates a new reaction handler.
func NewReactionHandler(reactionService *messages.ReactionService) *ReactionHandler {
	return &ReactionHandler{
		reactionService: reactionService,
		validate:        validator.New(),
	}
}

// AddReactionRequest is the request body for adding a reaction.
type AddReactionRequest struct {
	Emoji string `json:"emoji" validate:"required,min=1,max=32"`
}

// ReactionGroupResponse is the response for a group of reactions.
type ReactionGroupResponse struct {
	Emoji      string   `json:"emoji"`
	Count      int      `json:"count"`
	Users      []string `json:"users"`
	HasReacted bool     `json:"has_reacted"`
}

// AddReaction adds a reaction to a message.
func (h *ReactionHandler) AddReaction(w http.ResponseWriter, r *http.Request) {
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

	var req AddReactionRequest
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
	if err := h.reactionService.AddReaction(r.Context(), messageID, userID, req.Emoji, isAdmin); err != nil {
		handleReactionError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"message": "Reaction added"})
}

// RemoveReaction removes a reaction from a message.
func (h *ReactionHandler) RemoveReaction(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	vars := mux.Vars(r)
	messageID, err := uuid.Parse(vars["id"])
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid message ID")
		return
	}

	emoji := vars["emoji"]
	if emoji == "" {
		apperrors.BadRequest(w, r, "Emoji is required")
		return
	}

	isAdmin := middleware.GetIsAdmin(r.Context())
	if err := h.reactionService.RemoveReaction(r.Context(), messageID, userID, emoji, isAdmin); err != nil {
		handleReactionError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetMessageReactions returns all reactions for a message.
func (h *ReactionHandler) GetMessageReactions(w http.ResponseWriter, r *http.Request) {
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
	reactions, err := h.reactionService.GetMessageReactions(r.Context(), messageID, userID, isAdmin)
	if err != nil {
		handleReactionError(w, r, err)
		return
	}

	response := make([]ReactionGroupResponse, len(reactions))
	for i, rg := range reactions {
		response[i] = ReactionGroupResponse{
			Emoji:      rg.Emoji,
			Count:      rg.Count,
			Users:      rg.Users,
			HasReacted: rg.HasReacted,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"reactions": response})
}

// ToggleReaction toggles a reaction on a message (add if not exists, remove if exists).
func (h *ReactionHandler) ToggleReaction(w http.ResponseWriter, r *http.Request) {
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

	var req AddReactionRequest
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
	added, err := h.reactionService.ToggleReaction(r.Context(), messageID, userID, req.Emoji, isAdmin)
	if err != nil {
		handleReactionError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"added": added})
}

// handleReactionError maps reaction errors to HTTP responses.
func handleReactionError(w http.ResponseWriter, r *http.Request, err error) {
	switch err {
	case apperrors.ErrMessageNotFound:
		apperrors.NotFound(w, r, "Message")
	case apperrors.ErrForbidden, apperrors.ErrInsufficientRole:
		apperrors.Forbidden(w, r)
	case apperrors.ErrInvalidEmoji:
		apperrors.BadRequest(w, r, "Invalid emoji - not in curated set")
	case apperrors.ErrRateLimited:
		apperrors.TooManyRequests(w, r)
	default:
		apperrors.InternalError(w, r)
	}
}
