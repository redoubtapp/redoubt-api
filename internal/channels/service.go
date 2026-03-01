package channels

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/redoubtapp/redoubt-api/internal/db/generated"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
)

// Service handles channel-related business logic.
type Service struct {
	queries *generated.Queries
	logger  *slog.Logger
}

// NewService creates a new channel service.
func NewService(queries *generated.Queries, logger *slog.Logger) *Service {
	return &Service{
		queries: queries,
		logger:  logger,
	}
}

// CreateChannelRequest is the request for creating a channel.
type CreateChannelRequest struct {
	SpaceID         uuid.UUID
	Name            string
	Type            generated.ChannelType
	MaxParticipants *int32 // Only used for voice channels
}

// CreateChannel creates a new channel in a space.
func (s *Service) CreateChannel(ctx context.Context, req CreateChannelRequest, userID uuid.UUID, isInstanceAdmin bool) (*generated.Channel, error) {
	// Check permissions - need admin role
	if err := s.checkSpacePermission(ctx, req.SpaceID, userID, isInstanceAdmin, generated.MembershipRoleAdmin); err != nil {
		return nil, err
	}

	// Get next position
	maxPos, err := s.queries.GetMaxChannelPosition(ctx, pgtype.UUID{Bytes: req.SpaceID, Valid: true})
	if err != nil {
		return nil, err
	}

	position := int32(0)
	if maxPos != nil {
		if pos, ok := maxPos.(int64); ok {
			position = int32(pos) + 1 // #nosec G115 - position values are small integers
		}
	}

	var maxParticipants pgtype.Int4
	if req.MaxParticipants != nil {
		maxParticipants = pgtype.Int4{Int32: *req.MaxParticipants, Valid: true}
	}

	channel, err := s.queries.CreateChannel(ctx, generated.CreateChannelParams{
		SpaceID:         pgtype.UUID{Bytes: req.SpaceID, Valid: true},
		Name:            req.Name,
		Type:            req.Type,
		Position:        position,
		MaxParticipants: maxParticipants,
	})
	if err != nil {
		s.logger.Error("failed to create channel", slog.String("error", err.Error()))
		return nil, err
	}

	return &channel, nil
}

// GetChannel returns a channel by ID, checking membership.
func (s *Service) GetChannel(ctx context.Context, channelID, userID uuid.UUID, isInstanceAdmin bool) (*generated.Channel, error) {
	channel, err := s.queries.GetChannelByID(ctx, pgtype.UUID{Bytes: channelID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrChannelNotFound
		}
		return nil, err
	}

	// Check membership (just need to be a member)
	spaceID := UUIDFromPgtype(channel.SpaceID)
	if err := s.checkSpacePermission(ctx, spaceID, userID, isInstanceAdmin, generated.MembershipRoleMember); err != nil {
		return nil, err
	}

	return &channel, nil
}

// ListSpaceChannels returns all channels in a space.
func (s *Service) ListSpaceChannels(ctx context.Context, spaceID, userID uuid.UUID, isInstanceAdmin bool) ([]generated.Channel, error) {
	// Check membership
	if err := s.checkSpacePermission(ctx, spaceID, userID, isInstanceAdmin, generated.MembershipRoleMember); err != nil {
		return nil, err
	}

	return s.queries.ListSpaceChannels(ctx, pgtype.UUID{Bytes: spaceID, Valid: true})
}

// UpdateChannelRequest is the request for updating a channel.
type UpdateChannelRequest struct {
	ChannelID       uuid.UUID
	Name            *string
	Type            *generated.ChannelType
	MaxParticipants *int32
}

// UpdateChannel updates a channel.
func (s *Service) UpdateChannel(ctx context.Context, req UpdateChannelRequest, userID uuid.UUID, isInstanceAdmin bool) (*generated.Channel, error) {
	// Get channel to find space ID
	channel, err := s.queries.GetChannelByID(ctx, pgtype.UUID{Bytes: req.ChannelID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrChannelNotFound
		}
		return nil, err
	}

	// Check permissions - need admin role
	spaceID := UUIDFromPgtype(channel.SpaceID)
	if err := s.checkSpacePermission(ctx, spaceID, userID, isInstanceAdmin, generated.MembershipRoleAdmin); err != nil {
		return nil, err
	}

	var name pgtype.Text
	if req.Name != nil {
		name = pgtype.Text{String: *req.Name, Valid: true}
	}

	var channelType generated.NullChannelType
	if req.Type != nil {
		channelType = generated.NullChannelType{ChannelType: *req.Type, Valid: true}
	}

	var maxParticipants pgtype.Int4
	if req.MaxParticipants != nil {
		maxParticipants = pgtype.Int4{Int32: *req.MaxParticipants, Valid: true}
	}

	updatedChannel, err := s.queries.UpdateChannel(ctx, generated.UpdateChannelParams{
		ID:              pgtype.UUID{Bytes: req.ChannelID, Valid: true},
		Name:            name,
		Type:            channelType,
		MaxParticipants: maxParticipants,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrChannelNotFound
		}
		return nil, err
	}

	return &updatedChannel, nil
}

// DeleteChannel soft-deletes a channel.
func (s *Service) DeleteChannel(ctx context.Context, channelID, userID uuid.UUID, isInstanceAdmin bool) error {
	// Get channel to find space ID
	channel, err := s.queries.GetChannelByID(ctx, pgtype.UUID{Bytes: channelID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return apperrors.ErrChannelNotFound
		}
		return err
	}

	// Check permissions - need admin role
	spaceID := UUIDFromPgtype(channel.SpaceID)
	if err := s.checkSpacePermission(ctx, spaceID, userID, isInstanceAdmin, generated.MembershipRoleAdmin); err != nil {
		return err
	}

	return s.queries.SoftDeleteChannel(ctx, pgtype.UUID{Bytes: channelID, Valid: true})
}

// ReorderChannelsRequest is the request for reordering channels.
type ReorderChannelsRequest struct {
	SpaceID    uuid.UUID
	ChannelIDs []uuid.UUID // Ordered list of channel IDs
}

// ReorderChannels reorders channels in a space.
func (s *Service) ReorderChannels(ctx context.Context, req ReorderChannelsRequest, userID uuid.UUID, isInstanceAdmin bool) error {
	// Check permissions - need admin role
	if err := s.checkSpacePermission(ctx, req.SpaceID, userID, isInstanceAdmin, generated.MembershipRoleAdmin); err != nil {
		return err
	}

	// Update each channel's position
	for i, channelID := range req.ChannelIDs {
		// Verify channel belongs to the space
		spaceID, err := s.queries.GetSpaceIDByChannelID(ctx, pgtype.UUID{Bytes: channelID, Valid: true})
		if err != nil {
			if err == pgx.ErrNoRows {
				return apperrors.ErrChannelNotFound
			}
			return err
		}

		if UUIDFromPgtype(spaceID) != req.SpaceID {
			return apperrors.ErrForbidden
		}

		err = s.queries.UpdateChannelPosition(ctx, generated.UpdateChannelPositionParams{
			ID:       pgtype.UUID{Bytes: channelID, Valid: true},
			Position: int32(i), // #nosec G115 - i is bounded by slice length, won't overflow
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// checkSpacePermission verifies a user has the required role in a space.
func (s *Service) checkSpacePermission(ctx context.Context, spaceID, userID uuid.UUID, isInstanceAdmin bool, minRole generated.MembershipRole) error {
	// Instance admins always have permission
	if isInstanceAdmin {
		return nil
	}

	role, err := s.queries.GetUserSpaceRole(ctx, generated.GetUserSpaceRoleParams{
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

// hasMinRole checks if a role meets the minimum required role.
func hasMinRole(role, minRole generated.MembershipRole) bool {
	roleOrder := map[generated.MembershipRole]int{
		generated.MembershipRoleMember: 0,
		generated.MembershipRoleAdmin:  1,
		generated.MembershipRoleOwner:  2,
	}

	return roleOrder[role] >= roleOrder[minRole]
}

// UUIDFromPgtype converts a pgtype.UUID to a uuid.UUID.
func UUIDFromPgtype(id pgtype.UUID) uuid.UUID {
	if !id.Valid {
		return uuid.Nil
	}
	return id.Bytes
}
