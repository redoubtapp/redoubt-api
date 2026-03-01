package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/redoubtapp/redoubt-api/internal/api/middleware"
	"github.com/redoubtapp/redoubt-api/internal/auth"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
)

// SessionHandler handles session management endpoints.
type SessionHandler struct {
	authService *auth.Service
}

// NewSessionHandler creates a new session handler.
func NewSessionHandler(authService *auth.Service) *SessionHandler {
	return &SessionHandler{
		authService: authService,
	}
}

// SessionResponse represents a session in API responses.
type SessionResponse struct {
	ID         string  `json:"id"`
	UserAgent  *string `json:"user_agent"`
	IPAddress  *string `json:"ip_address"`
	LastUsedAt string  `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
	Current    bool    `json:"current"`
}

// ListSessionsResponse is the response for listing sessions.
type ListSessionsResponse struct {
	Sessions []SessionResponse `json:"sessions"`
}

// ListSessions returns all active sessions for the current user.
func (h *SessionHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	// Get current session ID from context (if available)
	currentSessionID, hasCurrentSession := middleware.GetSessionID(r.Context())

	sessions, err := h.authService.GetUserSessions(r.Context(), userID)
	if err != nil {
		apperrors.InternalError(w, r)
		return
	}

	response := ListSessionsResponse{
		Sessions: make([]SessionResponse, 0, len(sessions)),
	}

	for _, session := range sessions {
		sessionID := auth.UUIDFromPgtype(session.ID)

		var userAgent *string
		if session.UserAgent.Valid {
			userAgent = &session.UserAgent.String
		}

		var ipAddress *string
		if session.IpAddress != nil {
			ip := session.IpAddress.String()
			ipAddress = &ip
		}

		response.Sessions = append(response.Sessions, SessionResponse{
			ID:         sessionID.String(),
			UserAgent:  userAgent,
			IPAddress:  ipAddress,
			LastUsedAt: session.LastUsedAt.Time.Format("2006-01-02T15:04:05Z"),
			CreatedAt:  session.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
			Current:    hasCurrentSession && sessionID == currentSessionID,
		})
	}

	writeJSON(w, http.StatusOK, response)
}

// RevokeSession revokes a specific session.
func (h *SessionHandler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	// Get session ID from URL
	vars := mux.Vars(r)
	sessionIDStr := vars["id"]

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		apperrors.BadRequest(w, r, "Invalid session ID")
		return
	}

	if err := h.authService.RevokeSession(r.Context(), userID, sessionID); err != nil {
		handleSessionError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Session revoked successfully"})
}

// RevokeAllSessionsRequest is the request body for revoking all sessions.
type RevokeAllSessionsRequest struct {
	ExcludeCurrent bool `json:"exclude_current"`
}

// RevokeAllSessions revokes all sessions for the current user.
func (h *SessionHandler) RevokeAllSessions(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apperrors.Unauthorized(w, r)
		return
	}

	// Parse optional request body
	var req RevokeAllSessionsRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperrors.BadRequest(w, r, "Invalid request body")
			return
		}
	}

	var currentSessionID *uuid.UUID
	if req.ExcludeCurrent {
		if sid, ok := middleware.GetSessionID(r.Context()); ok {
			currentSessionID = &sid
		}
	}

	if err := h.authService.RevokeAllSessions(r.Context(), userID, currentSessionID); err != nil {
		apperrors.InternalError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "All sessions revoked successfully"})
}

// handleSessionError maps session errors to HTTP responses.
func handleSessionError(w http.ResponseWriter, r *http.Request, err error) {
	switch err {
	case apperrors.ErrSessionNotFound:
		apperrors.NotFound(w, r, "Session")
	case apperrors.ErrForbidden:
		apperrors.Forbidden(w, r)
	default:
		apperrors.InternalError(w, r)
	}
}
