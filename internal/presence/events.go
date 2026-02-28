package presence

import "time"

// Event types for WebSocket communication.
const (
	EventTypeAuth           = "auth"
	EventTypeAuthSuccess    = "auth.success"
	EventTypeAuthError      = "auth.error"
	EventTypePing           = "ping"
	EventTypePong           = "pong"
	EventTypeSubscribe      = "subscribe"
	EventTypeUnsubscribe    = "unsubscribe"
	EventTypePresenceUpdate = "presence.update"
	EventTypeVoiceJoin      = "voice.join"
	EventTypeVoiceLeave     = "voice.leave"
	EventTypeVoiceMute      = "voice.mute"
	EventTypeTypingStart    = "typing.start"
	EventTypeTypingStop     = "typing.stop"
	EventTypeUserOnline     = "user.online"
	EventTypeUserOffline    = "user.offline"
	EventTypeUserIdle       = "user.idle"
	EventTypeError          = "error"

	// Message events
	EventTypeMessageCreate  = "message.create"
	EventTypeMessageUpdate  = "message.update"
	EventTypeMessageDelete  = "message.delete"
	EventTypeReactionAdd    = "reaction.add"
	EventTypeReactionRemove = "reaction.remove"
	EventTypeThreadReply    = "thread.reply"
)

// Event is the base structure for all WebSocket messages.
type Event struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   interface{} `json:"payload,omitempty"`
}

// NewEvent creates a new event with the current timestamp.
func NewEvent(eventType string, payload interface{}) Event {
	return Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Payload:   payload,
	}
}

// AuthPayload is sent by the client to authenticate.
type AuthPayload struct {
	Token string `json:"token"`
}

// AuthSuccessPayload is sent on successful authentication.
type AuthSuccessPayload struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

// SubscribePayload is sent to subscribe to a space's presence events.
type SubscribePayload struct {
	SpaceID string `json:"space_id"`
}

// PresenceStatus represents a user's online status.
type PresenceStatus string

const (
	StatusOnline  PresenceStatus = "online"
	StatusIdle    PresenceStatus = "idle"
	StatusOffline PresenceStatus = "offline"
)

// UserPresencePayload contains user presence information.
type UserPresencePayload struct {
	UserID   string         `json:"user_id"`
	Username string         `json:"username"`
	Status   PresenceStatus `json:"status"`
	SpaceID  string         `json:"space_id,omitempty"`
}

// VoiceJoinPayload is sent when a user joins a voice channel.
type VoiceJoinPayload struct {
	ChannelID    string  `json:"channel_id"`
	SpaceID      string  `json:"space_id"`
	UserID       string  `json:"user_id"`
	Username     string  `json:"username"`
	AvatarURL    *string `json:"avatar_url,omitempty"`
	SelfMuted    bool    `json:"self_muted"`
	SelfDeafened bool    `json:"self_deafened"`
	ServerMuted  bool    `json:"server_muted"`
}

// VoiceLeavePayload is sent when a user leaves a voice channel.
type VoiceLeavePayload struct {
	ChannelID string `json:"channel_id"`
	SpaceID   string `json:"space_id"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
}

// VoiceMutePayload is sent when a user's mute state changes.
type VoiceMutePayload struct {
	ChannelID    string `json:"channel_id"`
	SpaceID      string `json:"space_id"`
	UserID       string `json:"user_id"`
	SelfMuted    *bool  `json:"self_muted,omitempty"`
	SelfDeafened *bool  `json:"self_deafened,omitempty"`
	ServerMuted  *bool  `json:"server_muted,omitempty"`
}

// TypingPayload is sent when a user starts/stops typing.
type TypingPayload struct {
	ChannelID string `json:"channel_id"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
}

// ErrorPayload contains error information.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// UserBrief contains minimal user info for message payloads.
type UserBrief struct {
	ID        string  `json:"id"`
	Username  string  `json:"username"`
	AvatarURL *string `json:"avatar_url,omitempty"`
}

// MessageCreatePayload is sent when a new message is created.
type MessageCreatePayload struct {
	ID        string    `json:"id"`
	ChannelID string    `json:"channel_id"`
	SpaceID   string    `json:"space_id"`
	Author    UserBrief `json:"author"`
	Content   string    `json:"content"`
	ThreadID  *string   `json:"thread_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Nonce     string    `json:"nonce,omitempty"`
}

// MessageUpdatePayload is sent when a message is edited.
type MessageUpdatePayload struct {
	ID        string    `json:"id"`
	ChannelID string    `json:"channel_id"`
	SpaceID   string    `json:"space_id"`
	Content   string    `json:"content"`
	EditedAt  time.Time `json:"edited_at"`
	EditCount int       `json:"edit_count"`
}

// MessageDeletePayload is sent when a message is deleted.
type MessageDeletePayload struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	SpaceID   string `json:"space_id"`
}

// ReactionPayload is sent when a reaction is added or removed.
type ReactionPayload struct {
	MessageID string `json:"message_id"`
	ChannelID string `json:"channel_id"`
	SpaceID   string `json:"space_id"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Emoji     string `json:"emoji"`
}
