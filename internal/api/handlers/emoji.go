package handlers

import (
	"net/http"

	"github.com/redoubtapp/redoubt-api/internal/api/middleware"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
	"github.com/redoubtapp/redoubt-api/internal/messages"
)

// EmojiHandler handles emoji endpoints.
type EmojiHandler struct {
	reactionService *messages.ReactionService
}

// NewEmojiHandler creates a new emoji handler.
func NewEmojiHandler(reactionService *messages.ReactionService) *EmojiHandler {
	return &EmojiHandler{
		reactionService: reactionService,
	}
}

// EmojiResponse is the response for a single emoji.
type EmojiResponse struct {
	Emoji    string `json:"emoji"`
	Name     string `json:"name"`
	Category string `json:"category"`
}

// ListEmojiResponse is the response for listing emoji.
type ListEmojiResponse struct {
	Emoji []EmojiResponse `json:"emoji"`
}

// ListEmoji returns all curated emoji.
func (h *EmojiHandler) ListEmoji(w http.ResponseWriter, r *http.Request) {
	// Auth check - emoji list requires authentication
	_, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	emoji, err := h.reactionService.GetAllEmoji(r.Context())
	if err != nil {
		apperrors.InternalError(w, r)
		return
	}

	response := ListEmojiResponse{
		Emoji: make([]EmojiResponse, len(emoji)),
	}

	for i, e := range emoji {
		response.Emoji[i] = EmojiResponse{
			Emoji:    e.Emoji,
			Name:     e.Name,
			Category: e.Category,
		}
	}

	writeJSON(w, http.StatusOK, response)
}
