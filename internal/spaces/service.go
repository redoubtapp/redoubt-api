package spaces

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/redoubtapp/redoubt-api/internal/db/generated"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
)

// Service handles space-related business logic.
type Service struct {
	queries *generated.Queries
	logger  *slog.Logger
}

// NewService creates a new space service.
func NewService(queries *generated.Queries, logger *slog.Logger) *Service {
	return &Service{
		queries: queries,
		logger:  logger,
	}
}

// CreateSpaceRequest is the request for creating a space.
type CreateSpaceRequest struct {
	Name    string
	IconURL *string
	OwnerID uuid.UUID
}

// CreateSpaceResult is the result of creating a space.
type CreateSpaceResult struct {
	Space    generated.Space
	Channels []generated.Channel
}

// CreateSpace creates a new space with default channels.
func (s *Service) CreateSpace(ctx context.Context, req CreateSpaceRequest) (*CreateSpaceResult, error) {
	// Create the space
	var iconURL pgtype.Text
	if req.IconURL != nil {
		iconURL = pgtype.Text{String: *req.IconURL, Valid: true}
	}

	space, err := s.queries.CreateSpace(ctx, generated.CreateSpaceParams{
		Name:    req.Name,
		IconUrl: iconURL,
		OwnerID: pgtype.UUID{Bytes: req.OwnerID, Valid: true},
	})
	if err != nil {
		s.logger.Error("failed to create space", slog.String("error", err.Error()))
		return nil, err
	}

	// Create owner membership
	_, err = s.queries.CreateMembership(ctx, generated.CreateMembershipParams{
		UserID:  pgtype.UUID{Bytes: req.OwnerID, Valid: true},
		SpaceID: space.ID,
		Role:    generated.MembershipRoleOwner,
	})
	if err != nil {
		s.logger.Error("failed to create owner membership", slog.String("error", err.Error()))
		return nil, err
	}

	// Create default channels
	channels := make([]generated.Channel, 0, 2)

	// Create default text channel "general"
	textChannel, err := s.queries.CreateChannel(ctx, generated.CreateChannelParams{
		SpaceID:  space.ID,
		Name:     "general",
		Type:     generated.ChannelTypeText,
		Position: 0,
	})
	if err != nil {
		s.logger.Error("failed to create default text channel", slog.String("error", err.Error()))
		return nil, err
	}
	channels = append(channels, textChannel)

	// Create default voice channel "General"
	voiceChannel, err := s.queries.CreateChannel(ctx, generated.CreateChannelParams{
		SpaceID:  space.ID,
		Name:     "General",
		Type:     generated.ChannelTypeVoice,
		Position: 1,
	})
	if err != nil {
		s.logger.Error("failed to create default voice channel", slog.String("error", err.Error()))
		return nil, err
	}
	channels = append(channels, voiceChannel)

	return &CreateSpaceResult{
		Space:    space,
		Channels: channels,
	}, nil
}

// GetSpace returns a space by ID, checking membership.
func (s *Service) GetSpace(ctx context.Context, spaceID, userID uuid.UUID, isInstanceAdmin bool) (*generated.Space, error) {
	space, err := s.queries.GetSpaceByID(ctx, pgtype.UUID{Bytes: spaceID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrSpaceNotFound
		}
		return nil, err
	}

	// Instance admins can view any space
	if isInstanceAdmin {
		return &space, nil
	}

	// Check membership
	isMember, err := s.queries.IsUserSpaceMember(ctx, generated.IsUserSpaceMemberParams{
		UserID:  pgtype.UUID{Bytes: userID, Valid: true},
		SpaceID: pgtype.UUID{Bytes: spaceID, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	if !isMember {
		return nil, apperrors.ErrForbidden
	}

	return &space, nil
}

// ListUserSpaces returns all spaces the user is a member of.
func (s *Service) ListUserSpaces(ctx context.Context, userID uuid.UUID) ([]generated.Space, error) {
	return s.queries.ListUserSpaces(ctx, pgtype.UUID{Bytes: userID, Valid: true})
}

// UpdateSpaceRequest is the request for updating a space.
type UpdateSpaceRequest struct {
	SpaceID uuid.UUID
	Name    *string
	IconURL *string
}

// UpdateSpace updates a space.
func (s *Service) UpdateSpace(ctx context.Context, req UpdateSpaceRequest, userID uuid.UUID, isInstanceAdmin bool) (*generated.Space, error) {
	// Check permissions
	if err := s.checkSpacePermission(ctx, req.SpaceID, userID, isInstanceAdmin, generated.MembershipRoleAdmin); err != nil {
		return nil, err
	}

	var name pgtype.Text
	if req.Name != nil {
		name = pgtype.Text{String: *req.Name, Valid: true}
	}

	var iconURL pgtype.Text
	if req.IconURL != nil {
		iconURL = pgtype.Text{String: *req.IconURL, Valid: true}
	}

	space, err := s.queries.UpdateSpace(ctx, generated.UpdateSpaceParams{
		ID:      pgtype.UUID{Bytes: req.SpaceID, Valid: true},
		Name:    name,
		IconUrl: iconURL,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrSpaceNotFound
		}
		return nil, err
	}

	return &space, nil
}

// DeleteSpace soft-deletes a space.
func (s *Service) DeleteSpace(ctx context.Context, spaceID, userID uuid.UUID, isInstanceAdmin bool) error {
	// Check permissions - only owner or instance admin can delete
	if err := s.checkSpacePermission(ctx, spaceID, userID, isInstanceAdmin, generated.MembershipRoleOwner); err != nil {
		return err
	}

	return s.queries.SoftDeleteSpace(ctx, pgtype.UUID{Bytes: spaceID, Valid: true})
}

// ListSpaceMembers returns all members of a space.
func (s *Service) ListSpaceMembers(ctx context.Context, spaceID, userID uuid.UUID, isInstanceAdmin bool) ([]generated.ListSpaceMembersRow, error) {
	// Check if user is a member or instance admin
	if !isInstanceAdmin {
		isMember, err := s.queries.IsUserSpaceMember(ctx, generated.IsUserSpaceMemberParams{
			UserID:  pgtype.UUID{Bytes: userID, Valid: true},
			SpaceID: pgtype.UUID{Bytes: spaceID, Valid: true},
		})
		if err != nil {
			return nil, err
		}
		if !isMember {
			return nil, apperrors.ErrForbidden
		}
	}

	return s.queries.ListSpaceMembers(ctx, pgtype.UUID{Bytes: spaceID, Valid: true})
}

// KickMember removes a member from a space.
func (s *Service) KickMember(ctx context.Context, spaceID, targetUserID, actorUserID uuid.UUID, isInstanceAdmin bool) error {
	// Check actor permissions - need admin role
	if err := s.checkSpacePermission(ctx, spaceID, actorUserID, isInstanceAdmin, generated.MembershipRoleAdmin); err != nil {
		return err
	}

	// Get target's role
	targetRole, err := s.queries.GetUserSpaceRole(ctx, generated.GetUserSpaceRoleParams{
		UserID:  pgtype.UUID{Bytes: targetUserID, Valid: true},
		SpaceID: pgtype.UUID{Bytes: spaceID, Valid: true},
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return apperrors.ErrMembershipNotFound
		}
		return err
	}

	// Cannot kick owner
	if targetRole == generated.MembershipRoleOwner {
		return apperrors.ErrCannotKickOwner
	}

	// Get actor's role if not instance admin
	if !isInstanceAdmin {
		actorRole, err := s.queries.GetUserSpaceRole(ctx, generated.GetUserSpaceRoleParams{
			UserID:  pgtype.UUID{Bytes: actorUserID, Valid: true},
			SpaceID: pgtype.UUID{Bytes: spaceID, Valid: true},
		})
		if err != nil {
			return err
		}

		// Admins can only kick members, not other admins
		if actorRole == generated.MembershipRoleAdmin && targetRole == generated.MembershipRoleAdmin {
			return apperrors.ErrForbidden
		}
	}

	return s.queries.DeleteMembership(ctx, generated.DeleteMembershipParams{
		UserID:  pgtype.UUID{Bytes: targetUserID, Valid: true},
		SpaceID: pgtype.UUID{Bytes: spaceID, Valid: true},
	})
}

// ChangeMemberRole changes a member's role.
func (s *Service) ChangeMemberRole(ctx context.Context, spaceID, targetUserID, actorUserID uuid.UUID, newRole generated.MembershipRole, isInstanceAdmin bool) error {
	// Check actor permissions - need owner role
	if err := s.checkSpacePermission(ctx, spaceID, actorUserID, isInstanceAdmin, generated.MembershipRoleOwner); err != nil {
		return err
	}

	// Get target's current role
	currentRole, err := s.queries.GetUserSpaceRole(ctx, generated.GetUserSpaceRoleParams{
		UserID:  pgtype.UUID{Bytes: targetUserID, Valid: true},
		SpaceID: pgtype.UUID{Bytes: spaceID, Valid: true},
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return apperrors.ErrMembershipNotFound
		}
		return err
	}

	// Cannot change owner's role
	if currentRole == generated.MembershipRoleOwner {
		return apperrors.ErrCannotChangeOwner
	}

	// Cannot promote to owner
	if newRole == generated.MembershipRoleOwner {
		return apperrors.ErrCannotChangeOwner
	}

	return s.queries.UpdateMembershipRole(ctx, generated.UpdateMembershipRoleParams{
		UserID:  pgtype.UUID{Bytes: targetUserID, Valid: true},
		SpaceID: pgtype.UUID{Bytes: spaceID, Valid: true},
		Role:    newRole,
	})
}

// GetUserRole returns the user's role in a space.
func (s *Service) GetUserRole(ctx context.Context, spaceID, userID uuid.UUID) (generated.MembershipRole, error) {
	return s.queries.GetUserSpaceRole(ctx, generated.GetUserSpaceRoleParams{
		UserID:  pgtype.UUID{Bytes: userID, Valid: true},
		SpaceID: pgtype.UUID{Bytes: spaceID, Valid: true},
	})
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
