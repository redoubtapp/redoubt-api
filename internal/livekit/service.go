package livekit

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"

	"github.com/redoubtapp/redoubt-api/internal/config"
)

// Service provides LiveKit room management and token generation.
type Service struct {
	roomClient *lksdk.RoomServiceClient
	apiKey     string
	apiSecret  string
	wsURL      string
	logger     *slog.Logger
}

// NewService creates a new LiveKit service.
func NewService(cfg config.LiveKitConfig, logger *slog.Logger) *Service {
	roomClient := lksdk.NewRoomServiceClient(cfg.Host, cfg.APIKey, cfg.APISecret)

	return &Service{
		roomClient: roomClient,
		apiKey:     cfg.APIKey,
		apiSecret:  cfg.APISecret,
		wsURL:      cfg.WebSocketURL,
		logger:     logger,
	}
}

// TokenOptions contains parameters for generating a LiveKit access token.
type TokenOptions struct {
	UserID         uuid.UUID
	Username       string
	RoomName       string
	CanPublish     bool
	CanSubscribe   bool
	CanPublishData bool
}

// GenerateToken creates a signed JWT for LiveKit room access.
func (s *Service) GenerateToken(opts TokenOptions) (string, error) {
	at := auth.NewAccessToken(s.apiKey, s.apiSecret)

	canPublish := opts.CanPublish
	canSubscribe := opts.CanSubscribe
	canPublishData := opts.CanPublishData

	grant := &auth.VideoGrant{
		Room:           opts.RoomName,
		RoomJoin:       true,
		CanPublish:     &canPublish,
		CanSubscribe:   &canSubscribe,
		CanPublishData: &canPublishData,
	}

	at.SetVideoGrant(grant).
		SetIdentity(opts.UserID.String()).
		SetName(opts.Username).
		SetValidFor(time.Hour) // 1 hour validity

	token, err := at.ToJWT()
	if err != nil {
		return "", fmt.Errorf("failed to generate LiveKit token: %w", err)
	}

	return token, nil
}

// WebSocketURL returns the client-facing WebSocket URL for LiveKit.
func (s *Service) WebSocketURL() string {
	return s.wsURL
}

// RoomName generates a consistent room name for a voice channel.
func RoomName(spaceID, channelID uuid.UUID) string {
	return fmt.Sprintf("space_%s_channel_%s", spaceID.String()[:8], channelID.String()[:8])
}

// EnsureRoom creates a room if it doesn't exist, or returns existing room info.
func (s *Service) EnsureRoom(ctx context.Context, name string, maxParticipants uint32) (*livekit.Room, error) {
	room, err := s.roomClient.CreateRoom(ctx, &livekit.CreateRoomRequest{
		Name:            name,
		EmptyTimeout:    0, // Destroy immediately when empty
		MaxParticipants: maxParticipants,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create/ensure room: %w", err)
	}

	s.logger.Debug("room ensured",
		slog.String("room_name", name),
		slog.Uint64("max_participants", uint64(maxParticipants)),
	)

	return room, nil
}

// GetRoom returns room info or nil if not found.
func (s *Service) GetRoom(ctx context.Context, name string) (*livekit.Room, error) {
	rooms, err := s.roomClient.ListRooms(ctx, &livekit.ListRoomsRequest{
		Names: []string{name},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list rooms: %w", err)
	}

	if len(rooms.Rooms) == 0 {
		return nil, nil
	}

	return rooms.Rooms[0], nil
}

// GetParticipants returns all participants in a room.
func (s *Service) GetParticipants(ctx context.Context, roomName string) ([]*livekit.ParticipantInfo, error) {
	resp, err := s.roomClient.ListParticipants(ctx, &livekit.ListParticipantsRequest{
		Room: roomName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list participants: %w", err)
	}

	return resp.Participants, nil
}

// GetParticipantCount returns the number of participants in a room.
func (s *Service) GetParticipantCount(ctx context.Context, roomName string) (int, error) {
	participants, err := s.GetParticipants(ctx, roomName)
	if err != nil {
		return 0, err
	}
	return len(participants), nil
}

// RemoveParticipant force-disconnects a participant from a room.
func (s *Service) RemoveParticipant(ctx context.Context, roomName, identity string) error {
	_, err := s.roomClient.RemoveParticipant(ctx, &livekit.RoomParticipantIdentity{
		Room:     roomName,
		Identity: identity,
	})
	if err != nil {
		return fmt.Errorf("failed to remove participant: %w", err)
	}

	s.logger.Info("participant removed",
		slog.String("room_name", roomName),
		slog.String("identity", identity),
	)

	return nil
}

// MuteParticipant server-side mutes/unmutes a participant's audio track.
func (s *Service) MuteParticipant(ctx context.Context, roomName, identity string, muted bool) error {
	_, err := s.roomClient.MutePublishedTrack(ctx, &livekit.MuteRoomTrackRequest{
		Room:     roomName,
		Identity: identity,
		Muted:    muted,
		// TrackSid left empty to mute all audio tracks
	})
	if err != nil {
		return fmt.Errorf("failed to mute participant: %w", err)
	}

	s.logger.Info("participant mute state changed",
		slog.String("room_name", roomName),
		slog.String("identity", identity),
		slog.Bool("muted", muted),
	)

	return nil
}

// DeleteRoom deletes a room and disconnects all participants.
func (s *Service) DeleteRoom(ctx context.Context, roomName string) error {
	_, err := s.roomClient.DeleteRoom(ctx, &livekit.DeleteRoomRequest{
		Room: roomName,
	})
	if err != nil {
		return fmt.Errorf("failed to delete room: %w", err)
	}

	s.logger.Info("room deleted", slog.String("room_name", roomName))

	return nil
}
