package messages

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/redoubtapp/redoubt-api/internal/db/generated"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
)

// ReadStateService handles read state tracking.
type ReadStateService struct {
	queries *generated.Queries
	logger  *slog.Logger
}

// NewReadStateService creates a new read state service.
func NewReadStateService(queries *generated.Queries, logger *slog.Logger) *ReadStateService {
	return &ReadStateService{
		queries: queries,
		logger:  logger,
	}
}

// ReadState represents the read state for a channel.
type ReadState struct {
	LastReadAt        *time.Time `json:"last_read_at,omitempty"`
	LastReadMessageID *uuid.UUID `json:"last_read_message_id,omitempty"`
}

// ChannelUnread represents unread count for a channel.
type ChannelUnread struct {
	ChannelID   uuid.UUID `json:"channel_id"`
	UnreadCount int       `json:"unread_count"`
}

// MarkChannelAsRead marks a channel as read up to a specific message.
func (rs *ReadStateService) MarkChannelAsRead(ctx context.Context, channelID uuid.UUID, messageID *uuid.UUID, userID uuid.UUID, isInstanceAdmin bool) error {
	// Get channel to find space
	channel, err := rs.queries.GetChannelByID(ctx, pgtype.UUID{Bytes: channelID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return apperrors.ErrChannelNotFound
		}
		return err
	}

	spaceID := UUIDFromPgtype(channel.SpaceID)

	// Check membership
	if err := rs.checkSpacePermission(ctx, spaceID, userID, isInstanceAdmin, generated.MembershipRoleMember); err != nil {
		return err
	}

	// Get the timestamp to mark as read
	var lastReadAt pgtype.Timestamptz
	var lastReadMessageID pgtype.UUID

	if messageID != nil {
		// Get the message timestamp
		msg, err := rs.queries.GetMessageByID(ctx, pgtype.UUID{Bytes: *messageID, Valid: true})
		if err != nil {
			if err == pgx.ErrNoRows {
				return apperrors.ErrMessageNotFound
			}
			return err
		}
		lastReadAt = msg.CreatedAt
		lastReadMessageID = pgtype.UUID{Bytes: *messageID, Valid: true}
	} else {
		// Mark everything as read up to now
		lastReadAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}

	return rs.queries.UpdateReadState(ctx, generated.UpdateReadStateParams{
		UserID:            pgtype.UUID{Bytes: userID, Valid: true},
		SpaceID:           pgtype.UUID{Bytes: spaceID, Valid: true},
		LastReadAt:        lastReadAt,
		LastReadMessageID: lastReadMessageID,
	})
}

// GetUnreadCount returns the unread message count for a channel.
func (rs *ReadStateService) GetUnreadCount(ctx context.Context, channelID, userID uuid.UUID, isInstanceAdmin bool) (int, error) {
	// Get channel to find space
	channel, err := rs.queries.GetChannelByID(ctx, pgtype.UUID{Bytes: channelID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, apperrors.ErrChannelNotFound
		}
		return 0, err
	}

	spaceID := UUIDFromPgtype(channel.SpaceID)

	// Check membership
	if err := rs.checkSpacePermission(ctx, spaceID, userID, isInstanceAdmin, generated.MembershipRoleMember); err != nil {
		return 0, err
	}

	count, err := rs.queries.GetUnreadCount(ctx, generated.GetUnreadCountParams{
		ID:     pgtype.UUID{Bytes: channelID, Valid: true},
		UserID: pgtype.UUID{Bytes: userID, Valid: true},
	})
	if err != nil {
		return 0, err
	}

	return int(count), nil
}

// GetChannelUnreadCounts returns unread counts for all channels in a space.
func (rs *ReadStateService) GetChannelUnreadCounts(ctx context.Context, spaceID, userID uuid.UUID, isInstanceAdmin bool) ([]ChannelUnread, error) {
	// Check membership
	if err := rs.checkSpacePermission(ctx, spaceID, userID, isInstanceAdmin, generated.MembershipRoleMember); err != nil {
		return nil, err
	}

	rows, err := rs.queries.GetChannelUnreadCounts(ctx, generated.GetChannelUnreadCountsParams{
		UserID:  pgtype.UUID{Bytes: userID, Valid: true},
		SpaceID: pgtype.UUID{Bytes: spaceID, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	result := make([]ChannelUnread, len(rows))
	for i, row := range rows {
		unreadCount := 0
		if count, ok := row.UnreadCount.(int32); ok {
			unreadCount = int(count)
		} else if count, ok := row.UnreadCount.(int64); ok {
			unreadCount = int(count)
		}
		result[i] = ChannelUnread{
			ChannelID:   UUIDFromPgtype(row.ChannelID),
			UnreadCount: unreadCount,
		}
	}

	return result, nil
}

// GetReadState returns the read state for a user in a space.
func (rs *ReadStateService) GetReadState(ctx context.Context, spaceID, userID uuid.UUID, isInstanceAdmin bool) (*ReadState, error) {
	// Check membership
	if err := rs.checkSpacePermission(ctx, spaceID, userID, isInstanceAdmin, generated.MembershipRoleMember); err != nil {
		return nil, err
	}

	row, err := rs.queries.GetReadState(ctx, generated.GetReadStateParams{
		UserID:  pgtype.UUID{Bytes: userID, Valid: true},
		SpaceID: pgtype.UUID{Bytes: spaceID, Valid: true},
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return &ReadState{}, nil
		}
		return nil, err
	}

	result := &ReadState{}
	if row.LastReadAt.Valid {
		result.LastReadAt = &row.LastReadAt.Time
	}
	if row.LastReadMessageID.Valid {
		msgID := UUIDFromPgtype(row.LastReadMessageID)
		result.LastReadMessageID = &msgID
	}

	return result, nil
}

// checkSpacePermission verifies a user has the required role in a space.
func (rs *ReadStateService) checkSpacePermission(ctx context.Context, spaceID, userID uuid.UUID, isInstanceAdmin bool, minRole generated.MembershipRole) error {
	if isInstanceAdmin {
		return nil
	}

	role, err := rs.queries.GetUserSpaceRole(ctx, generated.GetUserSpaceRoleParams{
		UserID:  pgtype.UUID{Bytes: userID, Valid: true},
		SpaceID: pgtype.UUID{Bytes: spaceID, Valid: true},
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return apperrors.ErrForbidden
		}
		return err
	}

	if !hasMinRole(role, minRole) {
		return apperrors.ErrForbidden
	}

	return nil
}
