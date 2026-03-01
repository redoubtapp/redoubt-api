-- name: CreateMessage :one
INSERT INTO messages (channel_id, author_id, content, thread_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetMessageByID :one
SELECT * FROM messages
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetMessageWithAuthor :one
SELECT
    m.*,
    u.username AS author_username,
    u.avatar_url AS author_avatar_url
FROM messages m
JOIN users u ON u.id = m.author_id
WHERE m.id = $1 AND m.deleted_at IS NULL;

-- name: GetMessageIncludingDeleted :one
SELECT * FROM messages
WHERE id = $1;

-- name: ListChannelMessages :many
SELECT
    m.*,
    u.username AS author_username,
    u.avatar_url AS author_avatar_url
FROM messages m
JOIN users u ON u.id = m.author_id
WHERE m.channel_id = $1
  AND m.deleted_at IS NULL
  AND m.thread_id IS NULL
ORDER BY m.created_at DESC, m.id DESC
LIMIT $2;

-- name: ListChannelMessagesCursor :many
SELECT
    m.*,
    u.username AS author_username,
    u.avatar_url AS author_avatar_url
FROM messages m
JOIN users u ON u.id = m.author_id
WHERE m.channel_id = $1
  AND m.deleted_at IS NULL
  AND m.thread_id IS NULL
  AND (m.created_at < $2 OR (m.created_at = $2 AND m.id < $3))
ORDER BY m.created_at DESC, m.id DESC
LIMIT $4;

-- name: UpdateMessageContent :one
UPDATE messages
SET content = $2,
    edited_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteMessage :exec
UPDATE messages
SET deleted_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetThreadReplies :many
SELECT
    m.*,
    u.username AS author_username,
    u.avatar_url AS author_avatar_url
FROM messages m
JOIN users u ON u.id = m.author_id
WHERE m.thread_id = $1
  AND m.deleted_at IS NULL
ORDER BY m.created_at ASC;

-- name: GetThreadPreview :many
SELECT
    m.*,
    u.username AS author_username,
    u.avatar_url AS author_avatar_url
FROM messages m
JOIN users u ON u.id = m.author_id
WHERE m.thread_id = $1
  AND m.deleted_at IS NULL
ORDER BY m.created_at ASC
LIMIT 3;

-- name: MarkAsThreadRoot :exec
UPDATE messages
SET is_thread_root = TRUE
WHERE id = $1 AND deleted_at IS NULL;

-- name: IncrementReplyCount :exec
UPDATE messages
SET reply_count = reply_count + 1
WHERE id = $1;

-- name: DecrementReplyCount :exec
UPDATE messages
SET reply_count = GREATEST(reply_count - 1, 0)
WHERE id = $1;

-- name: GetChannelByMessageID :one
SELECT c.* FROM channels c
JOIN messages m ON m.channel_id = c.id
WHERE m.id = $1;

-- name: GetSpaceIDByMessageID :one
SELECT c.space_id FROM channels c
JOIN messages m ON m.channel_id = c.id
WHERE m.id = $1;

-- name: CountMessageEdits :one
SELECT COUNT(*)::int FROM message_edits
WHERE message_id = $1;

-- name: GetMessagesAfterTimestamp :many
SELECT * FROM messages
WHERE channel_id = $1
  AND deleted_at IS NULL
  AND created_at > $2
ORDER BY created_at ASC;

-- name: CountMessagesAfterTimestamp :one
SELECT COUNT(*)::int FROM messages
WHERE channel_id = $1
  AND deleted_at IS NULL
  AND created_at > $2;
