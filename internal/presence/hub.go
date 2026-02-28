package presence

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/redoubtapp/redoubt-api/internal/db/generated"
)

// Hub manages WebSocket connections and broadcasts presence events.
type Hub struct {
	mu          sync.RWMutex
	connections map[string]*Connection     // userID -> connection
	spaces      map[string]map[string]bool // spaceID -> set of userIDs
	typing      map[string]time.Time       // "channelID:userID" -> last typing time
	queries     *generated.Queries
	logger      *slog.Logger

	// Channels for concurrent operations
	register   chan *Connection
	unregister chan *Connection
	done       chan struct{}
}

// NewHub creates a new presence hub.
func NewHub(queries *generated.Queries, logger *slog.Logger) *Hub {
	h := &Hub{
		connections: make(map[string]*Connection),
		spaces:      make(map[string]map[string]bool),
		typing:      make(map[string]time.Time),
		queries:     queries,
		logger:      logger,
		register:    make(chan *Connection),
		unregister:  make(chan *Connection),
		done:        make(chan struct{}),
	}
	return h
}

// Run starts the hub's main loop.
func (h *Hub) Run() {
	typingTicker := time.NewTicker(1 * time.Second)
	defer typingTicker.Stop()

	for {
		select {
		case conn := <-h.register:
			h.handleRegister(conn)
		case conn := <-h.unregister:
			h.handleUnregister(conn)
		case <-typingTicker.C:
			h.cleanupTyping()
		case <-h.done:
			return
		}
	}
}

// Stop gracefully shuts down the hub by closing all connections.
func (h *Hub) Stop() {
	// Signal the Run loop to stop
	close(h.done)

	// Close all active connections
	h.mu.Lock()
	for _, conn := range h.connections {
		conn.Close()
	}
	h.connections = make(map[string]*Connection)
	h.mu.Unlock()
}

// Register adds a connection to the hub.
func (h *Hub) Register(conn *Connection) {
	h.register <- conn
}

// Unregister removes a connection from the hub.
func (h *Hub) Unregister(conn *Connection) {
	h.unregister <- conn
}

func (h *Hub) handleRegister(conn *Connection) {
	h.mu.Lock()

	// Disconnect existing connection for this user (one connection per user)
	if existing, ok := h.connections[conn.UserID]; ok {
		existing.CloseSessionReplaced()
	}

	h.connections[conn.UserID] = conn

	// Release lock before DB operations
	h.mu.Unlock()

	// Update user presence to online (runs DB query)
	h.updateUserPresence(conn.UserID, StatusOnline)

	h.logger.Info("user connected",
		slog.String("user_id", conn.UserID),
		slog.String("username", conn.Username),
	)
}

func (h *Hub) handleUnregister(conn *Connection) {
	h.mu.Lock()

	// Only unregister if this is the current connection
	if existing, ok := h.connections[conn.UserID]; ok && existing == conn {
		delete(h.connections, conn.UserID)

		// Remove from all space subscriptions
		for spaceID, users := range h.spaces {
			delete(users, conn.UserID)
			if len(users) == 0 {
				delete(h.spaces, spaceID)
			}
		}

		// Release lock before doing async operations to avoid deadlock
		h.mu.Unlock()

		// Update user presence to offline (runs DB query)
		h.updateUserPresence(conn.UserID, StatusOffline)

		// Broadcast offline status to all spaces the user was in
		h.broadcastUserOffline(conn.UserID, conn.Username)

		h.logger.Info("user disconnected",
			slog.String("user_id", conn.UserID),
			slog.String("username", conn.Username),
		)
	} else {
		h.mu.Unlock()
	}
}

// SubscribeToSpace adds a user to a space's presence notifications.
func (h *Hub) SubscribeToSpace(userID, spaceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.spaces[spaceID] == nil {
		h.spaces[spaceID] = make(map[string]bool)
	}
	h.spaces[spaceID][userID] = true
}

// UnsubscribeFromSpace removes a user from a space's presence notifications.
func (h *Hub) UnsubscribeFromSpace(userID, spaceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if users, ok := h.spaces[spaceID]; ok {
		delete(users, userID)
		if len(users) == 0 {
			delete(h.spaces, spaceID)
		}
	}
}

// BroadcastToSpace sends an event to all users subscribed to a space.
func (h *Hub) BroadcastToSpace(spaceID string, event Event) {
	h.mu.RLock()
	users := make([]string, 0, len(h.spaces[spaceID]))
	for userID := range h.spaces[spaceID] {
		users = append(users, userID)
	}
	h.mu.RUnlock()

	data, err := json.Marshal(event)
	if err != nil {
		h.logger.Error("failed to marshal event", slog.String("error", err.Error()))
		return
	}

	for _, userID := range users {
		h.mu.RLock()
		conn := h.connections[userID]
		h.mu.RUnlock()

		if conn != nil {
			conn.Send(data)
		}
	}
}

// BroadcastToUser sends an event to a specific user.
func (h *Hub) BroadcastToUser(userID string, event Event) {
	h.mu.RLock()
	conn := h.connections[userID]
	h.mu.RUnlock()

	if conn == nil {
		return
	}

	data, err := json.Marshal(event)
	if err != nil {
		h.logger.Error("failed to marshal event", slog.String("error", err.Error()))
		return
	}

	conn.Send(data)
}

// PublishVoiceJoin broadcasts a voice join event to the space.
func (h *Hub) PublishVoiceJoin(ctx context.Context, channelID, userID string) {
	// Get channel info to find space
	channelUUID, err := uuid.Parse(channelID)
	if err != nil {
		return
	}

	channel, err := h.queries.GetChannelByID(ctx, pgtype.UUID{Bytes: channelUUID, Valid: true})
	if err != nil {
		return
	}

	spaceID := uuidToString(channel.SpaceID)

	// Get user info
	userUUID, _ := uuid.Parse(userID)
	user, err := h.queries.GetUserByID(ctx, pgtype.UUID{Bytes: userUUID, Valid: true})
	if err != nil {
		return
	}

	// Get voice connection state
	conn, err := h.queries.GetVoiceConnectionByUserID(ctx, pgtype.UUID{Bytes: userUUID, Valid: true})
	if err != nil {
		return
	}

	var avatarURL *string
	if user.AvatarUrl.Valid {
		avatarURL = &user.AvatarUrl.String
	}

	event := NewEvent(EventTypeVoiceJoin, VoiceJoinPayload{
		ChannelID:    channelID,
		SpaceID:      spaceID,
		UserID:       userID,
		Username:     user.Username,
		AvatarURL:    avatarURL,
		SelfMuted:    conn.SelfMuted,
		SelfDeafened: conn.SelfDeafened,
		ServerMuted:  conn.ServerMuted,
	})

	h.BroadcastToSpace(spaceID, event)
}

// PublishVoiceLeave broadcasts a voice leave event to the space.
func (h *Hub) PublishVoiceLeave(ctx context.Context, channelID, userID string) {
	// Parse room name to extract space and channel info
	// Room names are formatted as "space_<spaceID>_channel_<channelID>"
	// For webhooks, we may need to look this up differently

	channelUUID, err := uuid.Parse(channelID)
	if err != nil {
		// If channelID is actually a room name, try to parse it
		h.logger.Debug("could not parse channelID as UUID, may be room name",
			slog.String("channel_id", channelID))
		return
	}

	channel, err := h.queries.GetChannelByID(ctx, pgtype.UUID{Bytes: channelUUID, Valid: true})
	if err != nil {
		return
	}

	spaceID := uuidToString(channel.SpaceID)

	// Get user info
	userUUID, _ := uuid.Parse(userID)
	user, err := h.queries.GetUserByID(ctx, pgtype.UUID{Bytes: userUUID, Valid: true})
	username := ""
	if err == nil {
		username = user.Username
	}

	event := NewEvent(EventTypeVoiceLeave, VoiceLeavePayload{
		ChannelID: channelID,
		SpaceID:   spaceID,
		UserID:    userID,
		Username:  username,
	})

	h.BroadcastToSpace(spaceID, event)
}

// PublishVoiceMute broadcasts a mute state change to the space.
func (h *Hub) PublishVoiceMute(ctx context.Context, channelID, userID string, muted bool) {
	channelUUID, err := uuid.Parse(channelID)
	if err != nil {
		return
	}

	channel, err := h.queries.GetChannelByID(ctx, pgtype.UUID{Bytes: channelUUID, Valid: true})
	if err != nil {
		return
	}

	spaceID := uuidToString(channel.SpaceID)

	event := NewEvent(EventTypeVoiceMute, VoiceMutePayload{
		ChannelID:   channelID,
		SpaceID:     spaceID,
		UserID:      userID,
		ServerMuted: &muted,
	})

	h.BroadcastToSpace(spaceID, event)
}

// PublishMessage broadcasts a new message to all space subscribers.
func (h *Hub) PublishMessage(spaceID string, payload MessageCreatePayload) {
	payload.SpaceID = spaceID
	h.BroadcastToSpace(spaceID, NewEvent(EventTypeMessageCreate, payload))
}

// PublishMessageUpdate broadcasts a message edit to all space subscribers.
func (h *Hub) PublishMessageUpdate(spaceID string, payload MessageUpdatePayload) {
	payload.SpaceID = spaceID
	h.BroadcastToSpace(spaceID, NewEvent(EventTypeMessageUpdate, payload))
}

// PublishMessageDelete broadcasts a message deletion to all space subscribers.
func (h *Hub) PublishMessageDelete(spaceID string, payload MessageDeletePayload) {
	payload.SpaceID = spaceID
	h.BroadcastToSpace(spaceID, NewEvent(EventTypeMessageDelete, payload))
}

// PublishReaction broadcasts a reaction change to all space subscribers.
func (h *Hub) PublishReaction(spaceID string, add bool, payload ReactionPayload) {
	payload.SpaceID = spaceID
	eventType := EventTypeReactionAdd
	if !add {
		eventType = EventTypeReactionRemove
	}
	h.BroadcastToSpace(spaceID, NewEvent(eventType, payload))
}

// SetTyping marks a user as typing in a channel.
func (h *Hub) SetTyping(ctx context.Context, channelID, userID, username string) {
	key := channelID + ":" + userID

	h.mu.Lock()
	_, wasTyping := h.typing[key]
	h.typing[key] = time.Now()
	h.mu.Unlock()

	// Only broadcast if this is a new typing event
	if !wasTyping {
		channelUUID, err := uuid.Parse(channelID)
		if err != nil {
			return
		}

		channel, err := h.queries.GetChannelByID(ctx, pgtype.UUID{Bytes: channelUUID, Valid: true})
		if err != nil {
			return
		}

		spaceID := uuidToString(channel.SpaceID)

		event := NewEvent(EventTypeTypingStart, TypingPayload{
			ChannelID: channelID,
			UserID:    userID,
			Username:  username,
		})

		h.BroadcastToSpace(spaceID, event)
	}
}

// cleanupTyping removes stale typing indicators and broadcasts stop events.
func (h *Hub) cleanupTyping() {
	h.mu.Lock()
	now := time.Now()
	expired := make([]string, 0)

	for key, lastTyping := range h.typing {
		if now.Sub(lastTyping) > 5*time.Second {
			expired = append(expired, key)
		}
	}

	for _, key := range expired {
		delete(h.typing, key)
	}
	h.mu.Unlock()

	// Broadcast typing stop events in a separate goroutine to avoid blocking the hub
	if len(expired) > 0 {
		go h.broadcastTypingStop(expired)
	}
}

// broadcastTypingStop sends typing.stop events for expired typing indicators.
func (h *Hub) broadcastTypingStop(expired []string) {
	ctx := context.Background()
	for _, key := range expired {
		// Key format is "channelID:userID"
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		channelID, userID := parts[0], parts[1]

		channelUUID, err := uuid.Parse(channelID)
		if err != nil {
			continue
		}

		channel, err := h.queries.GetChannelByID(ctx, pgtype.UUID{Bytes: channelUUID, Valid: true})
		if err != nil {
			continue
		}

		spaceID := uuidToString(channel.SpaceID)

		event := NewEvent(EventTypeTypingStop, TypingPayload{
			ChannelID: channelID,
			UserID:    userID,
		})

		h.BroadcastToSpace(spaceID, event)
	}
}

// updateUserPresence updates the user's presence status in the database.
func (h *Hub) updateUserPresence(userID string, status PresenceStatus) {
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return
	}

	ctx := context.Background()
	err = h.queries.UpdateUserPresence(ctx, generated.UpdateUserPresenceParams{
		ID:       pgtype.UUID{Bytes: userUUID, Valid: true},
		Presence: generated.PresenceStatus(status),
	})
	if err != nil {
		h.logger.Error("failed to update user presence",
			slog.String("user_id", userID),
			slog.String("error", err.Error()),
		)
	}
}

// broadcastUserOffline broadcasts offline status to all spaces.
func (h *Hub) broadcastUserOffline(userID, username string) {
	event := NewEvent(EventTypeUserOffline, UserPresencePayload{
		UserID:   userID,
		Username: username,
		Status:   StatusOffline,
	})

	// Broadcast to all spaces (simplified - in production might want to track which spaces user was in)
	for spaceID := range h.spaces {
		h.BroadcastToSpace(spaceID, event)
	}
}

// GetOnlineUsersInSpace returns the list of online users in a space.
func (h *Hub) GetOnlineUsersInSpace(spaceID string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	users := make([]string, 0)
	if spaceUsers, ok := h.spaces[spaceID]; ok {
		for userID := range spaceUsers {
			if _, connected := h.connections[userID]; connected {
				users = append(users, userID)
			}
		}
	}
	return users
}

// IsUserOnline checks if a user is currently connected.
func (h *Hub) IsUserOnline(userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.connections[userID]
	return ok
}

// GetConnectionCount returns the number of active WebSocket connections.
func (h *Hub) GetConnectionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.connections)
}

// uuidToString converts a pgtype.UUID to a string.
func uuidToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}
