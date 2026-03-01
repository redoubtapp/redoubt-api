package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/netip"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/redoubtapp/redoubt-api/internal/db/generated"
)

// Action types for audit logging.
const (
	ActionMemberKick       = "member.kick"
	ActionMemberRoleChange = "member.role_change"
	ActionChannelDelete    = "channel.delete"
	ActionChannelCreate    = "channel.create"
	ActionSpaceDelete      = "space.delete"
	ActionSpaceCreate      = "space.create"
	ActionSpaceUpdate      = "space.update"
	ActionInviteRevoke     = "invite.revoke"
	ActionInviteCreate     = "invite.create"
	ActionUserDelete       = "user.delete"
)

// TargetType types for audit logging.
const (
	TargetTypeUser    = "user"
	TargetTypeSpace   = "space"
	TargetTypeChannel = "channel"
	TargetTypeInvite  = "invite"
	TargetTypeMember  = "member"
)

// Service handles audit logging operations.
type Service struct {
	queries *generated.Queries
	logger  *slog.Logger
}

// NewService creates a new audit service.
func NewService(queries *generated.Queries, logger *slog.Logger) *Service {
	return &Service{
		queries: queries,
		logger:  logger,
	}
}

// LogEvent logs an audit event.
func (s *Service) LogEvent(ctx context.Context, actorID, targetID uuid.UUID, action, targetType string, metadata map[string]interface{}, ipAddress string) error {
	var metadataJSON []byte
	var err error
	if metadata != nil {
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			s.logger.Error("failed to marshal audit metadata", slog.String("error", err.Error()))
			metadataJSON = nil
		}
	}

	var ipAddr *netip.Addr
	if ipAddress != "" {
		addr, err := netip.ParseAddr(ipAddress)
		if err == nil {
			ipAddr = &addr
		}
	}

	_, err = s.queries.CreateAuditLog(ctx, generated.CreateAuditLogParams{
		ActorID:    pgtype.UUID{Bytes: actorID, Valid: true},
		Action:     action,
		TargetType: targetType,
		TargetID:   pgtype.UUID{Bytes: targetID, Valid: true},
		Metadata:   metadataJSON,
		IpAddress:  ipAddr,
	})
	if err != nil {
		s.logger.Error("failed to create audit log",
			slog.String("action", action),
			slog.String("error", err.Error()),
		)
		return err
	}

	s.logger.Info("audit event logged",
		slog.String("action", action),
		slog.String("target_type", targetType),
		slog.String("actor_id", actorID.String()),
		slog.String("target_id", targetID.String()),
	)

	return nil
}

// LogMemberKick logs a member kick event.
func (s *Service) LogMemberKick(ctx context.Context, actorID, kickedUserID, spaceID uuid.UUID, ipAddress string) error {
	return s.LogEvent(ctx, actorID, kickedUserID, ActionMemberKick, TargetTypeMember, map[string]interface{}{
		"space_id": spaceID.String(),
	}, ipAddress)
}

// LogMemberRoleChange logs a member role change event.
func (s *Service) LogMemberRoleChange(ctx context.Context, actorID, userID, spaceID uuid.UUID, oldRole, newRole string, ipAddress string) error {
	return s.LogEvent(ctx, actorID, userID, ActionMemberRoleChange, TargetTypeMember, map[string]interface{}{
		"space_id": spaceID.String(),
		"old_role": oldRole,
		"new_role": newRole,
	}, ipAddress)
}

// LogChannelDelete logs a channel delete event.
func (s *Service) LogChannelDelete(ctx context.Context, actorID, channelID, spaceID uuid.UUID, channelName string, ipAddress string) error {
	return s.LogEvent(ctx, actorID, channelID, ActionChannelDelete, TargetTypeChannel, map[string]interface{}{
		"space_id":     spaceID.String(),
		"channel_name": channelName,
	}, ipAddress)
}

// LogChannelCreate logs a channel create event.
func (s *Service) LogChannelCreate(ctx context.Context, actorID, channelID, spaceID uuid.UUID, channelName string, ipAddress string) error {
	return s.LogEvent(ctx, actorID, channelID, ActionChannelCreate, TargetTypeChannel, map[string]interface{}{
		"space_id":     spaceID.String(),
		"channel_name": channelName,
	}, ipAddress)
}

// LogSpaceDelete logs a space delete event.
func (s *Service) LogSpaceDelete(ctx context.Context, actorID, spaceID uuid.UUID, spaceName string, ipAddress string) error {
	return s.LogEvent(ctx, actorID, spaceID, ActionSpaceDelete, TargetTypeSpace, map[string]interface{}{
		"space_name": spaceName,
	}, ipAddress)
}

// LogSpaceCreate logs a space create event.
func (s *Service) LogSpaceCreate(ctx context.Context, actorID, spaceID uuid.UUID, spaceName string, ipAddress string) error {
	return s.LogEvent(ctx, actorID, spaceID, ActionSpaceCreate, TargetTypeSpace, map[string]interface{}{
		"space_name": spaceName,
	}, ipAddress)
}

// LogInviteRevoke logs an invite revoke event.
func (s *Service) LogInviteRevoke(ctx context.Context, actorID, inviteID, spaceID uuid.UUID, inviteCode string, ipAddress string) error {
	return s.LogEvent(ctx, actorID, inviteID, ActionInviteRevoke, TargetTypeInvite, map[string]interface{}{
		"space_id":    spaceID.String(),
		"invite_code": inviteCode,
	}, ipAddress)
}

// LogInviteCreate logs an invite create event.
func (s *Service) LogInviteCreate(ctx context.Context, actorID, inviteID, spaceID uuid.UUID, inviteCode string, ipAddress string) error {
	return s.LogEvent(ctx, actorID, inviteID, ActionInviteCreate, TargetTypeInvite, map[string]interface{}{
		"space_id":    spaceID.String(),
		"invite_code": inviteCode,
	}, ipAddress)
}

// LogUserDelete logs a user delete event.
func (s *Service) LogUserDelete(ctx context.Context, actorID, userID uuid.UUID, username string, ipAddress string) error {
	return s.LogEvent(ctx, actorID, userID, ActionUserDelete, TargetTypeUser, map[string]interface{}{
		"username": username,
	}, ipAddress)
}
