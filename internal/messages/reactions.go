package messages

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/redoubtapp/redoubt-api/internal/db/generated"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
	"github.com/redoubtapp/redoubt-api/internal/presence"
	"github.com/redoubtapp/redoubt-api/internal/ratelimit"
)

// ReactionService handles reaction-related business logic.
type ReactionService struct {
	queries     *generated.Queries
	logger      *slog.Logger
	hub         *presence.Hub
	rateLimiter *ratelimit.Limiter
}

// NewReactionService creates a new reaction service.
func NewReactionService(
	queries *generated.Queries,
	logger *slog.Logger,
	hub *presence.Hub,
	rateLimiter *ratelimit.Limiter,
) *ReactionService {
	return &ReactionService{
		queries:     queries,
		logger:      logger,
		hub:         hub,
		rateLimiter: rateLimiter,
	}
}

// ReactionGroup represents a group of reactions with the same emoji.
type ReactionGroup struct {
	Emoji      string   `json:"emoji"`
	Count      int      `json:"count"`
	Users      []string `json:"users"`
	HasReacted bool     `json:"has_reacted"`
}

// AddReaction adds a reaction to a message.
func (rs *ReactionService) AddReaction(ctx context.Context, messageID, userID uuid.UUID, emoji string, isInstanceAdmin bool) error {
	// Validate emoji is in curated set
	isValid, err := rs.queries.IsValidEmoji(ctx, emoji)
	if err != nil {
		return err
	}
	if !isValid {
		return apperrors.ErrInvalidEmoji
	}

	// Get message to verify access and get channel/space info
	msg, err := rs.queries.GetMessageByID(ctx, pgtype.UUID{Bytes: messageID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return apperrors.ErrMessageNotFound
		}
		return err
	}

	// Check membership via channel -> space
	channel, err := rs.queries.GetChannelByID(ctx, msg.ChannelID)
	if err != nil {
		return err
	}

	spaceID := UUIDFromPgtype(channel.SpaceID)
	if err := rs.checkSpacePermission(ctx, spaceID, userID, isInstanceAdmin, generated.MembershipRoleMember); err != nil {
		return err
	}

	// Check rate limit
	if rs.rateLimiter != nil {
		result, err := rs.rateLimiter.Check(ctx, ratelimit.ScopeReactionAdd, userID.String())
		if err != nil {
			rs.logger.Error("rate limiter error", slog.String("error", err.Error()))
		} else if !result.Allowed {
			return apperrors.ErrRateLimited
		}
	}

	// Add reaction (upsert - ignore if exists)
	if err := rs.queries.AddReaction(ctx, generated.AddReactionParams{
		MessageID: pgtype.UUID{Bytes: messageID, Valid: true},
		UserID:    pgtype.UUID{Bytes: userID, Valid: true},
		Emoji:     emoji,
	}); err != nil {
		return err
	}

	// Get username for broadcast
	user, err := rs.queries.GetUserByID(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		return err
	}

	// Broadcast via WebSocket
	if rs.hub != nil {
		rs.hub.PublishReaction(spaceID.String(), true, presence.ReactionPayload{
			MessageID: messageID.String(),
			ChannelID: UUIDFromPgtype(msg.ChannelID).String(),
			UserID:    userID.String(),
			Username:  user.Username,
			Emoji:     emoji,
		})
	}

	return nil
}

// RemoveReaction removes a reaction from a message.
func (rs *ReactionService) RemoveReaction(ctx context.Context, messageID, userID uuid.UUID, emoji string, isInstanceAdmin bool) error {
	// Get message to verify access and get channel/space info
	msg, err := rs.queries.GetMessageByID(ctx, pgtype.UUID{Bytes: messageID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return apperrors.ErrMessageNotFound
		}
		return err
	}

	// Check membership via channel -> space
	channel, err := rs.queries.GetChannelByID(ctx, msg.ChannelID)
	if err != nil {
		return err
	}

	spaceID := UUIDFromPgtype(channel.SpaceID)
	if err := rs.checkSpacePermission(ctx, spaceID, userID, isInstanceAdmin, generated.MembershipRoleMember); err != nil {
		return err
	}

	// Remove reaction
	if err := rs.queries.RemoveReaction(ctx, generated.RemoveReactionParams{
		MessageID: pgtype.UUID{Bytes: messageID, Valid: true},
		UserID:    pgtype.UUID{Bytes: userID, Valid: true},
		Emoji:     emoji,
	}); err != nil {
		return err
	}

	// Get username for broadcast
	user, err := rs.queries.GetUserByID(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		return err
	}

	// Broadcast via WebSocket
	if rs.hub != nil {
		rs.hub.PublishReaction(spaceID.String(), false, presence.ReactionPayload{
			MessageID: messageID.String(),
			ChannelID: UUIDFromPgtype(msg.ChannelID).String(),
			UserID:    userID.String(),
			Username:  user.Username,
			Emoji:     emoji,
		})
	}

	return nil
}

// GetMessageReactions returns all reactions for a message grouped by emoji.
func (rs *ReactionService) GetMessageReactions(ctx context.Context, messageID, userID uuid.UUID, isInstanceAdmin bool) ([]ReactionGroup, error) {
	// Get message to verify access
	msg, err := rs.queries.GetMessageByID(ctx, pgtype.UUID{Bytes: messageID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrMessageNotFound
		}
		return nil, err
	}

	// Check membership via channel -> space
	channel, err := rs.queries.GetChannelByID(ctx, msg.ChannelID)
	if err != nil {
		return nil, err
	}

	if err := rs.checkSpacePermission(ctx, UUIDFromPgtype(channel.SpaceID), userID, isInstanceAdmin, generated.MembershipRoleMember); err != nil {
		return nil, err
	}

	// Get all reactions for the message
	reactions, err := rs.queries.GetMessageReactions(ctx, pgtype.UUID{Bytes: messageID, Valid: true})
	if err != nil {
		return nil, err
	}

	// Group by emoji
	groups := make(map[string]*ReactionGroup)
	for _, r := range reactions {
		if g, ok := groups[r.Emoji]; ok {
			g.Count++
			g.Users = append(g.Users, r.Username)
			if UUIDFromPgtype(r.UserID) == userID {
				g.HasReacted = true
			}
		} else {
			groups[r.Emoji] = &ReactionGroup{
				Emoji:      r.Emoji,
				Count:      1,
				Users:      []string{r.Username},
				HasReacted: UUIDFromPgtype(r.UserID) == userID,
			}
		}
	}

	// Convert to slice
	result := make([]ReactionGroup, 0, len(groups))
	for _, g := range groups {
		result = append(result, *g)
	}

	return result, nil
}

// ToggleReaction adds a reaction if it doesn't exist, or removes it if it does.
func (rs *ReactionService) ToggleReaction(ctx context.Context, messageID, userID uuid.UUID, emoji string, isInstanceAdmin bool) (added bool, err error) {
	// Check if reaction already exists
	hasReacted, err := rs.queries.HasUserReacted(ctx, generated.HasUserReactedParams{
		MessageID: pgtype.UUID{Bytes: messageID, Valid: true},
		UserID:    pgtype.UUID{Bytes: userID, Valid: true},
		Emoji:     emoji,
	})
	if err != nil {
		return false, err
	}

	if hasReacted {
		return false, rs.RemoveReaction(ctx, messageID, userID, emoji, isInstanceAdmin)
	}

	return true, rs.AddReaction(ctx, messageID, userID, emoji, isInstanceAdmin)
}

// GetAllEmoji returns all curated emoji.
func (rs *ReactionService) GetAllEmoji(ctx context.Context) ([]generated.EmojiSet, error) {
	return rs.queries.GetAllEmoji(ctx)
}

// checkSpacePermission verifies a user has the required role in a space.
func (rs *ReactionService) checkSpacePermission(ctx context.Context, spaceID, userID uuid.UUID, isInstanceAdmin bool, minRole generated.MembershipRole) error {
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
