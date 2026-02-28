# Redoubt — Phase 2 Implementation Document

**Status:** Ready for implementation
**Last updated:** 2026-02-22
**Author:** Michael

This document defines the complete scope, technical decisions, and implementation details for Phase 2 (Voice & Video) of Redoubt.

---

## Table of Contents

- [1. Phase 2 Scope Summary](#1-phase-2-scope-summary)
- [2. Architecture Decisions](#2-architecture-decisions)
- [3. LiveKit Integration](#3-livekit-integration)
- [4. WebSocket Presence System](#4-websocket-presence-system)
- [5. Voice & Video API](#5-voice--video-api)
- [6. Database Schema Changes](#6-database-schema-changes)
- [7. Tauri Desktop Client](#7-tauri-desktop-client)
- [8. Client State Management](#8-client-state-management)
- [9. Audio & Video Controls](#9-audio--video-controls)
- [10. Permissions & Moderation](#10-permissions--moderation)
- [11. Infrastructure Updates](#11-infrastructure-updates)
- [12. Configuration](#12-configuration)
- [13. Testing Strategy](#13-testing-strategy)
- [14. Implementation Tasks](#14-implementation-tasks)
- [15. Acceptance Criteria](#15-acceptance-criteria)

---

## 1. Phase 2 Scope Summary

Phase 2 adds real-time voice and video communication to Redoubt with the following deliverables:

| Component | Scope |
|-----------|-------|
| LiveKit Integration | Self-hosted SFU, token generation, room management, webhook handling |
| Voice & Video | Full A/V support, screen sharing, adaptive quality |
| WebSocket Presence | Online/offline/idle status, voice channel presence, typing indicators |
| Go API Extensions | Voice channel join/leave, participant management, real-time stats |
| Tauri Desktop Client | Full shell UI with React + TypeScript, voice/video functional, text placeholders |
| Audio Controls | Mute/deafen (server-enforced), PTT + VAD modes, device selection |
| Moderation | Server-enforced mute, force-disconnect capability |

---

## 2. Architecture Decisions

### Core Libraries & Frameworks

| Concern | Choice | Rationale |
|---------|--------|-----------|
| LiveKit Server | Self-hosted Docker | Full control, no external dependency, bundled TURN |
| LiveKit Go SDK | `livekit/server-sdk-go` | Official SDK for token generation and room management |
| WebSocket | `gorilla/websocket` | Battle-tested, good performance, matches existing mux usage |
| Client Framework | Tauri + React + TypeScript | Native performance, shared codebase potential with mobile |
| Client State | Zustand | Lightweight, minimal boilerplate, excellent TS support |
| Client Styling | shadcn/ui + Tailwind CSS | Pre-built accessible components, fast development |
| LiveKit Client SDK | `livekit-client` (JS) | Official JavaScript SDK for WebRTC |

### Key Design Decisions

| Decision | Choice |
|----------|--------|
| Media scope | Full voice + video + screen sharing |
| LiveKit deployment | Self-hosted in Docker Compose |
| Presence architecture | Full (online/offline/idle + voice + typing) |
| WebSocket connection | Single connection for all real-time features |
| Voice quality | Auto/adaptive (LiveKit handles optimization) |
| Audio processing | LiveKit defaults (no user controls) |
| Mute/deafen | Server-enforced (moderation) + voluntary |
| Simultaneous sessions | One voice channel at a time |
| Disconnect handling | Immediate removal from voice channel |
| Room lifecycle | Lazy creation (on first join), destroyed when empty |
| Token generation | Per-request (fresh token each join) |
| Participant limits | Configurable per voice channel |
| Screen sharing | Anyone can share (one at a time) |
| Voice session history | Ephemeral (not persisted) |
| WebSocket format | JSON messages |
| WS keepalive | Both native pings + application heartbeat |
| Typing indicators | Debounced broadcast (5s auto-clear) |
| Input modes | Both VAD and PTT supported |
| Global shortcuts | Essential (PTT works outside app focus) |
| Quality indicators | Self connection quality only |
| System tray | Deferred to later phase |
| LiveKit version | Latest stable (1.5.x) |

---

## 3. LiveKit Integration

### 3.1 Docker Compose Configuration

```yaml
# docker-compose.yml additions
services:
  livekit:
    image: livekit/livekit-server:v1.5
    command: --config /etc/livekit.yaml
    ports:
      - "7880:7880"   # HTTP/WebSocket signaling
      - "7881:7881"   # RTC (WebRTC over TCP fallback)
      - "7882:7882"   # TURN/TLS
      - "50000-50100:50000-50100/udp"  # WebRTC UDP media
    volumes:
      - ./config/livekit.yaml:/etc/livekit.yaml:ro
    environment:
      - LIVEKIT_KEYS=${LIVEKIT_API_KEY}:${LIVEKIT_API_SECRET}
    depends_on:
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:7880"]
      interval: 10s
      timeout: 5s
      retries: 3
    restart: unless-stopped
```

### 3.2 LiveKit Server Configuration

```yaml
# config/livekit.yaml
port: 7880
rtc:
  port_range_start: 50000
  port_range_end: 50100
  tcp_port: 7881
  use_external_ip: true

turn:
  enabled: true
  tls_port: 7882
  udp_port: 7882

redis:
  address: redis:6379

keys:
  # Keys loaded from environment variable LIVEKIT_KEYS

room:
  empty_timeout: 0  # Immediate cleanup when empty
  max_participants: 0  # No global limit (per-channel limits in app)

logging:
  level: info
```

### 3.3 Caddyfile Updates

```
{$DOMAIN:localhost} {
    # Existing API routes
    handle /api/v1/* {
        reverse_proxy redoubt-api:8080
    }

    handle /health {
        reverse_proxy redoubt-api:8080
    }

    # WebSocket for presence/chat
    handle /ws {
        reverse_proxy redoubt-api:8080
    }

    # LiveKit signaling (HTTP/WebSocket)
    handle /livekit/* {
        reverse_proxy livekit:7880
    }

    handle {
        respond "Redoubt API" 200
    }
}
```

### 3.4 Go Package Structure

```
internal/
├── livekit/
│   ├── client.go         # LiveKit server SDK wrapper
│   ├── tokens.go         # Token generation
│   ├── rooms.go          # Room management
│   └── webhooks.go       # Webhook event handler
├── presence/
│   ├── hub.go            # WebSocket connection hub
│   ├── connection.go     # Individual connection handling
│   ├── events.go         # Event types and serialization
│   └── presence.go       # Presence state management
└── api/handlers/
    ├── voice.go          # Voice channel endpoints
    └── ws.go             # WebSocket upgrade handler
```

### 3.5 Token Generation

```go
// internal/livekit/tokens.go

package livekit

import (
    "time"

    "github.com/livekit/protocol/auth"
    "github.com/livekit/protocol/livekit"
)

type TokenService struct {
    apiKey    string
    apiSecret string
}

func NewTokenService(apiKey, apiSecret string) *TokenService {
    return &TokenService{
        apiKey:    apiKey,
        apiSecret: apiSecret,
    }
}

type TokenOptions struct {
    UserID       string
    Username     string
    RoomName     string
    CanPublish   bool
    CanSubscribe bool
    CanPublishData bool
}

func (s *TokenService) GenerateToken(opts TokenOptions) (string, error) {
    at := auth.NewAccessToken(s.apiKey, s.apiSecret)

    grant := &auth.VideoGrant{
        Room:           opts.RoomName,
        RoomJoin:       true,
        CanPublish:     &opts.CanPublish,
        CanSubscribe:   &opts.CanSubscribe,
        CanPublishData: &opts.CanPublishData,
    }

    at.SetVideoGrant(grant).
        SetIdentity(opts.UserID).
        SetName(opts.Username).
        SetValidFor(time.Hour) // 1 hour validity

    return at.ToJWT()
}
```

### 3.6 Room Management

```go
// internal/livekit/rooms.go

package livekit

import (
    "context"

    lksdk "github.com/livekit/server-sdk-go"
    "github.com/livekit/protocol/livekit"
)

type RoomService struct {
    client *lksdk.RoomServiceClient
}

func NewRoomService(host, apiKey, apiSecret string) *RoomService {
    client := lksdk.NewRoomServiceClient(host, apiKey, apiSecret)
    return &RoomService{client: client}
}

// EnsureRoom creates a room if it doesn't exist
func (s *RoomService) EnsureRoom(ctx context.Context, name string, maxParticipants uint32) (*livekit.Room, error) {
    return s.client.CreateRoom(ctx, &livekit.CreateRoomRequest{
        Name:            name,
        EmptyTimeout:    0,  // Destroy immediately when empty
        MaxParticipants: maxParticipants,
    })
}

// GetRoom returns room info or nil if not found
func (s *RoomService) GetRoom(ctx context.Context, name string) (*livekit.Room, error) {
    rooms, err := s.client.ListRooms(ctx, &livekit.ListRoomsRequest{
        Names: []string{name},
    })
    if err != nil {
        return nil, err
    }
    if len(rooms.Rooms) == 0 {
        return nil, nil
    }
    return rooms.Rooms[0], nil
}

// GetParticipants returns all participants in a room
func (s *RoomService) GetParticipants(ctx context.Context, roomName string) ([]*livekit.ParticipantInfo, error) {
    resp, err := s.client.ListParticipants(ctx, &livekit.ListParticipantsRequest{
        Room: roomName,
    })
    if err != nil {
        return nil, err
    }
    return resp.Participants, nil
}

// RemoveParticipant force-disconnects a participant
func (s *RoomService) RemoveParticipant(ctx context.Context, roomName, identity string) error {
    _, err := s.client.RemoveParticipant(ctx, &livekit.RoomParticipantIdentity{
        Room:     roomName,
        Identity: identity,
    })
    return err
}

// MuteParticipant server-side mutes a participant
func (s *RoomService) MuteParticipant(ctx context.Context, roomName, identity string, muted bool) error {
    _, err := s.client.MutePublishedTrack(ctx, &livekit.MuteRoomTrackRequest{
        Room:     roomName,
        Identity: identity,
        Muted:    muted,
        // TrackSid can be left empty to mute all tracks
    })
    return err
}
```

### 3.7 Webhook Handler

```go
// internal/livekit/webhooks.go

package livekit

import (
    "context"
    "encoding/json"
    "io"
    "net/http"

    "github.com/livekit/protocol/auth"
    "github.com/livekit/protocol/webhook"
)

type WebhookHandler struct {
    tokenVerifier *auth.TokenVerifier
    presenceHub   PresencePublisher
}

type PresencePublisher interface {
    PublishVoiceJoin(ctx context.Context, channelID, userID string)
    PublishVoiceLeave(ctx context.Context, channelID, userID string)
    PublishVoiceMute(ctx context.Context, channelID, userID string, muted bool)
}

func NewWebhookHandler(apiKey, apiSecret string, presence PresencePublisher) *WebhookHandler {
    return &WebhookHandler{
        tokenVerifier: auth.NewTokenVerifier(apiKey, apiSecret),
        presenceHub:   presence,
    }
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    body, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "failed to read body", http.StatusBadRequest)
        return
    }

    authHeader := r.Header.Get("Authorization")
    event, err := webhook.ReceiveWebhookEvent(body, authHeader, h.tokenVerifier)
    if err != nil {
        http.Error(w, "invalid webhook", http.StatusUnauthorized)
        return
    }

    ctx := r.Context()

    switch event.GetEvent() {
    case webhook.EventParticipantJoined:
        p := event.GetParticipant()
        h.presenceHub.PublishVoiceJoin(ctx, event.GetRoom().GetName(), p.GetIdentity())

    case webhook.EventParticipantLeft:
        p := event.GetParticipant()
        h.presenceHub.PublishVoiceLeave(ctx, event.GetRoom().GetName(), p.GetIdentity())

    case webhook.EventTrackPublished:
        // Track state changes handled via LiveKit client SDK

    case webhook.EventRoomFinished:
        // Room destroyed, presence already updated via participant left events
    }

    w.WriteHeader(http.StatusOK)
}
```

---

## 4. WebSocket Presence System

### 4.1 Event Types

```go
// internal/presence/events.go

package presence

import "time"

// Event types
const (
    EventTypeAuth           = "auth"
    EventTypeAuthSuccess    = "auth.success"
    EventTypeAuthError      = "auth.error"
    EventTypePing           = "ping"
    EventTypePong           = "pong"
    EventTypePresenceUpdate = "presence.update"
    EventTypeVoiceJoin      = "voice.join"
    EventTypeVoiceLeave     = "voice.leave"
    EventTypeVoiceMute      = "voice.mute"
    EventTypeVoiceDeafen    = "voice.deafen"
    EventTypeTypingStart    = "typing.start"
    EventTypeTypingStop     = "typing.stop"
    EventTypeUserOnline     = "user.online"
    EventTypeUserOffline    = "user.offline"
    EventTypeUserIdle       = "user.idle"
    EventTypeChannelStats   = "channel.stats"
    EventTypeError          = "error"
)

// Base event structure
type Event struct {
    Type      string      `json:"type"`
    Timestamp time.Time   `json:"timestamp"`
    Payload   interface{} `json:"payload,omitempty"`
}

// Auth request from client
type AuthPayload struct {
    Token string `json:"token"`
}

// Presence state
type PresenceStatus string

const (
    StatusOnline  PresenceStatus = "online"
    StatusIdle    PresenceStatus = "idle"
    StatusOffline PresenceStatus = "offline"
)

type UserPresencePayload struct {
    UserID   string         `json:"user_id"`
    Username string         `json:"username"`
    Status   PresenceStatus `json:"status"`
    SpaceID  string         `json:"space_id,omitempty"`
}

// Voice channel presence
type VoicePresencePayload struct {
    ChannelID string `json:"channel_id"`
    SpaceID   string `json:"space_id"`
    UserID    string `json:"user_id"`
    Username  string `json:"username"`
    Muted     bool   `json:"muted"`
    Deafened  bool   `json:"deafened"`
    Video     bool   `json:"video"`
    Streaming bool   `json:"streaming"` // Screen share
}

// Typing indicator
type TypingPayload struct {
    ChannelID string `json:"channel_id"`
    UserID    string `json:"user_id"`
    Username  string `json:"username"`
}

// Channel stats (for voice channels)
type ChannelStatsPayload struct {
    ChannelID        string  `json:"channel_id"`
    ParticipantCount int     `json:"participant_count"`
    Bitrate          int     `json:"bitrate_kbps,omitempty"`
    PacketLoss       float64 `json:"packet_loss_percent,omitempty"`
}

// Error payload
type ErrorPayload struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}
```

### 4.2 WebSocket Hub

```go
// internal/presence/hub.go

package presence

import (
    "context"
    "encoding/json"
    "log/slog"
    "sync"
    "time"
)

type Hub struct {
    mu          sync.RWMutex
    connections map[string]*Connection       // userID -> connection
    spaces      map[string]map[string]bool   // spaceID -> userIDs
    typing      map[string]time.Time         // channelID:userID -> last typing time
    logger      *slog.Logger
}

func NewHub(logger *slog.Logger) *Hub {
    h := &Hub{
        connections: make(map[string]*Connection),
        spaces:      make(map[string]map[string]bool),
        typing:      make(map[string]time.Time),
        logger:      logger,
    }
    go h.cleanupTyping()
    return h
}

// Register adds a connection to the hub
func (h *Hub) Register(conn *Connection) {
    h.mu.Lock()
    defer h.mu.Unlock()

    // Disconnect existing connection for this user
    if existing, ok := h.connections[conn.UserID]; ok {
        existing.Close()
    }

    h.connections[conn.UserID] = conn
}

// Unregister removes a connection
func (h *Hub) Unregister(conn *Connection) {
    h.mu.Lock()
    defer h.mu.Unlock()

    if existing, ok := h.connections[conn.UserID]; ok && existing == conn {
        delete(h.connections, conn.UserID)
    }

    // Remove from all spaces
    for spaceID, users := range h.spaces {
        delete(users, conn.UserID)
        if len(users) == 0 {
            delete(h.spaces, spaceID)
        }
    }
}

// SubscribeToSpace adds user to a space's presence notifications
func (h *Hub) SubscribeToSpace(userID, spaceID string) {
    h.mu.Lock()
    defer h.mu.Unlock()

    if h.spaces[spaceID] == nil {
        h.spaces[spaceID] = make(map[string]bool)
    }
    h.spaces[spaceID][userID] = true
}

// BroadcastToSpace sends event to all users in a space
func (h *Hub) BroadcastToSpace(spaceID string, event Event) {
    h.mu.RLock()
    users := h.spaces[spaceID]
    h.mu.RUnlock()

    data, err := json.Marshal(event)
    if err != nil {
        h.logger.Error("failed to marshal event", "error", err)
        return
    }

    for userID := range users {
        h.mu.RLock()
        conn := h.connections[userID]
        h.mu.RUnlock()

        if conn != nil {
            conn.Send(data)
        }
    }
}

// PublishVoiceJoin broadcasts voice join event
func (h *Hub) PublishVoiceJoin(ctx context.Context, channelID, userID string) {
    // Implementation: look up space from channel, broadcast to space
}

// PublishVoiceLeave broadcasts voice leave event
func (h *Hub) PublishVoiceLeave(ctx context.Context, channelID, userID string) {
    // Implementation
}

// PublishVoiceMute broadcasts mute state change
func (h *Hub) PublishVoiceMute(ctx context.Context, channelID, userID string, muted bool) {
    // Implementation
}

// SetTyping marks a user as typing in a channel
func (h *Hub) SetTyping(channelID, userID, username string) {
    key := channelID + ":" + userID

    h.mu.Lock()
    _, wasTyping := h.typing[key]
    h.typing[key] = time.Now()
    h.mu.Unlock()

    if !wasTyping {
        // Broadcast typing start
        // Look up space from channel, broadcast
    }
}

// cleanupTyping removes stale typing indicators
func (h *Hub) cleanupTyping() {
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()

    for range ticker.C {
        h.mu.Lock()
        now := time.Now()
        for key, lastTyping := range h.typing {
            if now.Sub(lastTyping) > 5*time.Second {
                delete(h.typing, key)
                // Broadcast typing stop
            }
        }
        h.mu.Unlock()
    }
}
```

### 4.3 WebSocket Connection

```go
// internal/presence/connection.go

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
    writeWait      = 10 * time.Second
    pongWait       = 60 * time.Second
    pingPeriod     = 30 * time.Second
    maxMessageSize = 4096
)

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

func NewConnection(ws *websocket.Conn, userID, username string, hub *Hub, logger *slog.Logger) *Connection {
    return &Connection{
        UserID:   userID,
        Username: username,
        hub:      hub,
        conn:     ws,
        send:     make(chan []byte, 256),
        logger:   logger,
    }
}

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
        c.Close()
    }
}

func (c *Connection) Close() {
    c.mu.Lock()
    if c.closed {
        c.mu.Unlock()
        return
    }
    c.closed = true
    c.mu.Unlock()

    close(c.send)
    c.conn.Close()
}

// ReadPump handles incoming messages
func (c *Connection) ReadPump() {
    defer func() {
        c.hub.Unregister(c)
        c.Close()
    }()

    c.conn.SetReadLimit(maxMessageSize)
    c.conn.SetReadDeadline(time.Now().Add(pongWait))
    c.conn.SetPongHandler(func(string) error {
        c.conn.SetReadDeadline(time.Now().Add(pongWait))
        return nil
    })

    for {
        _, data, err := c.conn.ReadMessage()
        if err != nil {
            if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
                c.logger.Error("websocket read error", "error", err, "user_id", c.UserID)
            }
            return
        }

        c.handleMessage(data)
    }
}

// WritePump handles outgoing messages
func (c *Connection) WritePump() {
    ticker := time.NewTicker(pingPeriod)
    defer func() {
        ticker.Stop()
        c.Close()
    }()

    for {
        select {
        case message, ok := <-c.send:
            c.conn.SetWriteDeadline(time.Now().Add(writeWait))
            if !ok {
                c.conn.WriteMessage(websocket.CloseMessage, []byte{})
                return
            }

            if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
                return
            }

        case <-ticker.C:
            c.conn.SetWriteDeadline(time.Now().Add(writeWait))
            if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                return
            }
            // Also send application-level ping
            c.sendPing()
        }
    }
}

func (c *Connection) handleMessage(data []byte) {
    var event Event
    if err := json.Unmarshal(data, &event); err != nil {
        c.logger.Error("invalid message format", "error", err)
        return
    }

    switch event.Type {
    case EventTypePong:
        // Application-level pong received

    case EventTypeTypingStart:
        var payload TypingPayload
        if err := mapPayload(event.Payload, &payload); err == nil {
            c.hub.SetTyping(payload.ChannelID, c.UserID, c.Username)
        }

    case EventTypePresenceUpdate:
        // Handle presence status change (online/idle)
    }
}

func (c *Connection) sendPing() {
    event := Event{
        Type:      EventTypePing,
        Timestamp: time.Now(),
    }
    data, _ := json.Marshal(event)
    c.Send(data)
}

func mapPayload(src interface{}, dst interface{}) error {
    data, err := json.Marshal(src)
    if err != nil {
        return err
    }
    return json.Unmarshal(data, dst)
}
```

---

## 5. Voice & Video API

### 5.1 New Endpoints

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| POST | `/channels/:id/join` | Join voice channel, get LiveKit token | Member |
| POST | `/channels/:id/leave` | Leave voice channel | Member |
| GET | `/channels/:id/participants` | List participants in voice channel | Member |
| GET | `/channels/:id/stats` | Get voice channel quality stats | Member |
| POST | `/channels/:id/mute/:userId` | Server-mute a participant | Admin+ |
| POST | `/channels/:id/unmute/:userId` | Server-unmute a participant | Admin+ |
| POST | `/channels/:id/disconnect/:userId` | Force-disconnect participant | Admin+ |
| GET | `/ws` | WebSocket upgrade for presence | Yes |

### 5.2 Request/Response Examples

#### Join Voice Channel

```http
POST /api/v1/channels/770e8400-e29b-41d4-a716-446655440003/join
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "url": "wss://redoubt.example.com/livekit",
  "room_name": "space_660e8400_channel_770e8400",
  "participant": {
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "username": "alice",
    "can_publish": true,
    "can_subscribe": true
  }
}
```

#### Leave Voice Channel

```http
POST /api/v1/channels/770e8400-e29b-41d4-a716-446655440003/leave
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

```json
{
  "message": "Left voice channel"
}
```

#### Get Participants

```http
GET /api/v1/channels/770e8400-e29b-41d4-a716-446655440003/participants
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

```json
{
  "participants": [
    {
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "username": "alice",
      "muted": false,
      "deafened": false,
      "video": true,
      "streaming": false,
      "joined_at": "2026-02-22T10:30:00Z"
    },
    {
      "user_id": "550e8400-e29b-41d4-a716-446655440001",
      "username": "bob",
      "muted": true,
      "deafened": false,
      "video": false,
      "streaming": false,
      "joined_at": "2026-02-22T10:32:00Z"
    }
  ],
  "count": 2,
  "max_participants": 25
}
```

#### Get Voice Channel Stats

```http
GET /api/v1/channels/770e8400-e29b-41d4-a716-446655440003/stats
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

```json
{
  "channel_id": "770e8400-e29b-41d4-a716-446655440003",
  "participant_count": 2,
  "room": {
    "name": "space_660e8400_channel_770e8400",
    "created_at": "2026-02-22T10:30:00Z"
  },
  "quality": {
    "avg_bitrate_kbps": 48,
    "avg_packet_loss_percent": 0.1
  }
}
```

#### Server-Mute Participant

```http
POST /api/v1/channels/770e8400-e29b-41d4-a716-446655440003/mute/550e8400-e29b-41d4-a716-446655440001
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

```json
{
  "message": "Participant muted"
}
```

### 5.3 Voice Handler Implementation

```go
// internal/api/handlers/voice.go

package handlers

import (
    "encoding/json"
    "net/http"

    "github.com/gorilla/mux"
)

type VoiceHandler struct {
    channelService ChannelService
    livekitService LiveKitService
    presenceHub    PresenceHub
}

type JoinVoiceResponse struct {
    Token       string              `json:"token"`
    URL         string              `json:"url"`
    RoomName    string              `json:"room_name"`
    Participant ParticipantInfo     `json:"participant"`
}

type ParticipantInfo struct {
    UserID       string `json:"user_id"`
    Username     string `json:"username"`
    CanPublish   bool   `json:"can_publish"`
    CanSubscribe bool   `json:"can_subscribe"`
}

func (h *VoiceHandler) JoinChannel(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    user := auth.UserFromContext(ctx)
    channelID := mux.Vars(r)["id"]

    // 1. Verify channel exists and is voice type
    channel, err := h.channelService.GetChannel(ctx, channelID)
    if err != nil {
        errors.NotFound(w, r, "Channel")
        return
    }
    if channel.Type != "voice" {
        errors.BadRequest(w, r, "Not a voice channel")
        return
    }

    // 2. Verify user is member of the space
    if !h.channelService.IsMember(ctx, user.ID, channel.SpaceID) {
        errors.Forbidden(w, r)
        return
    }

    // 3. Check if user is already in another voice channel
    if h.presenceHub.IsInVoice(user.ID) {
        errors.Conflict(w, r, "Already in a voice channel")
        return
    }

    // 4. Check participant limit
    if channel.MaxParticipants > 0 {
        count := h.livekitService.GetParticipantCount(ctx, channel.RoomName())
        if count >= channel.MaxParticipants {
            errors.Conflict(w, r, "Voice channel is full")
            return
        }
    }

    // 5. Ensure room exists
    _, err = h.livekitService.EnsureRoom(ctx, channel.RoomName(), channel.MaxParticipants)
    if err != nil {
        errors.InternalError(w, r)
        return
    }

    // 6. Generate LiveKit token
    token, err := h.livekitService.GenerateToken(livekit.TokenOptions{
        UserID:       user.ID,
        Username:     user.Username,
        RoomName:     channel.RoomName(),
        CanPublish:   true,
        CanSubscribe: true,
        CanPublishData: true,
    })
    if err != nil {
        errors.InternalError(w, r)
        return
    }

    // 7. Return token and connection info
    response := JoinVoiceResponse{
        Token:    token,
        URL:      h.livekitService.WebSocketURL(),
        RoomName: channel.RoomName(),
        Participant: ParticipantInfo{
            UserID:       user.ID,
            Username:     user.Username,
            CanPublish:   true,
            CanSubscribe: true,
        },
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}

func (h *VoiceHandler) LeaveChannel(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    user := auth.UserFromContext(ctx)
    channelID := mux.Vars(r)["id"]

    channel, err := h.channelService.GetChannel(ctx, channelID)
    if err != nil {
        errors.NotFound(w, r, "Channel")
        return
    }

    // Remove participant from LiveKit room
    err = h.livekitService.RemoveParticipant(ctx, channel.RoomName(), user.ID)
    if err != nil {
        // Log but don't fail - participant may have already left
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"message": "Left voice channel"})
}

func (h *VoiceHandler) MuteParticipant(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    user := auth.UserFromContext(ctx)
    channelID := mux.Vars(r)["id"]
    targetUserID := mux.Vars(r)["userId"]

    channel, err := h.channelService.GetChannel(ctx, channelID)
    if err != nil {
        errors.NotFound(w, r, "Channel")
        return
    }

    // Check admin permission
    role, err := h.channelService.GetMemberRole(ctx, user.ID, channel.SpaceID)
    if err != nil || (role != "admin" && role != "owner") {
        errors.Forbidden(w, r)
        return
    }

    // Server-mute the participant
    err = h.livekitService.MuteParticipant(ctx, channel.RoomName(), targetUserID, true)
    if err != nil {
        errors.InternalError(w, r)
        return
    }

    // Broadcast mute event
    h.presenceHub.PublishVoiceMute(ctx, channelID, targetUserID, true)

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"message": "Participant muted"})
}
```

---

## 6. Database Schema Changes

### Migration: `0002_voice_channels.up.sql`

```sql
-- Add max_participants to channels
ALTER TABLE channels
    ADD COLUMN max_participants INTEGER DEFAULT NULL;

-- Voice state tracking (for presence persistence across reconnects)
-- Note: This is optional since we chose ephemeral voice sessions
-- Keeping structure for potential future use

-- Track active voice connections for rate limiting and analytics
CREATE TABLE voice_connections (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id      UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    space_id        UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    muted           BOOLEAN NOT NULL DEFAULT FALSE,
    deafened        BOOLEAN NOT NULL DEFAULT FALSE,
    video_enabled   BOOLEAN NOT NULL DEFAULT FALSE,
    screen_sharing  BOOLEAN NOT NULL DEFAULT FALSE,
    connected_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Only one connection per user
    CONSTRAINT voice_connections_user_unique UNIQUE (user_id)
);

CREATE INDEX idx_voice_connections_channel ON voice_connections(channel_id);
CREATE INDEX idx_voice_connections_space ON voice_connections(space_id);

-- WebSocket connection tracking (for graceful shutdown/reconnect)
CREATE TABLE ws_connections (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    connected_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_ping_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ip_address      INET,
    user_agent      TEXT,

    CONSTRAINT ws_connections_user_unique UNIQUE (user_id)
);

-- Presence status
CREATE TYPE presence_status AS ENUM ('online', 'idle', 'offline');

ALTER TABLE users
    ADD COLUMN presence_status presence_status NOT NULL DEFAULT 'offline',
    ADD COLUMN last_seen_at TIMESTAMPTZ;
```

### Migration: `0002_voice_channels.down.sql`

```sql
ALTER TABLE channels DROP COLUMN IF EXISTS max_participants;
DROP TABLE IF EXISTS voice_connections;
DROP TABLE IF EXISTS ws_connections;
ALTER TABLE users DROP COLUMN IF EXISTS presence_status;
ALTER TABLE users DROP COLUMN IF EXISTS last_seen_at;
DROP TYPE IF EXISTS presence_status;
```

---

## 7. Tauri Desktop Client

### 7.1 Project Structure

```
client/
├── src-tauri/
│   ├── Cargo.toml
│   ├── tauri.conf.json
│   ├── src/
│   │   ├── main.rs
│   │   ├── commands/
│   │   │   ├── mod.rs
│   │   │   ├── shortcuts.rs      # Global keyboard shortcuts
│   │   │   └── audio.rs          # Audio device enumeration
│   │   └── lib.rs
│   └── icons/
├── src/
│   ├── main.tsx
│   ├── App.tsx
│   ├── index.css
│   ├── components/
│   │   ├── ui/                   # shadcn/ui components
│   │   ├── layout/
│   │   │   ├── Sidebar.tsx
│   │   │   ├── SpaceList.tsx
│   │   │   ├── ChannelList.tsx
│   │   │   └── MemberList.tsx
│   │   ├── voice/
│   │   │   ├── VoiceChannel.tsx
│   │   │   ├── VoiceControls.tsx
│   │   │   ├── ParticipantList.tsx
│   │   │   ├── ParticipantTile.tsx
│   │   │   ├── AudioSettings.tsx
│   │   │   └── ConnectionQuality.tsx
│   │   ├── chat/
│   │   │   ├── ChatPlaceholder.tsx   # Non-functional in Phase 2
│   │   │   └── MessageInput.tsx
│   │   └── auth/
│   │       ├── LoginForm.tsx
│   │       └── RegisterForm.tsx
│   ├── hooks/
│   │   ├── useAuth.ts
│   │   ├── useWebSocket.ts
│   │   ├── useVoice.ts
│   │   ├── usePresence.ts
│   │   └── useAudioDevices.ts
│   ├── lib/
│   │   ├── api.ts                # REST API client
│   │   ├── ws.ts                 # WebSocket client
│   │   └── livekit.ts            # LiveKit wrapper
│   ├── store/
│   │   ├── index.ts
│   │   ├── authStore.ts
│   │   ├── spaceStore.ts
│   │   ├── presenceStore.ts
│   │   └── voiceStore.ts
│   └── types/
│       ├── api.ts
│       ├── ws.ts
│       └── voice.ts
├── package.json
├── tsconfig.json
├── tailwind.config.js
├── postcss.config.js
└── vite.config.ts
```

### 7.2 Dependencies

```json
{
  "dependencies": {
    "@livekit/components-react": "^2.0.0",
    "livekit-client": "^2.0.0",
    "@tauri-apps/api": "^2.0.0",
    "@tauri-apps/plugin-global-shortcut": "^2.0.0",
    "react": "^18.2.0",
    "react-dom": "^18.2.0",
    "zustand": "^4.5.0",
    "tailwindcss": "^3.4.0",
    "class-variance-authority": "^0.7.0",
    "clsx": "^2.1.0",
    "lucide-react": "^0.300.0",
    "@radix-ui/react-dialog": "^1.0.0",
    "@radix-ui/react-dropdown-menu": "^2.0.0",
    "@radix-ui/react-tooltip": "^1.0.0"
  },
  "devDependencies": {
    "@tauri-apps/cli": "^2.0.0",
    "@types/react": "^18.2.0",
    "@types/react-dom": "^18.2.0",
    "typescript": "^5.3.0",
    "vite": "^5.0.0",
    "@vitejs/plugin-react": "^4.2.0"
  }
}
```

### 7.3 Tauri Configuration

```json
{
  "productName": "Redoubt",
  "identifier": "com.redoubt.app",
  "version": "0.1.0",
  "build": {
    "beforeDevCommand": "npm run dev",
    "beforeBuildCommand": "npm run build",
    "frontendDist": "../dist",
    "devUrl": "http://localhost:5173"
  },
  "app": {
    "windows": [
      {
        "title": "Redoubt",
        "width": 1280,
        "height": 720,
        "minWidth": 940,
        "minHeight": 500,
        "resizable": true,
        "fullscreen": false,
        "center": true
      }
    ],
    "security": {
      "csp": null
    }
  },
  "plugins": {
    "global-shortcut": {
      "enabled": true
    }
  }
}
```

### 7.4 Global Shortcuts (Rust)

```rust
// src-tauri/src/commands/shortcuts.rs

use tauri::{AppHandle, Manager};
use tauri_plugin_global_shortcut::{GlobalShortcutExt, Shortcut, ShortcutState};

#[tauri::command]
pub async fn register_ptt_shortcut(app: AppHandle, shortcut: String) -> Result<(), String> {
    let shortcut: Shortcut = shortcut.parse().map_err(|e| format!("{}", e))?;

    app.global_shortcut()
        .on_shortcut(shortcut, move |app, _shortcut, event| {
            match event.state() {
                ShortcutState::Pressed => {
                    let _ = app.emit("ptt-pressed", ());
                }
                ShortcutState::Released => {
                    let _ = app.emit("ptt-released", ());
                }
            }
        })
        .map_err(|e| format!("{}", e))?;

    Ok(())
}

#[tauri::command]
pub async fn register_mute_shortcut(app: AppHandle, shortcut: String) -> Result<(), String> {
    let shortcut: Shortcut = shortcut.parse().map_err(|e| format!("{}", e))?;

    app.global_shortcut()
        .on_shortcut(shortcut, move |app, _shortcut, event| {
            if event.state() == ShortcutState::Pressed {
                let _ = app.emit("toggle-mute", ());
            }
        })
        .map_err(|e| format!("{}", e))?;

    Ok(())
}

#[tauri::command]
pub async fn register_deafen_shortcut(app: AppHandle, shortcut: String) -> Result<(), String> {
    let shortcut: Shortcut = shortcut.parse().map_err(|e| format!("{}", e))?;

    app.global_shortcut()
        .on_shortcut(shortcut, move |app, _shortcut, event| {
            if event.state() == ShortcutState::Pressed {
                let _ = app.emit("toggle-deafen", ());
            }
        })
        .map_err(|e| format!("{}", e))?;

    Ok(())
}
```

---

## 8. Client State Management

### 8.1 Voice Store

```typescript
// src/store/voiceStore.ts

import { create } from 'zustand';
import { Room, LocalParticipant, RemoteParticipant, ConnectionState } from 'livekit-client';

interface VoiceState {
  // Connection state
  room: Room | null;
  connectionState: ConnectionState;
  currentChannelId: string | null;

  // Local state
  isMuted: boolean;
  isDeafened: boolean;
  isVideoEnabled: boolean;
  isScreenSharing: boolean;
  inputMode: 'vad' | 'ptt';
  isPttActive: boolean;

  // Device selection
  audioInputDevice: string | null;
  audioOutputDevice: string | null;
  videoInputDevice: string | null;

  // Quality
  connectionQuality: 'excellent' | 'good' | 'poor' | 'unknown';

  // Actions
  setRoom: (room: Room | null) => void;
  setConnectionState: (state: ConnectionState) => void;
  setCurrentChannel: (channelId: string | null) => void;
  toggleMute: () => void;
  toggleDeafen: () => void;
  toggleVideo: () => void;
  toggleScreenShare: () => void;
  setInputMode: (mode: 'vad' | 'ptt') => void;
  setPttActive: (active: boolean) => void;
  setAudioInputDevice: (deviceId: string) => void;
  setAudioOutputDevice: (deviceId: string) => void;
  setVideoInputDevice: (deviceId: string) => void;
  setConnectionQuality: (quality: 'excellent' | 'good' | 'poor' | 'unknown') => void;
  reset: () => void;
}

const initialState = {
  room: null,
  connectionState: ConnectionState.Disconnected,
  currentChannelId: null,
  isMuted: false,
  isDeafened: false,
  isVideoEnabled: false,
  isScreenSharing: false,
  inputMode: 'vad' as const,
  isPttActive: false,
  audioInputDevice: null,
  audioOutputDevice: null,
  videoInputDevice: null,
  connectionQuality: 'unknown' as const,
};

export const useVoiceStore = create<VoiceState>((set, get) => ({
  ...initialState,

  setRoom: (room) => set({ room }),
  setConnectionState: (connectionState) => set({ connectionState }),
  setCurrentChannel: (currentChannelId) => set({ currentChannelId }),

  toggleMute: () => {
    const { room, isMuted, isDeafened } = get();
    if (!room || isDeafened) return;

    const newMuted = !isMuted;
    room.localParticipant.setMicrophoneEnabled(!newMuted);
    set({ isMuted: newMuted });
  },

  toggleDeafen: () => {
    const { room, isDeafened, isMuted } = get();
    if (!room) return;

    const newDeafened = !isDeafened;

    // Deafen also mutes
    if (newDeafened && !isMuted) {
      room.localParticipant.setMicrophoneEnabled(false);
    }

    // Undeafen restores previous mute state
    if (!newDeafened && !isMuted) {
      room.localParticipant.setMicrophoneEnabled(true);
    }

    // Toggle audio subscription for all remote participants
    room.remoteParticipants.forEach(participant => {
      participant.audioTrackPublications.forEach(pub => {
        if (pub.track) {
          pub.track.setEnabled(!newDeafened);
        }
      });
    });

    set({ isDeafened: newDeafened, isMuted: newDeafened || isMuted });
  },

  toggleVideo: () => {
    const { room, isVideoEnabled } = get();
    if (!room) return;

    const newEnabled = !isVideoEnabled;
    room.localParticipant.setCameraEnabled(newEnabled);
    set({ isVideoEnabled: newEnabled });
  },

  toggleScreenShare: async () => {
    const { room, isScreenSharing } = get();
    if (!room) return;

    const newSharing = !isScreenSharing;
    if (newSharing) {
      await room.localParticipant.setScreenShareEnabled(true);
    } else {
      await room.localParticipant.setScreenShareEnabled(false);
    }
    set({ isScreenSharing: newSharing });
  },

  setInputMode: (inputMode) => set({ inputMode }),

  setPttActive: (isPttActive) => {
    const { room, inputMode, isDeafened } = get();
    if (!room || inputMode !== 'ptt' || isDeafened) return;

    room.localParticipant.setMicrophoneEnabled(isPttActive);
    set({ isPttActive, isMuted: !isPttActive });
  },

  setAudioInputDevice: (deviceId) => {
    const { room } = get();
    room?.switchActiveDevice('audioinput', deviceId);
    set({ audioInputDevice: deviceId });
  },

  setAudioOutputDevice: (deviceId) => {
    const { room } = get();
    room?.switchActiveDevice('audiooutput', deviceId);
    set({ audioOutputDevice: deviceId });
  },

  setVideoInputDevice: (deviceId) => {
    const { room } = get();
    room?.switchActiveDevice('videoinput', deviceId);
    set({ videoInputDevice: deviceId });
  },

  setConnectionQuality: (connectionQuality) => set({ connectionQuality }),

  reset: () => set(initialState),
}));
```

### 8.2 Presence Store

```typescript
// src/store/presenceStore.ts

import { create } from 'zustand';

type PresenceStatus = 'online' | 'idle' | 'offline';

interface UserPresence {
  userId: string;
  username: string;
  status: PresenceStatus;
  voiceChannelId?: string;
  muted?: boolean;
  deafened?: boolean;
  video?: boolean;
  streaming?: boolean;
}

interface TypingUser {
  userId: string;
  username: string;
  channelId: string;
  startedAt: number;
}

interface PresenceState {
  // User presence by userId
  presence: Map<string, UserPresence>;

  // Typing indicators by channelId
  typing: Map<string, TypingUser[]>;

  // WebSocket connection
  isConnected: boolean;
  reconnectAttempts: number;

  // Actions
  setPresence: (userId: string, presence: UserPresence) => void;
  removePresence: (userId: string) => void;
  setVoiceState: (userId: string, channelId: string | null, state?: Partial<UserPresence>) => void;
  setTyping: (channelId: string, userId: string, username: string) => void;
  clearTyping: (channelId: string, userId: string) => void;
  setConnected: (connected: boolean) => void;
  incrementReconnectAttempts: () => void;
  resetReconnectAttempts: () => void;

  // Selectors
  getUsersInChannel: (channelId: string) => UserPresence[];
  getTypingUsers: (channelId: string) => TypingUser[];
  isUserOnline: (userId: string) => boolean;
}

export const usePresenceStore = create<PresenceState>((set, get) => ({
  presence: new Map(),
  typing: new Map(),
  isConnected: false,
  reconnectAttempts: 0,

  setPresence: (userId, presence) => {
    set(state => {
      const newPresence = new Map(state.presence);
      newPresence.set(userId, presence);
      return { presence: newPresence };
    });
  },

  removePresence: (userId) => {
    set(state => {
      const newPresence = new Map(state.presence);
      newPresence.delete(userId);
      return { presence: newPresence };
    });
  },

  setVoiceState: (userId, channelId, state = {}) => {
    set(s => {
      const existing = s.presence.get(userId);
      if (!existing) return s;

      const newPresence = new Map(s.presence);
      newPresence.set(userId, {
        ...existing,
        voiceChannelId: channelId || undefined,
        ...state,
      });
      return { presence: newPresence };
    });
  },

  setTyping: (channelId, userId, username) => {
    set(state => {
      const newTyping = new Map(state.typing);
      const channelTyping = newTyping.get(channelId) || [];

      // Update or add typing user
      const existingIndex = channelTyping.findIndex(t => t.userId === userId);
      const typingUser = { userId, username, channelId, startedAt: Date.now() };

      if (existingIndex >= 0) {
        channelTyping[existingIndex] = typingUser;
      } else {
        channelTyping.push(typingUser);
      }

      newTyping.set(channelId, channelTyping);
      return { typing: newTyping };
    });
  },

  clearTyping: (channelId, userId) => {
    set(state => {
      const newTyping = new Map(state.typing);
      const channelTyping = newTyping.get(channelId) || [];
      newTyping.set(channelId, channelTyping.filter(t => t.userId !== userId));
      return { typing: newTyping };
    });
  },

  setConnected: (isConnected) => set({ isConnected }),
  incrementReconnectAttempts: () => set(s => ({ reconnectAttempts: s.reconnectAttempts + 1 })),
  resetReconnectAttempts: () => set({ reconnectAttempts: 0 }),

  getUsersInChannel: (channelId) => {
    return Array.from(get().presence.values())
      .filter(p => p.voiceChannelId === channelId);
  },

  getTypingUsers: (channelId) => {
    return get().typing.get(channelId) || [];
  },

  isUserOnline: (userId) => {
    const presence = get().presence.get(userId);
    return presence?.status === 'online' || presence?.status === 'idle';
  },
}));
```

### 8.3 WebSocket Hook

```typescript
// src/hooks/useWebSocket.ts

import { useEffect, useRef, useCallback } from 'react';
import { useAuthStore } from '../store/authStore';
import { usePresenceStore } from '../store/presenceStore';

const WS_URL = import.meta.env.VITE_WS_URL || 'ws://localhost:8080/ws';
const RECONNECT_DELAYS = [1000, 2000, 5000, 10000, 30000]; // Exponential backoff

export function useWebSocket() {
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<NodeJS.Timeout>();

  const { token } = useAuthStore();
  const {
    setConnected,
    setPresence,
    removePresence,
    setVoiceState,
    setTyping,
    clearTyping,
    reconnectAttempts,
    incrementReconnectAttempts,
    resetReconnectAttempts,
  } = usePresenceStore();

  const connect = useCallback(() => {
    if (!token || wsRef.current?.readyState === WebSocket.OPEN) return;

    const ws = new WebSocket(WS_URL);
    wsRef.current = ws;

    ws.onopen = () => {
      console.log('WebSocket connected');
      // Authenticate
      ws.send(JSON.stringify({
        type: 'auth',
        payload: { token },
      }));
    };

    ws.onmessage = (event) => {
      const data = JSON.parse(event.data);
      handleMessage(data);
    };

    ws.onclose = (event) => {
      console.log('WebSocket closed', event.code);
      setConnected(false);

      // Reconnect with exponential backoff
      const delay = RECONNECT_DELAYS[Math.min(reconnectAttempts, RECONNECT_DELAYS.length - 1)];
      reconnectTimeoutRef.current = setTimeout(() => {
        incrementReconnectAttempts();
        connect();
      }, delay);
    };

    ws.onerror = (error) => {
      console.error('WebSocket error', error);
    };
  }, [token, reconnectAttempts]);

  const handleMessage = useCallback((event: any) => {
    switch (event.type) {
      case 'auth.success':
        setConnected(true);
        resetReconnectAttempts();
        break;

      case 'auth.error':
        console.error('WebSocket auth failed:', event.payload);
        break;

      case 'ping':
        wsRef.current?.send(JSON.stringify({ type: 'pong' }));
        break;

      case 'user.online':
      case 'user.idle':
      case 'presence.update':
        setPresence(event.payload.user_id, event.payload);
        break;

      case 'user.offline':
        removePresence(event.payload.user_id);
        break;

      case 'voice.join':
        setVoiceState(event.payload.user_id, event.payload.channel_id, {
          muted: event.payload.muted,
          deafened: event.payload.deafened,
          video: event.payload.video,
          streaming: event.payload.streaming,
        });
        break;

      case 'voice.leave':
        setVoiceState(event.payload.user_id, null);
        break;

      case 'voice.mute':
        setVoiceState(event.payload.user_id, undefined, { muted: event.payload.muted });
        break;

      case 'typing.start':
        setTyping(event.payload.channel_id, event.payload.user_id, event.payload.username);
        break;

      case 'typing.stop':
        clearTyping(event.payload.channel_id, event.payload.user_id);
        break;
    }
  }, [setPresence, removePresence, setVoiceState, setTyping, clearTyping]);

  const sendTyping = useCallback((channelId: string) => {
    wsRef.current?.send(JSON.stringify({
      type: 'typing.start',
      payload: { channel_id: channelId },
    }));
  }, []);

  useEffect(() => {
    connect();

    return () => {
      clearTimeout(reconnectTimeoutRef.current);
      wsRef.current?.close();
    };
  }, [connect]);

  return { sendTyping };
}
```

---

## 9. Audio & Video Controls

### 9.1 Voice Controls Component

```tsx
// src/components/voice/VoiceControls.tsx

import { Mic, MicOff, Headphones, HeadphoneOff, Video, VideoOff, Monitor, Phone } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useVoiceStore } from '@/store/voiceStore';
import { ConnectionQuality } from './ConnectionQuality';

export function VoiceControls() {
  const {
    isMuted,
    isDeafened,
    isVideoEnabled,
    isScreenSharing,
    toggleMute,
    toggleDeafen,
    toggleVideo,
    toggleScreenShare,
    currentChannelId,
    connectionQuality,
  } = useVoiceStore();

  const handleDisconnect = () => {
    // Call leave API and reset state
  };

  if (!currentChannelId) return null;

  return (
    <div className="flex items-center gap-2 p-3 bg-zinc-900 border-t border-zinc-800">
      <ConnectionQuality quality={connectionQuality} />

      <div className="flex-1 flex items-center justify-center gap-2">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant={isMuted ? "destructive" : "secondary"}
              size="icon"
              onClick={toggleMute}
              disabled={isDeafened}
            >
              {isMuted ? <MicOff className="h-4 w-4" /> : <Mic className="h-4 w-4" />}
            </Button>
          </TooltipTrigger>
          <TooltipContent>{isMuted ? 'Unmute' : 'Mute'}</TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant={isDeafened ? "destructive" : "secondary"}
              size="icon"
              onClick={toggleDeafen}
            >
              {isDeafened ? <HeadphoneOff className="h-4 w-4" /> : <Headphones className="h-4 w-4" />}
            </Button>
          </TooltipTrigger>
          <TooltipContent>{isDeafened ? 'Undeafen' : 'Deafen'}</TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant={isVideoEnabled ? "default" : "secondary"}
              size="icon"
              onClick={toggleVideo}
            >
              {isVideoEnabled ? <Video className="h-4 w-4" /> : <VideoOff className="h-4 w-4" />}
            </Button>
          </TooltipTrigger>
          <TooltipContent>{isVideoEnabled ? 'Turn off camera' : 'Turn on camera'}</TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant={isScreenSharing ? "default" : "secondary"}
              size="icon"
              onClick={toggleScreenShare}
            >
              <Monitor className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{isScreenSharing ? 'Stop sharing' : 'Share screen'}</TooltipContent>
        </Tooltip>
      </div>

      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="destructive" size="icon" onClick={handleDisconnect}>
            <Phone className="h-4 w-4 rotate-135" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Disconnect</TooltipContent>
      </Tooltip>
    </div>
  );
}
```

### 9.2 Audio Settings Dialog

```tsx
// src/components/voice/AudioSettings.tsx

import { useState, useEffect } from 'react';
import { Settings2, Mic, Volume2, Video } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { useVoiceStore } from '@/store/voiceStore';
import { useAudioDevices } from '@/hooks/useAudioDevices';

export function AudioSettings() {
  const {
    audioInputDevice,
    audioOutputDevice,
    videoInputDevice,
    inputMode,
    setAudioInputDevice,
    setAudioOutputDevice,
    setVideoInputDevice,
    setInputMode,
  } = useVoiceStore();

  const { audioInputs, audioOutputs, videoInputs, refresh } = useAudioDevices();

  useEffect(() => {
    refresh();
  }, []);

  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button variant="ghost" size="icon">
          <Settings2 className="h-4 w-4" />
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle>Audio & Video Settings</DialogTitle>
        </DialogHeader>

        <div className="space-y-6 py-4">
          {/* Input Mode */}
          <div className="space-y-2">
            <Label>Input Mode</Label>
            <RadioGroup
              value={inputMode}
              onValueChange={(v) => setInputMode(v as 'vad' | 'ptt')}
              className="flex gap-4"
            >
              <div className="flex items-center space-x-2">
                <RadioGroupItem value="vad" id="vad" />
                <Label htmlFor="vad">Voice Activity</Label>
              </div>
              <div className="flex items-center space-x-2">
                <RadioGroupItem value="ptt" id="ptt" />
                <Label htmlFor="ptt">Push to Talk</Label>
              </div>
            </RadioGroup>
          </div>

          {/* Audio Input */}
          <div className="space-y-2">
            <Label className="flex items-center gap-2">
              <Mic className="h-4 w-4" />
              Microphone
            </Label>
            <Select value={audioInputDevice || ''} onValueChange={setAudioInputDevice}>
              <SelectTrigger>
                <SelectValue placeholder="Select microphone" />
              </SelectTrigger>
              <SelectContent>
                {audioInputs.map(device => (
                  <SelectItem key={device.deviceId} value={device.deviceId}>
                    {device.label || 'Unknown Device'}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {/* TODO: Add input level meter */}
          </div>

          {/* Audio Output */}
          <div className="space-y-2">
            <Label className="flex items-center gap-2">
              <Volume2 className="h-4 w-4" />
              Speakers
            </Label>
            <Select value={audioOutputDevice || ''} onValueChange={setAudioOutputDevice}>
              <SelectTrigger>
                <SelectValue placeholder="Select speakers" />
              </SelectTrigger>
              <SelectContent>
                {audioOutputs.map(device => (
                  <SelectItem key={device.deviceId} value={device.deviceId}>
                    {device.label || 'Unknown Device'}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button variant="outline" size="sm" className="mt-2">
              Test Audio
            </Button>
          </div>

          {/* Video Input */}
          <div className="space-y-2">
            <Label className="flex items-center gap-2">
              <Video className="h-4 w-4" />
              Camera
            </Label>
            <Select value={videoInputDevice || ''} onValueChange={setVideoInputDevice}>
              <SelectTrigger>
                <SelectValue placeholder="Select camera" />
              </SelectTrigger>
              <SelectContent>
                {videoInputs.map(device => (
                  <SelectItem key={device.deviceId} value={device.deviceId}>
                    {device.label || 'Unknown Device'}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {/* TODO: Add camera preview */}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
```

### 9.3 Audio Devices Hook

```typescript
// src/hooks/useAudioDevices.ts

import { useState, useCallback } from 'react';

interface MediaDeviceInfo {
  deviceId: string;
  label: string;
  kind: MediaDeviceKind;
}

export function useAudioDevices() {
  const [audioInputs, setAudioInputs] = useState<MediaDeviceInfo[]>([]);
  const [audioOutputs, setAudioOutputs] = useState<MediaDeviceInfo[]>([]);
  const [videoInputs, setVideoInputs] = useState<MediaDeviceInfo[]>([]);

  const refresh = useCallback(async () => {
    try {
      // Request permission to access devices (needed for labels)
      await navigator.mediaDevices.getUserMedia({ audio: true, video: true });

      const devices = await navigator.mediaDevices.enumerateDevices();

      setAudioInputs(devices.filter(d => d.kind === 'audioinput'));
      setAudioOutputs(devices.filter(d => d.kind === 'audiooutput'));
      setVideoInputs(devices.filter(d => d.kind === 'videoinput'));
    } catch (err) {
      console.error('Failed to enumerate devices:', err);
    }
  }, []);

  return { audioInputs, audioOutputs, videoInputs, refresh };
}
```

---

## 10. Permissions & Moderation

### 10.1 Permission Model

Voice and video permissions are role-based at the Space level:

| Action | Owner | Admin | Member |
|--------|-------|-------|--------|
| Join voice channel | Yes | Yes | Yes |
| Use microphone | Yes | Yes | Yes |
| Use camera | Yes | Yes | Yes |
| Share screen | Yes | Yes | Yes |
| Server-mute others | Yes | Yes | No |
| Server-deafen others | Yes | Yes | No |
| Force-disconnect others | Yes | Yes | No |

### 10.2 Moderation Actions

Admins and owners can:
- **Server-mute**: Force a participant to be muted (they cannot unmute themselves)
- **Force-disconnect**: Remove a participant from the voice channel
- Both actions trigger audit log entries

### 10.3 Implementation

```go
// Moderation checks in handlers

func (h *VoiceHandler) MuteParticipant(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    actor := auth.UserFromContext(ctx)
    channelID := mux.Vars(r)["id"]
    targetUserID := mux.Vars(r)["userId"]

    // Get channel and verify voice type
    channel, err := h.channelService.GetChannel(ctx, channelID)
    if err != nil {
        errors.NotFound(w, r, "Channel")
        return
    }

    // Check actor has admin+ role
    role, err := h.channelService.GetMemberRole(ctx, actor.ID, channel.SpaceID)
    if err != nil || (role != "admin" && role != "owner") {
        errors.Forbidden(w, r)
        return
    }

    // Cannot mute yourself
    if actor.ID == targetUserID {
        errors.BadRequest(w, r, "Cannot mute yourself")
        return
    }

    // Cannot mute higher role
    targetRole, _ := h.channelService.GetMemberRole(ctx, targetUserID, channel.SpaceID)
    if !canModerate(role, targetRole) {
        errors.Forbidden(w, r)
        return
    }

    // Execute mute
    err = h.livekitService.MuteParticipant(ctx, channel.RoomName(), targetUserID, true)
    if err != nil {
        errors.InternalError(w, r)
        return
    }

    // Audit log
    h.auditService.Log(ctx, audit.Entry{
        ActorID:    actor.ID,
        Action:     "voice.mute",
        TargetType: "user",
        TargetID:   targetUserID,
        Metadata: map[string]any{
            "channel_id": channelID,
            "space_id":   channel.SpaceID,
        },
        IPAddress: r.RemoteAddr,
    })

    // Broadcast mute event
    h.presenceHub.PublishVoiceMute(ctx, channelID, targetUserID, true)

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"message": "Participant muted"})
}

func canModerate(actorRole, targetRole string) bool {
    roleWeight := map[string]int{"owner": 3, "admin": 2, "member": 1}
    return roleWeight[actorRole] > roleWeight[targetRole]
}
```

---

## 11. Infrastructure Updates

### 11.1 Updated Docker Compose

```yaml
# docker-compose.yml - additions for Phase 2

services:
  redoubt-api:
    # ... existing config ...
    environment:
      - LIVEKIT_HOST=http://livekit:7880
      - LIVEKIT_API_KEY=${LIVEKIT_API_KEY}
      - LIVEKIT_API_SECRET=${LIVEKIT_API_SECRET}
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
      livekit:
        condition: service_healthy

  livekit:
    image: livekit/livekit-server:v1.5
    command: --config /etc/livekit.yaml
    ports:
      - "7880:7880"
      - "7881:7881"
      - "7882:7882"
      - "50000-50100:50000-50100/udp"
    volumes:
      - ./config/livekit.yaml:/etc/livekit.yaml:ro
    environment:
      - LIVEKIT_KEYS=${LIVEKIT_API_KEY}:${LIVEKIT_API_SECRET}
    depends_on:
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:7880"]
      interval: 10s
      timeout: 5s
      retries: 3
    restart: unless-stopped
```

### 11.2 Updated Caddyfile

```
{$DOMAIN:localhost} {
    # API routes
    handle /api/v1/* {
        reverse_proxy redoubt-api:8080
    }

    handle /health {
        reverse_proxy redoubt-api:8080
    }

    # WebSocket for presence
    handle /ws {
        reverse_proxy redoubt-api:8080
    }

    # LiveKit webhook endpoint
    handle /livekit/webhook {
        reverse_proxy redoubt-api:8080
    }

    # LiveKit signaling (clients connect here)
    handle /livekit/* {
        uri strip_prefix /livekit
        reverse_proxy livekit:7880
    }

    # LiveKit RTC over TCP (fallback)
    handle /rtc/* {
        reverse_proxy livekit:7881
    }

    handle {
        respond "Redoubt API" 200
    }
}
```

### 11.3 Environment Variables

Add to `.env`:

```bash
# LiveKit
LIVEKIT_API_KEY=devkey
LIVEKIT_API_SECRET=secret_dev_key_change_in_production
LIVEKIT_URL=ws://localhost:7880
```

---

## 12. Configuration

### 12.1 Config File Additions

```yaml
# config/config.yaml additions

livekit:
  host: "http://livekit:7880"
  api_key: "${LIVEKIT_API_KEY}"
  api_secret: "${LIVEKIT_API_SECRET}"
  ws_url: "wss://${DOMAIN}/livekit"  # Client-facing URL
  webhook_path: "/livekit/webhook"

websocket:
  read_buffer_size: 1024
  write_buffer_size: 1024
  ping_interval: 30s
  pong_timeout: 60s
  max_message_size: 4096

presence:
  typing_timeout: 5s
  idle_timeout: 5m

voice:
  default_max_participants: 25
```

---

## 13. Testing Strategy

### 13.1 Unit Tests

Mock interfaces for isolated testing:

```go
// internal/livekit/mocks/room_service.go

type MockRoomService struct {
    Rooms        map[string]*livekit.Room
    Participants map[string][]*livekit.ParticipantInfo
}

func (m *MockRoomService) EnsureRoom(ctx context.Context, name string, maxParticipants uint32) (*livekit.Room, error) {
    if room, ok := m.Rooms[name]; ok {
        return room, nil
    }
    room := &livekit.Room{Name: name, MaxParticipants: maxParticipants}
    m.Rooms[name] = room
    return room, nil
}
```

### 13.2 WebSocket Tests

```go
// internal/presence/hub_test.go

func TestHub_BroadcastToSpace(t *testing.T) {
    hub := NewHub(slog.Default())

    // Create mock connections
    conn1 := newMockConnection("user1")
    conn2 := newMockConnection("user2")

    hub.Register(conn1)
    hub.Register(conn2)
    hub.SubscribeToSpace("user1", "space1")
    hub.SubscribeToSpace("user2", "space1")

    event := Event{
        Type:      EventTypeUserOnline,
        Timestamp: time.Now(),
        Payload:   UserPresencePayload{UserID: "user3", Status: StatusOnline},
    }

    hub.BroadcastToSpace("space1", event)

    // Assert both connections received the message
    assert.Len(t, conn1.received, 1)
    assert.Len(t, conn2.received, 1)
}
```

### 13.3 Manual Testing Checklist

- [ ] Join voice channel with single user
- [ ] Join voice channel with multiple users
- [ ] Audio works between participants
- [ ] Video works between participants
- [ ] Screen sharing works
- [ ] Mute/unmute works
- [ ] Deafen/undeafen works
- [ ] PTT mode works with global shortcuts
- [ ] VAD mode works
- [ ] Device selection works
- [ ] Connection quality indicator updates
- [ ] Server-mute works (admin)
- [ ] Force-disconnect works (admin)
- [ ] Presence updates when joining/leaving voice
- [ ] Typing indicators work
- [ ] WebSocket reconnects after network interruption
- [ ] LiveKit reconnects after network interruption
- [ ] Participant limit enforcement works

---

## 14. Implementation Tasks

### Milestone 1: LiveKit Infrastructure

- [ ] Add LiveKit to docker-compose.yml
- [ ] Create LiveKit configuration file
- [ ] Update Caddyfile for LiveKit routes
- [ ] Add LiveKit environment variables
- [ ] Verify LiveKit health check
- [ ] Test basic room creation via LiveKit API

### Milestone 2: Go API - LiveKit Integration

- [ ] Add `livekit/server-sdk-go` dependency
- [ ] Implement TokenService
- [ ] Implement RoomService
- [ ] Implement WebhookHandler
- [ ] Add LiveKit client to dependency injection
- [ ] Configure webhook endpoint in router

### Milestone 3: Database & API Extensions

- [ ] Write migration `0002_voice_channels.up.sql`
- [ ] Add sqlc queries for voice state
- [ ] Implement voice handler endpoints
- [ ] Wire up voice routes
- [ ] Add voice permissions checks
- [ ] Implement audit logging for moderation

### Milestone 4: WebSocket Presence

- [ ] Add `gorilla/websocket` dependency
- [ ] Implement WebSocket Hub
- [ ] Implement Connection handling
- [ ] Define event types and payloads
- [ ] Implement presence state management
- [ ] Implement typing indicators
- [ ] Wire up WebSocket upgrade endpoint
- [ ] Integrate with LiveKit webhooks

### Milestone 5: Tauri Client Setup

- [ ] Initialize Tauri project
- [ ] Set up React + TypeScript + Vite
- [ ] Configure Tailwind CSS
- [ ] Add shadcn/ui components
- [ ] Create project structure
- [ ] Implement basic routing

### Milestone 6: Client Authentication & Navigation

- [ ] Implement auth store (Zustand)
- [ ] Create login form
- [ ] Create registration form
- [ ] Implement JWT token handling
- [ ] Create main layout with sidebar
- [ ] Implement space list
- [ ] Implement channel list

### Milestone 7: Client Voice Integration

- [ ] Add `livekit-client` dependency
- [ ] Implement voice store
- [ ] Create useVoice hook
- [ ] Implement join/leave voice
- [ ] Create VoiceControls component
- [ ] Create ParticipantList component
- [ ] Implement mute/deafen/video toggles

### Milestone 8: Client Presence & WebSocket

- [ ] Implement presence store
- [ ] Create useWebSocket hook
- [ ] Handle presence events
- [ ] Handle voice events
- [ ] Implement typing indicators
- [ ] Show online/offline status

### Milestone 9: Audio Settings & Devices

- [ ] Implement device enumeration hook
- [ ] Create AudioSettings dialog
- [ ] Implement device selection
- [ ] Add input level meter
- [ ] Add output test button
- [ ] Implement camera preview

### Milestone 10: Global Shortcuts (Tauri)

- [ ] Add tauri-plugin-global-shortcut
- [ ] Implement PTT shortcut registration
- [ ] Implement mute toggle shortcut
- [ ] Implement deafen toggle shortcut
- [ ] Handle shortcut events in React
- [ ] Add keybind configuration UI

### Milestone 11: Testing

- [ ] Write LiveKit service unit tests
- [ ] Write WebSocket hub unit tests
- [ ] Write voice handler tests
- [ ] Write presence event tests
- [ ] Manual testing of all features
- [ ] Fix identified issues

### Milestone 12: Documentation

- [ ] Update OpenAPI spec with voice endpoints
- [ ] Document WebSocket event protocol
- [ ] Document LiveKit configuration
- [ ] Update README with Phase 2 features

---

## 15. Acceptance Criteria

### Voice & Video

- [ ] User can join a voice channel and hear other participants
- [ ] User can enable camera and be seen by other participants
- [ ] User can share screen
- [ ] Only one screen share active at a time per channel
- [ ] LiveKit handles quality adaptation automatically
- [ ] Token is generated per-request on join
- [ ] Room is created lazily on first join
- [ ] Room is destroyed when last participant leaves

### Audio Controls

- [ ] Mute/unmute works instantly
- [ ] Deafen stops all incoming audio and mutes outgoing
- [ ] PTT mode only transmits while key is held
- [ ] VAD mode transmits on voice activity
- [ ] User can switch between PTT and VAD
- [ ] Global shortcuts work outside app focus
- [ ] Device selection works for input/output/camera

### Permissions & Moderation

- [ ] All Space members can join voice channels
- [ ] Only admins/owners can server-mute
- [ ] Only admins/owners can force-disconnect
- [ ] Admins cannot moderate owners
- [ ] Moderation actions are audit logged

### Presence

- [ ] WebSocket connects and authenticates
- [ ] Online/offline status is broadcast
- [ ] Idle status after 5 minutes of inactivity
- [ ] Voice channel presence is broadcast on join/leave
- [ ] Mute/deafen state is broadcast
- [ ] Typing indicators appear and auto-clear
- [ ] WebSocket auto-reconnects with backoff

### Tauri Client

- [ ] Full shell UI with space/channel navigation
- [ ] Voice controls visible when in voice
- [ ] Participant list shows who's in voice
- [ ] Connection quality indicator shows self quality
- [ ] Text chat area shows placeholder (Phase 3)
- [ ] Audio settings dialog works
- [ ] Device selection with live preview

### Infrastructure

- [ ] LiveKit runs in Docker Compose
- [ ] LiveKit WebSocket accessible via Caddy
- [ ] LiveKit UDP ports exposed for media
- [ ] Webhook events flow to Go API
- [ ] All services have health checks

---

## Summary

Phase 2 adds real-time voice and video communication to Redoubt with:

- **Self-hosted LiveKit SFU** running in Docker Compose
- **Full voice + video + screen sharing** with adaptive quality
- **Comprehensive presence system** with online/offline/idle, voice presence, and typing indicators
- **Server-enforced moderation** for mute and disconnect
- **Tauri desktop client** with React + TypeScript, full shell UI, and global keyboard shortcuts
- **Both VAD and PTT** input modes supported
- **Full audio device selection** with preview

The architecture maintains simplicity while providing a real-time communication experience, all self-hosted on a single VPS.
