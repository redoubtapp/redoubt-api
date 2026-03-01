package handlers

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/redoubtapp/redoubt-api/internal/auth"
	"github.com/redoubtapp/redoubt-api/internal/db/generated"
	"github.com/redoubtapp/redoubt-api/internal/presence"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// In production, implement proper origin checking
		return true
	},
}

// WebSocketHandler handles WebSocket connections for real-time presence.
type WebSocketHandler struct {
	hub        *presence.Hub
	jwtManager *auth.JWTManager
	queries    *generated.Queries
	logger     *slog.Logger
}

// NewWebSocketHandler creates a new WebSocket handler.
func NewWebSocketHandler(hub *presence.Hub, jwtManager *auth.JWTManager, queries *generated.Queries, logger *slog.Logger) *WebSocketHandler {
	return &WebSocketHandler{
		hub:        hub,
		jwtManager: jwtManager,
		queries:    queries,
		logger:     logger,
	}
}

// ServeHTTP upgrades HTTP connections to WebSocket and handles authentication.
func (h *WebSocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Get token from query parameter or Authorization header
	token := r.URL.Query().Get("token")
	if token == "" {
		// Try Authorization header
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}
	}

	// Validate token
	if token == "" {
		h.logger.Warn("websocket auth failed: missing token",
			slog.String("remote_addr", r.RemoteAddr),
			slog.String("url", r.URL.String()),
		)
		http.Error(w, "missing authentication token", http.StatusUnauthorized)
		return
	}

	claims, err := h.jwtManager.ValidateToken(token)
	if err != nil {
		h.logger.Warn("websocket auth failed: invalid token",
			slog.String("error", err.Error()),
			slog.String("remote_addr", r.RemoteAddr),
			slog.Int("token_length", len(token)),
		)
		http.Error(w, "invalid authentication token", http.StatusUnauthorized)
		return
	}

	userID, err := claims.GetUserID()
	if err != nil {
		http.Error(w, "invalid token claims", http.StatusUnauthorized)
		return
	}

	// Look up the username from the database
	user, err := h.queries.GetUserByID(r.Context(), pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		h.logger.Error("failed to get user for websocket",
			slog.String("user_id", userID.String()),
			slog.String("error", err.Error()),
		)
		http.Error(w, "user not found", http.StatusUnauthorized)
		return
	}
	username := user.Username

	// Upgrade to WebSocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("websocket upgrade failed",
			slog.String("error", err.Error()),
			slog.String("remote_addr", r.RemoteAddr),
		)
		return
	}

	h.logger.Debug("websocket upgrade successful",
		slog.String("user_id", userID.String()),
		slog.String("username", username),
		slog.String("remote_addr", r.RemoteAddr),
	)

	// Create connection
	conn := presence.NewConnection(ws, userID.String(), username, h.hub, h.logger)

	// Register with hub
	h.hub.Register(conn)

	// Send auth success
	h.logger.Debug("sending auth success",
		slog.String("user_id", userID.String()),
	)
	conn.SendAuthSuccess()

	// Start read/write pumps
	h.logger.Debug("starting read/write pumps",
		slog.String("user_id", userID.String()),
	)
	go conn.WritePump()
	conn.ReadPump() // This blocks until connection closes
}
