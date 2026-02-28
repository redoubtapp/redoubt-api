package presence

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = 30 * time.Second

	// Maximum message size allowed from peer.
	maxMessageSize = 4096

	// Send buffer size.
	sendBufferSize = 256

	// Custom close code for when another session takes over.
	// 4000-4999 range is reserved for application use.
	CloseSessionReplaced = 4000
)

// Connection represents a WebSocket connection for a user.
type Connection struct {
	UserID   string
	Username string

	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	logger *slog.Logger

	mu     sync.Mutex
	closed bool
}

// NewConnection creates a new WebSocket connection wrapper.
func NewConnection(ws *websocket.Conn, userID, username string, hub *Hub, logger *slog.Logger) *Connection {
	return &Connection{
		UserID:   userID,
		Username: username,
		hub:      hub,
		conn:     ws,
		send:     make(chan []byte, sendBufferSize),
		logger:   logger,
	}
}

// Send queues a message to be sent to the client.
func (c *Connection) Send(data []byte) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	select {
	case c.send <- data:
	default:
		// Buffer full, close connection
		c.logger.Warn("send buffer full, closing connection",
			slog.String("user_id", c.UserID),
		)
		c.Close()
	}
}

// Close closes the connection.
func (c *Connection) Close() {
	c.closeWithCode(websocket.CloseNormalClosure, "")
}

// CloseSessionReplaced closes the connection because another session took over.
func (c *Connection) CloseSessionReplaced() {
	c.closeWithCode(CloseSessionReplaced, "session replaced by another connection")
}

// closeWithCode closes the connection with a specific close code and reason.
func (c *Connection) closeWithCode(code int, reason string) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.mu.Unlock()

	// Send close message with code before closing
	_ = c.conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(code, reason),
		time.Now().Add(writeWait),
	)

	close(c.send)
	_ = c.conn.Close()
}

// ReadPump handles incoming messages from the WebSocket connection.
// It runs in its own goroutine for each connection.
func (c *Connection) ReadPump() {
	c.logger.Debug("ReadPump started",
		slog.String("user_id", c.UserID),
	)
	defer func() {
		c.logger.Debug("ReadPump exiting",
			slog.String("user_id", c.UserID),
		)
		c.hub.Unregister(c)
		c.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			c.logger.Debug("ReadPump: read error",
				slog.String("error", err.Error()),
				slog.String("user_id", c.UserID),
			)
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Error("websocket read error",
					slog.String("error", err.Error()),
					slog.String("user_id", c.UserID),
				)
			}
			return
		}

		c.logger.Debug("ReadPump: received message",
			slog.String("user_id", c.UserID),
			slog.Int("message_size", len(data)),
		)
		c.handleMessage(data)
	}
}

// WritePump handles outgoing messages to the WebSocket connection.
// It runs in its own goroutine for each connection.
func (c *Connection) WritePump() {
	c.logger.Debug("WritePump started",
		slog.String("user_id", c.UserID),
	)
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		c.logger.Debug("WritePump exiting",
			slog.String("user_id", c.UserID),
		)
		ticker.Stop()
		c.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Channel closed
				c.logger.Debug("WritePump: send channel closed",
					slog.String("user_id", c.UserID),
				)
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			c.logger.Debug("WritePump: sending message",
				slog.String("user_id", c.UserID),
				slog.Int("message_size", len(message)),
			)
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				c.logger.Error("websocket write error",
					slog.String("error", err.Error()),
					slog.String("user_id", c.UserID),
				)
				return
			}
			c.logger.Debug("WritePump: message sent successfully",
				slog.String("user_id", c.UserID),
			)

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
			// Also send application-level ping
			c.sendPing()
		}
	}
}

// handleMessage processes an incoming message from the client.
func (c *Connection) handleMessage(data []byte) {
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		c.logger.Error("invalid message format",
			slog.String("error", err.Error()),
			slog.String("user_id", c.UserID),
		)
		return
	}

	ctx := context.Background()

	switch event.Type {
	case EventTypePong:
		// Application-level pong received, nothing to do

	case EventTypeSubscribe:
		c.handleSubscribe(event.Payload)

	case EventTypeUnsubscribe:
		c.handleUnsubscribe(event.Payload)

	case EventTypeTypingStart:
		c.handleTypingStart(ctx, event.Payload)

	case EventTypePresenceUpdate:
		c.handlePresenceUpdate(event.Payload)

	default:
		c.logger.Debug("unhandled event type",
			slog.String("type", event.Type),
			slog.String("user_id", c.UserID),
		)
	}
}

// handleSubscribe processes a subscribe request.
func (c *Connection) handleSubscribe(payload interface{}) {
	var p SubscribePayload
	if err := mapPayload(payload, &p); err != nil {
		return
	}

	c.hub.SubscribeToSpace(c.UserID, p.SpaceID)

	// Broadcast that user is online to the space
	event := NewEvent(EventTypeUserOnline, UserPresencePayload{
		UserID:   c.UserID,
		Username: c.Username,
		Status:   StatusOnline,
		SpaceID:  p.SpaceID,
	})
	c.hub.BroadcastToSpace(p.SpaceID, event)
}

// handleUnsubscribe processes an unsubscribe request.
func (c *Connection) handleUnsubscribe(payload interface{}) {
	var p SubscribePayload
	if err := mapPayload(payload, &p); err != nil {
		return
	}

	c.hub.UnsubscribeFromSpace(c.UserID, p.SpaceID)
}

// handleTypingStart processes a typing start event.
func (c *Connection) handleTypingStart(ctx context.Context, payload interface{}) {
	var p TypingPayload
	if err := mapPayload(payload, &p); err != nil {
		return
	}

	c.hub.SetTyping(ctx, p.ChannelID, c.UserID, c.Username)
}

// handlePresenceUpdate processes a presence status update.
func (c *Connection) handlePresenceUpdate(payload interface{}) {
	var p UserPresencePayload
	if err := mapPayload(payload, &p); err != nil {
		return
	}

	// Only allow updating own status
	if p.UserID != "" && p.UserID != c.UserID {
		return
	}

	// Update status (e.g., set idle)
	// This would be broadcast to all subscribed spaces
}

// sendPing sends an application-level ping to the client.
func (c *Connection) sendPing() {
	event := NewEvent(EventTypePing, nil)
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	c.Send(data)
}

// SendAuthSuccess sends an authentication success message.
func (c *Connection) SendAuthSuccess() {
	event := NewEvent(EventTypeAuthSuccess, AuthSuccessPayload{
		UserID:   c.UserID,
		Username: c.Username,
	})
	data, _ := json.Marshal(event)
	c.Send(data)
}

// SendError sends an error message to the client.
func (c *Connection) SendError(code, message string) {
	event := NewEvent(EventTypeError, ErrorPayload{
		Code:    code,
		Message: message,
	})
	data, _ := json.Marshal(event)
	c.Send(data)
}

// mapPayload converts a generic payload to a specific type.
func mapPayload(src interface{}, dst interface{}) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}
