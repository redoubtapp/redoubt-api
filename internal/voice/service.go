package voice

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/redoubtapp/redoubt-api/internal/db/generated"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
	"github.com/redoubtapp/redoubt-api/internal/livekit"
)

// EventPublisher broadcasts voice events to connected clients.
type EventPublisher interface {
	PublishVoiceJoin(ctx context.Context, channelID, userID string)
	PublishVoiceLeave(ctx context.Context, channelID, userID string)
}

// Service handles voice channel operations.
type Service struct {
	queries   *generated.Queries
	livekit   *livekit.Service
	publisher EventPublisher
	logger    *slog.Logger
	maxUsers  int // default max participants if not set on channel
}

// NewService creates a new voice service.
func NewService(queries *generated.Queries, livekitService *livekit.Service, publisher EventPublisher, logger *slog.Logger, defaultMaxUsers int) *Service {
	if defaultMaxUsers <= 0 {
		defaultMaxUsers = 25
	}
	return &Service{
		queries:   queries,
		livekit:   livekitService,
		publisher: publisher,
		logger:    logger,
		maxUsers:  defaultMaxUsers,
	}
}

// JoinResponse contains the data needed to connect to a voice channel.
type JoinResponse struct {
	Token        string `json:"token"`
	WebSocketURL string `json:"ws_url"`
	RoomName     string `json:"room_name"`
}

// VoiceParticipant represents a user in a voice channel.
type VoiceParticipant struct {
	UserID       uuid.UUID `json:"user_id"`
	Username     string    `json:"username"`
	AvatarURL    *string   `json:"avatar_url,omitempty"`
	SelfMuted    bool      `json:"self_muted"`
	SelfDeafened bool      `json:"self_deafened"`
	ServerMuted  bool      `json:"server_muted"`
	ConnectedAt  string    `json:"connected_at"`
}

// VoiceState represents the current voice state for a user.
type VoiceState struct {
	ChannelID    uuid.UUID `json:"channel_id"`
	SpaceID      uuid.UUID `json:"space_id"`
	SelfMuted    bool      `json:"self_muted"`
	SelfDeafened bool      `json:"self_deafened"`
	ServerMuted  bool      `json:"server_muted"`
}

// Join allows a user to join a voice channel.
func (s *Service) Join(ctx context.Context, channelID, userID uuid.UUID, isInstanceAdmin bool) (*JoinResponse, error) {
	// Get user info for username
	user, err := s.queries.GetUserByID(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrUserNotFound
		}
		return nil, err
	}
	username := user.Username

	// Get channel info
	channel, err := s.queries.GetChannelByID(ctx, pgtype.UUID{Bytes: channelID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrChannelNotFound
		}
		return nil, err
	}

	// Verify it's a voice channel
	if channel.Type != generated.ChannelTypeVoice {
		return nil, apperrors.ErrNotVoiceChannel
	}

	spaceID := uuidFromPgtype(channel.SpaceID)

	// Check membership
	if !isInstanceAdmin {
		_, err := s.queries.GetUserSpaceRole(ctx, generated.GetUserSpaceRoleParams{
			UserID:  pgtype.UUID{Bytes: userID, Valid: true},
			SpaceID: channel.SpaceID,
		})
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil, apperrors.ErrForbidden
			}
			return nil, err
		}
	}

	// Check if user is already in a voice channel - if so, leave it first
	existing, err := s.queries.GetVoiceConnectionByUserID(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err == nil && existing.ID.Valid {
		s.logger.Info("user has existing voice connection, cleaning up",
			slog.String("user_id", userID.String()),
			slog.String("old_channel", uuidFromPgtype(existing.ChannelID).String()),
			slog.String("new_channel", channelID.String()),
		)
		// Clean up the old connection (handles both rejoining same channel and switching)
		if err := s.Leave(ctx, userID); err != nil {
			s.logger.Warn("failed to leave previous voice channel",
				slog.String("error", err.Error()),
			)
			// Force delete the connection record to allow joining
			_ = s.queries.DeleteVoiceConnection(ctx, pgtype.UUID{Bytes: userID, Valid: true})
		}
	} else if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}

	// Check channel capacity
	maxParticipants := s.maxUsers
	if channel.MaxParticipants.Valid && channel.MaxParticipants.Int32 > 0 {
		maxParticipants = int(channel.MaxParticipants.Int32)
	}

	count, err := s.queries.CountVoiceConnectionsByChannelID(ctx, pgtype.UUID{Bytes: channelID, Valid: true})
	if err != nil {
		return nil, err
	}

	if int(count) >= maxParticipants {
		return nil, apperrors.ErrVoiceChannelFull
	}

	// Generate room name
	roomName := livekit.RoomName(spaceID, channelID)

	// Ensure room exists in LiveKit
	_, err = s.livekit.EnsureRoom(ctx, roomName, uint32(maxParticipants)) // #nosec G115 - maxParticipants is a small positive integer
	if err != nil {
		s.logger.Error("failed to ensure LiveKit room", slog.String("error", err.Error()))
		return nil, apperrors.ErrLiveKitUnavailable
	}

	// Generate LiveKit token
	token, err := s.livekit.GenerateToken(livekit.TokenOptions{
		UserID:         userID,
		Username:       username,
		RoomName:       roomName,
		CanPublish:     true,
		CanSubscribe:   true,
		CanPublishData: true,
	})
	if err != nil {
		s.logger.Error("failed to generate LiveKit token", slog.String("error", err.Error()))
		return nil, apperrors.ErrLiveKitUnavailable
	}

	// Record voice connection in database
	_, err = s.queries.CreateVoiceConnection(ctx, generated.CreateVoiceConnectionParams{
		UserID:      pgtype.UUID{Bytes: userID, Valid: true},
		ChannelID:   pgtype.UUID{Bytes: channelID, Valid: true},
		SpaceID:     pgtype.UUID{Bytes: spaceID, Valid: true},
		LivekitRoom: roomName,
	})
	if err != nil {
		s.logger.Error("failed to create voice connection record", slog.String("error", err.Error()))
		return nil, err
	}

	s.logger.Info("user joined voice channel",
		slog.String("user_id", userID.String()),
		slog.String("channel_id", channelID.String()),
		slog.String("room", roomName),
	)

	// Broadcast voice join event to connected clients
	if s.publisher != nil {
		s.publisher.PublishVoiceJoin(ctx, channelID.String(), userID.String())
	}

	return &JoinResponse{
		Token:        token,
		WebSocketURL: s.livekit.WebSocketURL(),
		RoomName:     roomName,
	}, nil
}

// Leave removes a user from their current voice channel.
func (s *Service) Leave(ctx context.Context, userID uuid.UUID) error {
	// Get current connection
	conn, err := s.queries.GetVoiceConnectionByUserID(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return apperrors.ErrNotInVoiceChannel
		}
		return err
	}

	// Save channel ID for broadcasting after deletion
	channelID := uuidFromPgtype(conn.ChannelID)

	// Remove from LiveKit
	if err := s.livekit.RemoveParticipant(ctx, conn.LivekitRoom, userID.String()); err != nil {
		s.logger.Warn("failed to remove participant from LiveKit",
			slog.String("error", err.Error()),
			slog.String("room", conn.LivekitRoom),
		)
		// Continue anyway - DB is source of truth
	}

	// Remove from database
	if err := s.queries.DeleteVoiceConnection(ctx, pgtype.UUID{Bytes: userID, Valid: true}); err != nil {
		return err
	}

	s.logger.Info("user left voice channel",
		slog.String("user_id", userID.String()),
		slog.String("room", conn.LivekitRoom),
	)

	// Broadcast voice leave event to connected clients
	if s.publisher != nil {
		s.publisher.PublishVoiceLeave(ctx, channelID.String(), userID.String())
	}

	return nil
}

// UpdateMuteStateRequest contains the mute state to update.
type UpdateMuteStateRequest struct {
	SelfMuted    *bool
	SelfDeafened *bool
}

// UpdateMuteState updates a user's self-mute/deafen state.
func (s *Service) UpdateMuteState(ctx context.Context, userID uuid.UUID, req UpdateMuteStateRequest) error {
	// Verify user is in a voice channel
	_, err := s.queries.GetVoiceConnectionByUserID(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return apperrors.ErrNotInVoiceChannel
		}
		return err
	}

	var selfMuted, selfDeafened pgtype.Bool
	if req.SelfMuted != nil {
		selfMuted = pgtype.Bool{Bool: *req.SelfMuted, Valid: true}
	}
	if req.SelfDeafened != nil {
		selfDeafened = pgtype.Bool{Bool: *req.SelfDeafened, Valid: true}
	}

	return s.queries.UpdateVoiceConnectionMuteState(ctx, generated.UpdateVoiceConnectionMuteStateParams{
		UserID:       pgtype.UUID{Bytes: userID, Valid: true},
		SelfMuted:    selfMuted,
		SelfDeafened: selfDeafened,
	})
}

// ServerMute allows an admin to server-mute another user.
func (s *Service) ServerMute(ctx context.Context, targetUserID, actorUserID uuid.UUID, muted bool, isInstanceAdmin bool) error {
	// Get target's voice connection
	conn, err := s.queries.GetVoiceConnectionByUserID(ctx, pgtype.UUID{Bytes: targetUserID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return apperrors.ErrNotInVoiceChannel
		}
		return err
	}

	// Cannot mute yourself
	if targetUserID == actorUserID {
		return apperrors.ErrCannotMuteSelf
	}

	// Check actor has permission (admin/owner in the space)
	if !isInstanceAdmin {
		actorRole, err := s.queries.GetUserSpaceRole(ctx, generated.GetUserSpaceRoleParams{
			UserID:  pgtype.UUID{Bytes: actorUserID, Valid: true},
			SpaceID: conn.SpaceID,
		})
		if err != nil {
			if err == pgx.ErrNoRows {
				return apperrors.ErrForbidden
			}
			return err
		}

		// Must be admin or owner
		if actorRole != generated.MembershipRoleAdmin && actorRole != generated.MembershipRoleOwner {
			return apperrors.ErrForbidden
		}

		// Check target's role - cannot mute higher role
		targetRole, err := s.queries.GetUserSpaceRole(ctx, generated.GetUserSpaceRoleParams{
			UserID:  pgtype.UUID{Bytes: targetUserID, Valid: true},
			SpaceID: conn.SpaceID,
		})
		if err != nil && err != pgx.ErrNoRows {
			return err
		}

		// Owner can mute anyone except instance admins (already checked)
		// Admin cannot mute owner or other admins
		if actorRole == generated.MembershipRoleAdmin {
			if targetRole == generated.MembershipRoleOwner || targetRole == generated.MembershipRoleAdmin {
				return apperrors.ErrCannotMuteHigherRole
			}
		}
	}

	// Update in LiveKit
	if err := s.livekit.MuteParticipant(ctx, conn.LivekitRoom, targetUserID.String(), muted); err != nil {
		s.logger.Warn("failed to mute participant in LiveKit",
			slog.String("error", err.Error()),
			slog.String("target", targetUserID.String()),
		)
		// Continue anyway
	}

	// Update in database
	return s.queries.UpdateVoiceConnectionMuteState(ctx, generated.UpdateVoiceConnectionMuteStateParams{
		UserID:      pgtype.UUID{Bytes: targetUserID, Valid: true},
		ServerMuted: pgtype.Bool{Bool: muted, Valid: true},
	})
}

// GetChannelParticipants returns all participants in a voice channel.
func (s *Service) GetChannelParticipants(ctx context.Context, channelID, userID uuid.UUID, isInstanceAdmin bool) ([]VoiceParticipant, error) {
	// Get channel to verify it exists and get space ID
	channel, err := s.queries.GetChannelByID(ctx, pgtype.UUID{Bytes: channelID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrChannelNotFound
		}
		return nil, err
	}

	// Check membership
	if !isInstanceAdmin {
		_, err := s.queries.GetUserSpaceRole(ctx, generated.GetUserSpaceRoleParams{
			UserID:  pgtype.UUID{Bytes: userID, Valid: true},
			SpaceID: channel.SpaceID,
		})
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil, apperrors.ErrForbidden
			}
			return nil, err
		}
	}

	connections, err := s.queries.GetVoiceConnectionsByChannelID(ctx, pgtype.UUID{Bytes: channelID, Valid: true})
	if err != nil {
		return nil, err
	}

	participants := make([]VoiceParticipant, 0, len(connections))
	for _, conn := range connections {
		var avatarURL *string
		if conn.AvatarUrl.Valid {
			avatarURL = &conn.AvatarUrl.String
		}

		participants = append(participants, VoiceParticipant{
			UserID:       uuidFromPgtype(conn.UserID),
			Username:     conn.Username,
			AvatarURL:    avatarURL,
			SelfMuted:    conn.SelfMuted,
			SelfDeafened: conn.SelfDeafened,
			ServerMuted:  conn.ServerMuted,
			ConnectedAt:  conn.ConnectedAt.Time.Format("2006-01-02T15:04:05Z"),
		})
	}

	return participants, nil
}

// GetUserVoiceState returns the current voice state for a user.
func (s *Service) GetUserVoiceState(ctx context.Context, userID uuid.UUID) (*VoiceState, error) {
	conn, err := s.queries.GetVoiceConnectionByUserID(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Not in a voice channel
		}
		return nil, err
	}

	return &VoiceState{
		ChannelID:    uuidFromPgtype(conn.ChannelID),
		SpaceID:      uuidFromPgtype(conn.SpaceID),
		SelfMuted:    conn.SelfMuted,
		SelfDeafened: conn.SelfDeafened,
		ServerMuted:  conn.ServerMuted,
	}, nil
}

// uuidFromPgtype converts a pgtype.UUID to a uuid.UUID.
func uuidFromPgtype(id pgtype.UUID) uuid.UUID {
	if !id.Valid {
		return uuid.Nil
	}
	return id.Bytes
}
