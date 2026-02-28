package messages

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/redoubtapp/redoubt-api/internal/db/generated"
	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
	"github.com/redoubtapp/redoubt-api/internal/presence"
	"github.com/redoubtapp/redoubt-api/internal/ratelimit"
)

// Service handles message-related business logic.
type Service struct {
	queries     *generated.Queries
	logger      *slog.Logger
	hub         *presence.Hub
	rateLimiter *ratelimit.Limiter
	editWindow  time.Duration
}

// NewService creates a new message service.
func NewService(
	queries *generated.Queries,
	logger *slog.Logger,
	hub *presence.Hub,
	rateLimiter *ratelimit.Limiter,
	editWindow time.Duration,
) *Service {
	return &Service{
		queries:     queries,
		logger:      logger,
		hub:         hub,
		rateLimiter: rateLimiter,
		editWindow:  editWindow,
	}
}

// SendMessageRequest is the request for sending a message.
type SendMessageRequest struct {
	ChannelID uuid.UUID
	Content   string
	ThreadID  *uuid.UUID
	Nonce     string
}

// AuthorInfo contains minimal author information.
type AuthorInfo struct {
	ID        uuid.UUID
	Username  string
	AvatarURL *string
}

// AttachmentInfo contains attachment information for a message.
type AttachmentInfo struct {
	ID          uuid.UUID
	Filename    string
	ContentType string
	SizeBytes   int64
	IsImage     bool
}

// MessageWithAuthor combines a message with author information.
type MessageWithAuthor struct {
	Message     generated.Message
	Author      AuthorInfo
	Attachments []AttachmentInfo
}

// MessageListResult is the result of listing messages.
type MessageListResult struct {
	Messages   []MessageWithAuthor
	NextCursor string
	HasMore    bool
}

// Cursor represents a pagination cursor.
type Cursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

// SendMessage creates a new message in a channel.
func (s *Service) SendMessage(ctx context.Context, req SendMessageRequest, userID uuid.UUID, isInstanceAdmin bool) (*MessageWithAuthor, error) {
	// Validate content
	if err := ValidateContent(req.Content); err != nil {
		return nil, err
	}

	// Get channel to find space ID
	channel, err := s.queries.GetChannelByID(ctx, pgtype.UUID{Bytes: req.ChannelID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrChannelNotFound
		}
		return nil, err
	}

	spaceID := UUIDFromPgtype(channel.SpaceID)

	// Check membership
	if err := s.checkSpacePermission(ctx, spaceID, userID, isInstanceAdmin, generated.MembershipRoleMember); err != nil {
		return nil, err
	}

	// Check rate limit
	if s.rateLimiter != nil {
		result, err := s.rateLimiter.Check(ctx, ratelimit.ScopeMessageSend, userID.String())
		if err != nil {
			s.logger.Error("rate limiter error", slog.String("error", err.Error()))
			// Fail open
		} else if !result.Allowed {
			return nil, apperrors.ErrRateLimited
		}
	}

	// Handle thread
	var threadID pgtype.UUID
	if req.ThreadID != nil {
		threadID = pgtype.UUID{Bytes: *req.ThreadID, Valid: true}

		// Verify parent message exists and is in the same channel
		parentMsg, err := s.queries.GetMessageByID(ctx, threadID)
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil, apperrors.ErrMessageNotFound
			}
			return nil, err
		}

		// Ensure we're replying to the same channel
		if UUIDFromPgtype(parentMsg.ChannelID) != req.ChannelID {
			return nil, apperrors.ErrForbidden
		}

		// If parent has a thread_id, use that instead (flat threading)
		if parentMsg.ThreadID.Valid {
			threadID = parentMsg.ThreadID
		}

		// Mark parent as thread root if needed
		if !parentMsg.IsThreadRoot && !parentMsg.ThreadID.Valid {
			if err := s.queries.MarkAsThreadRoot(ctx, pgtype.UUID{Bytes: *req.ThreadID, Valid: true}); err != nil {
				s.logger.Error("failed to mark as thread root", slog.String("error", err.Error()))
			}
		}

		// Increment reply count on the thread root
		rootID := threadID
		if !parentMsg.ThreadID.Valid {
			rootID = pgtype.UUID{Bytes: *req.ThreadID, Valid: true}
		}
		if err := s.queries.IncrementReplyCount(ctx, rootID); err != nil {
			s.logger.Error("failed to increment reply count", slog.String("error", err.Error()))
		}
	}

	// Create message
	msg, err := s.queries.CreateMessage(ctx, generated.CreateMessageParams{
		ChannelID: pgtype.UUID{Bytes: req.ChannelID, Valid: true},
		AuthorID:  pgtype.UUID{Bytes: userID, Valid: true},
		Content:   strings.TrimSpace(req.Content),
		ThreadID:  threadID,
	})
	if err != nil {
		s.logger.Error("failed to create message", slog.String("error", err.Error()))
		return nil, err
	}

	// Get author info
	user, err := s.queries.GetUserByID(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		return nil, err
	}

	author := AuthorInfo{
		ID:       userID,
		Username: user.Username,
	}
	if user.AvatarUrl.Valid {
		author.AvatarURL = &user.AvatarUrl.String
	}

	result := &MessageWithAuthor{
		Message: msg,
		Author:  author,
	}

	// Broadcast via WebSocket
	if s.hub != nil {
		var threadIDStr *string
		if msg.ThreadID.Valid {
			tid := UUIDFromPgtype(msg.ThreadID).String()
			threadIDStr = &tid
		}

		s.hub.PublishMessage(spaceID.String(), presence.MessageCreatePayload{
			ID:        UUIDFromPgtype(msg.ID).String(),
			ChannelID: req.ChannelID.String(),
			Author: presence.UserBrief{
				ID:        userID.String(),
				Username:  user.Username,
				AvatarURL: author.AvatarURL,
			},
			Content:   msg.Content,
			ThreadID:  threadIDStr,
			CreatedAt: msg.CreatedAt.Time,
			Nonce:     req.Nonce,
		})
	}

	return result, nil
}

// EditMessage edits an existing message.
func (s *Service) EditMessage(ctx context.Context, messageID uuid.UUID, content string, userID uuid.UUID, isInstanceAdmin bool) (*generated.Message, error) {
	// Validate content
	if err := ValidateContent(content); err != nil {
		return nil, err
	}

	// Get message
	msg, err := s.queries.GetMessageByID(ctx, pgtype.UUID{Bytes: messageID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrMessageNotFound
		}
		return nil, err
	}

	// Check if user is the author (instance admin can't edit others' messages)
	if UUIDFromPgtype(msg.AuthorID) != userID {
		return nil, apperrors.ErrCannotEditOthers
	}

	// Check edit window
	if time.Since(msg.CreatedAt.Time) > s.editWindow {
		return nil, apperrors.ErrEditWindowExpired
	}

	// Check rate limit
	if s.rateLimiter != nil {
		result, err := s.rateLimiter.Check(ctx, ratelimit.ScopeMessageEdit, messageID.String())
		if err != nil {
			s.logger.Error("rate limiter error", slog.String("error", err.Error()))
		} else if !result.Allowed {
			return nil, apperrors.ErrRateLimited
		}
	}

	// Save edit history
	_, err = s.queries.CreateMessageEdit(ctx, generated.CreateMessageEditParams{
		MessageID:       pgtype.UUID{Bytes: messageID, Valid: true},
		PreviousContent: msg.Content,
		EditedBy:        pgtype.UUID{Bytes: userID, Valid: true},
	})
	if err != nil {
		s.logger.Error("failed to create edit history", slog.String("error", err.Error()))
	}

	// Update message
	updatedMsg, err := s.queries.UpdateMessageContent(ctx, generated.UpdateMessageContentParams{
		ID:      pgtype.UUID{Bytes: messageID, Valid: true},
		Content: strings.TrimSpace(content),
	})
	if err != nil {
		return nil, err
	}

	// Get space ID for broadcasting
	spaceID, err := s.queries.GetSpaceIDByMessageID(ctx, pgtype.UUID{Bytes: messageID, Valid: true})
	if err == nil && s.hub != nil {
		editCount, _ := s.queries.CountMessageEdits(ctx, pgtype.UUID{Bytes: messageID, Valid: true})
		s.hub.PublishMessageUpdate(UUIDFromPgtype(spaceID).String(), presence.MessageUpdatePayload{
			ID:        messageID.String(),
			ChannelID: UUIDFromPgtype(updatedMsg.ChannelID).String(),
			Content:   updatedMsg.Content,
			EditedAt:  updatedMsg.EditedAt.Time,
			EditCount: int(editCount),
		})
	}

	return &updatedMsg, nil
}

// DeleteMessage soft-deletes a message.
func (s *Service) DeleteMessage(ctx context.Context, messageID, userID uuid.UUID, isInstanceAdmin bool) error {
	// Get message
	msg, err := s.queries.GetMessageByID(ctx, pgtype.UUID{Bytes: messageID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return apperrors.ErrMessageNotFound
		}
		return err
	}

	// Get channel to find space
	channel, err := s.queries.GetChannelByID(ctx, msg.ChannelID)
	if err != nil {
		return err
	}

	spaceID := UUIDFromPgtype(channel.SpaceID)

	// Check permission - author can always delete, or need admin role
	authorID := UUIDFromPgtype(msg.AuthorID)
	if authorID != userID {
		if err := s.checkSpacePermission(ctx, spaceID, userID, isInstanceAdmin, generated.MembershipRoleAdmin); err != nil {
			return apperrors.ErrCannotDeleteOthers
		}
	}

	// Soft delete
	if err := s.queries.SoftDeleteMessage(ctx, pgtype.UUID{Bytes: messageID, Valid: true}); err != nil {
		return err
	}

	// Decrement reply count if this was a thread reply
	if msg.ThreadID.Valid {
		if err := s.queries.DecrementReplyCount(ctx, msg.ThreadID); err != nil {
			s.logger.Error("failed to decrement reply count", slog.String("error", err.Error()))
		}
	}

	// Broadcast via WebSocket
	if s.hub != nil {
		s.hub.PublishMessageDelete(spaceID.String(), presence.MessageDeletePayload{
			ID:        messageID.String(),
			ChannelID: UUIDFromPgtype(msg.ChannelID).String(),
		})
	}

	return nil
}

// GetMessage returns a single message.
func (s *Service) GetMessage(ctx context.Context, messageID, userID uuid.UUID, isInstanceAdmin bool) (*MessageWithAuthor, error) {
	row, err := s.queries.GetMessageWithAuthor(ctx, pgtype.UUID{Bytes: messageID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrMessageNotFound
		}
		return nil, err
	}

	// Check membership via channel -> space
	channel, err := s.queries.GetChannelByID(ctx, row.ChannelID)
	if err != nil {
		return nil, err
	}

	if err := s.checkSpacePermission(ctx, UUIDFromPgtype(channel.SpaceID), userID, isInstanceAdmin, generated.MembershipRoleMember); err != nil {
		return nil, err
	}

	result := &MessageWithAuthor{
		Message: generated.Message{
			ID:           row.ID,
			ChannelID:    row.ChannelID,
			AuthorID:     row.AuthorID,
			Content:      row.Content,
			ThreadID:     row.ThreadID,
			IsThreadRoot: row.IsThreadRoot,
			ReplyCount:   row.ReplyCount,
			EditedAt:     row.EditedAt,
			DeletedAt:    row.DeletedAt,
			CreatedAt:    row.CreatedAt,
		},
		Author: AuthorInfo{
			ID:       UUIDFromPgtype(row.AuthorID),
			Username: row.AuthorUsername,
		},
	}
	if row.AuthorAvatarUrl.Valid {
		result.Author.AvatarURL = &row.AuthorAvatarUrl.String
	}

	// Fetch attachments
	attachments, err := s.fetchAttachmentsForMessage(ctx, messageID)
	if err != nil {
		s.logger.Error("failed to fetch attachments", slog.String("error", err.Error()))
	} else {
		result.Attachments = attachments
	}

	return result, nil
}

// ListChannelMessages returns paginated messages for a channel.
func (s *Service) ListChannelMessages(ctx context.Context, channelID uuid.UUID, cursorStr string, limit int32, userID uuid.UUID, isInstanceAdmin bool) (*MessageListResult, error) {
	// Get channel to find space
	channel, err := s.queries.GetChannelByID(ctx, pgtype.UUID{Bytes: channelID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrChannelNotFound
		}
		return nil, err
	}

	// Check membership
	if err := s.checkSpacePermission(ctx, UUIDFromPgtype(channel.SpaceID), userID, isInstanceAdmin, generated.MembershipRoleMember); err != nil {
		return nil, err
	}

	if limit <= 0 || limit > 50 {
		limit = 50
	}

	var rows []generated.ListChannelMessagesRow
	if cursorStr == "" {
		rows, err = s.queries.ListChannelMessages(ctx, generated.ListChannelMessagesParams{
			ChannelID: pgtype.UUID{Bytes: channelID, Valid: true},
			Limit:     limit + 1, // Fetch one extra to check for more
		})
	} else {
		cursor, parseErr := decodeCursor(cursorStr)
		if parseErr != nil {
			return nil, apperrors.ErrInvalidInput
		}
		cursorRows, cursorErr := s.queries.ListChannelMessagesCursor(ctx, generated.ListChannelMessagesCursorParams{
			ChannelID: pgtype.UUID{Bytes: channelID, Valid: true},
			CreatedAt: pgtype.Timestamptz{Time: cursor.CreatedAt, Valid: true},
			ID:        pgtype.UUID{Bytes: cursor.ID, Valid: true},
			Limit:     limit + 1,
		})
		if cursorErr != nil {
			return nil, cursorErr
		}
		// Convert cursor rows to regular rows
		for _, r := range cursorRows {
			rows = append(rows, generated.ListChannelMessagesRow{
				ID:              r.ID,
				ChannelID:       r.ChannelID,
				AuthorID:        r.AuthorID,
				Content:         r.Content,
				ThreadID:        r.ThreadID,
				IsThreadRoot:    r.IsThreadRoot,
				ReplyCount:      r.ReplyCount,
				EditedAt:        r.EditedAt,
				DeletedAt:       r.DeletedAt,
				CreatedAt:       r.CreatedAt,
				AuthorUsername:  r.AuthorUsername,
				AuthorAvatarUrl: r.AuthorAvatarUrl,
			})
		}
		err = cursorErr
	}

	if err != nil {
		return nil, err
	}

	hasMore := len(rows) > int(limit)
	if hasMore {
		rows = rows[:limit]
	}

	messages := make([]MessageWithAuthor, len(rows))
	messageIDs := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		msgID := UUIDFromPgtype(row.ID)
		messageIDs[i] = msgID
		messages[i] = MessageWithAuthor{
			Message: generated.Message{
				ID:           row.ID,
				ChannelID:    row.ChannelID,
				AuthorID:     row.AuthorID,
				Content:      row.Content,
				ThreadID:     row.ThreadID,
				IsThreadRoot: row.IsThreadRoot,
				ReplyCount:   row.ReplyCount,
				EditedAt:     row.EditedAt,
				DeletedAt:    row.DeletedAt,
				CreatedAt:    row.CreatedAt,
			},
			Author: AuthorInfo{
				ID:       UUIDFromPgtype(row.AuthorID),
				Username: row.AuthorUsername,
			},
		}
		if row.AuthorAvatarUrl.Valid {
			messages[i].Author.AvatarURL = &row.AuthorAvatarUrl.String
		}
	}

	// Fetch attachments for all messages
	attachmentsMap, err := s.fetchAttachmentsForMessages(ctx, messageIDs)
	if err != nil {
		s.logger.Error("failed to fetch attachments", slog.String("error", err.Error()))
	} else {
		for i := range messages {
			msgID := UUIDFromPgtype(messages[i].Message.ID)
			messages[i].Attachments = attachmentsMap[msgID]
		}
	}

	var nextCursor string
	if hasMore && len(rows) > 0 {
		lastRow := rows[len(rows)-1]
		nextCursor = encodeCursor(Cursor{
			CreatedAt: lastRow.CreatedAt.Time,
			ID:        UUIDFromPgtype(lastRow.ID),
		})
	}

	return &MessageListResult{
		Messages:   messages,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// GetThreadReplies returns all replies in a thread.
func (s *Service) GetThreadReplies(ctx context.Context, parentID, userID uuid.UUID, isInstanceAdmin bool) ([]MessageWithAuthor, error) {
	// Get parent message to verify access
	msg, err := s.queries.GetMessageByID(ctx, pgtype.UUID{Bytes: parentID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrMessageNotFound
		}
		return nil, err
	}

	// Check membership via channel -> space
	channel, err := s.queries.GetChannelByID(ctx, msg.ChannelID)
	if err != nil {
		return nil, err
	}

	if err := s.checkSpacePermission(ctx, UUIDFromPgtype(channel.SpaceID), userID, isInstanceAdmin, generated.MembershipRoleMember); err != nil {
		return nil, err
	}

	rows, err := s.queries.GetThreadReplies(ctx, pgtype.UUID{Bytes: parentID, Valid: true})
	if err != nil {
		return nil, err
	}

	replies := make([]MessageWithAuthor, len(rows))
	messageIDs := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		msgID := UUIDFromPgtype(row.ID)
		messageIDs[i] = msgID
		replies[i] = MessageWithAuthor{
			Message: generated.Message{
				ID:           row.ID,
				ChannelID:    row.ChannelID,
				AuthorID:     row.AuthorID,
				Content:      row.Content,
				ThreadID:     row.ThreadID,
				IsThreadRoot: row.IsThreadRoot,
				ReplyCount:   row.ReplyCount,
				EditedAt:     row.EditedAt,
				DeletedAt:    row.DeletedAt,
				CreatedAt:    row.CreatedAt,
			},
			Author: AuthorInfo{
				ID:       UUIDFromPgtype(row.AuthorID),
				Username: row.AuthorUsername,
			},
		}
		if row.AuthorAvatarUrl.Valid {
			replies[i].Author.AvatarURL = &row.AuthorAvatarUrl.String
		}
	}

	// Fetch attachments for all replies
	attachmentsMap, err := s.fetchAttachmentsForMessages(ctx, messageIDs)
	if err != nil {
		s.logger.Error("failed to fetch attachments", slog.String("error", err.Error()))
	} else {
		for i := range replies {
			msgID := UUIDFromPgtype(replies[i].Message.ID)
			replies[i].Attachments = attachmentsMap[msgID]
		}
	}

	return replies, nil
}

// GetEditHistory returns the edit history for a message.
func (s *Service) GetEditHistory(ctx context.Context, messageID, userID uuid.UUID, isInstanceAdmin bool) ([]generated.MessageEdit, error) {
	// Get message to verify access
	msg, err := s.queries.GetMessageByID(ctx, pgtype.UUID{Bytes: messageID, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrMessageNotFound
		}
		return nil, err
	}

	// Only author or admin can view edit history
	authorID := UUIDFromPgtype(msg.AuthorID)
	if authorID != userID {
		channel, err := s.queries.GetChannelByID(ctx, msg.ChannelID)
		if err != nil {
			return nil, err
		}
		if err := s.checkSpacePermission(ctx, UUIDFromPgtype(channel.SpaceID), userID, isInstanceAdmin, generated.MembershipRoleAdmin); err != nil {
			return nil, apperrors.ErrForbidden
		}
	}

	return s.queries.GetMessageEditHistory(ctx, pgtype.UUID{Bytes: messageID, Valid: true})
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

// encodeCursor encodes a cursor to a base64 string.
func encodeCursor(c Cursor) string {
	data, _ := json.Marshal(c)
	return base64.URLEncoding.EncodeToString(data)
}

// decodeCursor decodes a base64 cursor string.
func decodeCursor(s string) (Cursor, error) {
	var c Cursor
	data, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(data, &c)
	return c, err
}

// isImageContentType checks if a content type is an image.
func isImageContentType(contentType string) bool {
	return strings.HasPrefix(contentType, "image/")
}

// fetchAttachmentsForMessage fetches attachments for a single message.
func (s *Service) fetchAttachmentsForMessage(ctx context.Context, messageID uuid.UUID) ([]AttachmentInfo, error) {
	rows, err := s.queries.GetMessageAttachments(ctx, pgtype.UUID{Bytes: messageID, Valid: true})
	if err != nil {
		return nil, err
	}

	attachments := make([]AttachmentInfo, len(rows))
	for i, row := range rows {
		attachments[i] = AttachmentInfo{
			ID:          UUIDFromPgtype(row.ID),
			Filename:    row.Filename,
			ContentType: row.ContentType,
			SizeBytes:   row.SizeBytes,
			IsImage:     isImageContentType(row.ContentType),
		}
	}
	return attachments, nil
}

// fetchAttachmentsForMessages fetches attachments for multiple messages.
func (s *Service) fetchAttachmentsForMessages(ctx context.Context, messageIDs []uuid.UUID) (map[uuid.UUID][]AttachmentInfo, error) {
	if len(messageIDs) == 0 {
		return make(map[uuid.UUID][]AttachmentInfo), nil
	}

	// Convert to pgtype.UUID slice
	pgIDs := make([]pgtype.UUID, len(messageIDs))
	for i, id := range messageIDs {
		pgIDs[i] = pgtype.UUID{Bytes: id, Valid: true}
	}

	rows, err := s.queries.GetMessageAttachmentsByMessages(ctx, pgIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[uuid.UUID][]AttachmentInfo)
	for _, row := range rows {
		msgID := UUIDFromPgtype(row.MessageID)
		result[msgID] = append(result[msgID], AttachmentInfo{
			ID:          UUIDFromPgtype(row.ID),
			Filename:    row.Filename,
			ContentType: row.ContentType,
			SizeBytes:   row.SizeBytes,
			IsImage:     isImageContentType(row.ContentType),
		})
	}
	return result, nil
}
