package invites

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

// Service handles invite-related business logic.
type Service struct {
	queries *generated.Queries
	logger  *slog.Logger
}

// NewService creates a new invite service.
func NewService(queries *generated.Queries, logger *slog.Logger) *Service {
	return &Service{
		queries: queries,
		logger:  logger,
	}
}

// CreateInviteRequest is the request for creating an invite.
type CreateInviteRequest struct {
	SpaceID   uuid.UUID
	CreatedBy uuid.UUID
	MaxUses   *int32
	ExpiresIn *time.Duration
}

// CreateInvite creates a new invite code for a space.
func (s *Service) CreateInvite(ctx context.Context, req CreateInviteRequest, userID uuid.UUID, isInstanceAdmin bool) (*generated.Invite, error) {
	// Check permissions - need admin role
	if err := s.checkSpacePermission(ctx, req.SpaceID, userID, isInstanceAdmin, generated.MembershipRoleAdmin); err != nil {
		return nil, err
	}

	// Generate invite code
	code, err := GenerateInviteCode()
	if err != nil {
		s.logger.Error("failed to generate invite code", slog.String("error", err.Error()))
		return nil, err
	}

	// Build parameters
	params := generated.CreateInviteParams{
		Code:      code,
		SpaceID:   pgtype.UUID{Bytes: req.SpaceID, Valid: true},
		CreatedBy: pgtype.UUID{Bytes: req.CreatedBy, Valid: true},
	}

	if req.MaxUses != nil {
		params.MaxUses = pgtype.Int4{Int32: *req.MaxUses, Valid: true}
	}

	if req.ExpiresIn != nil {
		expiresAt := time.Now().Add(*req.ExpiresIn)
		params.ExpiresAt = pgtype.Timestamptz{Time: expiresAt, Valid: true}
	}

	invite, err := s.queries.CreateInvite(ctx, params)
	if err != nil {
		s.logger.Error("failed to create invite", slog.String("error", err.Error()))
		return nil, err
	}

	return &invite, nil
}

// GetInviteInfo returns public invite information (for preview before joining).
func (s *Service) GetInviteInfo(ctx context.Context, code string) (*generated.GetInviteWithSpaceInfoRow, error) {
	invite, err := s.queries.GetInviteWithSpaceInfo(ctx, code)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrInviteNotFound
		}
		return nil, err
	}

	return &invite, nil
}

// JoinSpaceViaInvite allows a user to join a space using an invite code.
func (s *Service) JoinSpaceViaInvite(ctx context.Context, code string, userID uuid.UUID) (*generated.Space, error) {
	// Get and validate invite
	invite, err := s.queries.GetInviteByCode(ctx, code)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrInviteNotFound
		}
		return nil, err
	}

	// Check if invite is valid (these checks are also in the query, but double-check)
	if invite.RevokedAt.Valid {
		return nil, apperrors.ErrInviteRevoked
	}

	if invite.ExpiresAt.Valid && time.Now().After(invite.ExpiresAt.Time) {
		return nil, apperrors.ErrInviteExpired
	}

	if invite.MaxUses.Valid && invite.Uses >= invite.MaxUses.Int32 {
		return nil, apperrors.ErrInviteExhausted
	}

	// Check if user is already a member
	spaceID := UUIDFromPgtype(invite.SpaceID)
	isMember, err := s.queries.IsUserSpaceMember(ctx, generated.IsUserSpaceMemberParams{
		UserID:  pgtype.UUID{Bytes: userID, Valid: true},
		SpaceID: pgtype.UUID{Bytes: spaceID, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	if isMember {
		return nil, apperrors.ErrAlreadyMember
	}

	// Get the space to verify it still exists
	space, err := s.queries.GetSpaceByID(ctx, pgtype.UUID{Bytes: spaceID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrSpaceNotFound
		}
		return nil, err
	}

	// Create membership
	_, err = s.queries.CreateMembership(ctx, generated.CreateMembershipParams{
		UserID:  pgtype.UUID{Bytes: userID, Valid: true},
		SpaceID: pgtype.UUID{Bytes: spaceID, Valid: true},
		Role:    generated.MembershipRoleMember,
	})
	if err != nil {
		s.logger.Error("failed to create membership", slog.String("error", err.Error()))
		return nil, err
	}

	// Increment invite uses
	if err := s.queries.IncrementInviteUses(ctx, invite.ID); err != nil {
		s.logger.Error("failed to increment invite uses", slog.String("error", err.Error()))
		// Don't fail the join, just log
	}

	return &space, nil
}

// ListSpaceInvites returns all active invites for a space.
func (s *Service) ListSpaceInvites(ctx context.Context, spaceID, userID uuid.UUID, isInstanceAdmin bool) ([]generated.ListSpaceInvitesRow, error) {
	// Check permissions - need admin role
	if err := s.checkSpacePermission(ctx, spaceID, userID, isInstanceAdmin, generated.MembershipRoleAdmin); err != nil {
		return nil, err
	}

	return s.queries.ListSpaceInvites(ctx, pgtype.UUID{Bytes: spaceID, Valid: true})
}

// GetInviteByID returns an invite by its ID.
func (s *Service) GetInviteByID(ctx context.Context, inviteID, userID uuid.UUID, isInstanceAdmin bool) (*generated.Invite, error) {
	invite, err := s.queries.GetInviteByID(ctx, pgtype.UUID{Bytes: inviteID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrInviteNotFound
		}
		return nil, err
	}

	// Check permissions - need admin role for the space
	spaceID := UUIDFromPgtype(invite.SpaceID)
	if err := s.checkSpacePermission(ctx, spaceID, userID, isInstanceAdmin, generated.MembershipRoleAdmin); err != nil {
		return nil, err
	}

	return &invite, nil
}

// RevokeInvite revokes an invite.
func (s *Service) RevokeInvite(ctx context.Context, inviteID, userID uuid.UUID, isInstanceAdmin bool) error {
	// Get invite to find space ID
	invite, err := s.queries.GetInviteByID(ctx, pgtype.UUID{Bytes: inviteID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return apperrors.ErrInviteNotFound
		}
		return err
	}

	// Check permissions - need admin role for the space
	spaceID := UUIDFromPgtype(invite.SpaceID)
	if err := s.checkSpacePermission(ctx, spaceID, userID, isInstanceAdmin, generated.MembershipRoleAdmin); err != nil {
		return err
	}

	return s.queries.RevokeInvite(ctx, pgtype.UUID{Bytes: inviteID, Valid: true})
}

// CreateBootstrapInvite creates the initial bootstrap invite for registration.
// This is called on first run when no users exist.
func (s *Service) CreateBootstrapInvite(ctx context.Context) (string, error) {
	// Check if already initialized
	initialized, err := s.queries.IsBootstrapInitialized(ctx)
	if err != nil && err != pgx.ErrNoRows {
		return "", err
	}

	if initialized {
		// Return existing bootstrap code
		state, err := s.queries.GetBootstrapState(ctx)
		if err != nil {
			return "", err
		}
		if state.InviteCode.Valid {
			return state.InviteCode.String, nil
		}
		return "", nil
	}

	// Generate bootstrap invite code
	code, err := GenerateInviteCode()
	if err != nil {
		s.logger.Error("failed to generate bootstrap invite code", slog.String("error", err.Error()))
		return "", err
	}

	// Mark as initialized with the code
	if err := s.queries.SetBootstrapInitialized(ctx, pgtype.Text{String: code, Valid: true}); err != nil {
		s.logger.Error("failed to set bootstrap initialized", slog.String("error", err.Error()))
		return "", err
	}

	s.logger.Info("bootstrap invite code created", slog.String("code", code))
	return code, nil
}

// GetBootstrapInviteCode returns the bootstrap invite code if it exists.
func (s *Service) GetBootstrapInviteCode(ctx context.Context) (string, bool, error) {
	state, err := s.queries.GetBootstrapState(ctx)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}

	if !state.Initialized || !state.InviteCode.Valid {
		return "", false, nil
	}

	return state.InviteCode.String, true, nil
}

// IsBootstrapCode checks if a code is the bootstrap code.
func (s *Service) IsBootstrapCode(ctx context.Context, code string) (bool, error) {
	bootstrapCode, exists, err := s.GetBootstrapInviteCode(ctx)
	if err != nil {
		return false, err
	}

	if !exists {
		return false, nil
	}

	return code == bootstrapCode, nil
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
