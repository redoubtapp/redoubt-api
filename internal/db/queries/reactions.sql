-- name: AddReaction :exec
INSERT INTO reactions (message_id, user_id, emoji)
VALUES ($1, $2, $3)
ON CONFLICT (message_id, user_id, emoji) DO NOTHING;

-- name: RemoveReaction :exec
DELETE FROM reactions
WHERE message_id = $1 AND user_id = $2 AND emoji = $3;

-- name: GetMessageReactions :many
SELECT
    r.emoji,
    r.user_id,
    u.username,
    r.created_at
FROM reactions r
JOIN users u ON u.id = r.user_id
WHERE r.message_id = $1
ORDER BY r.created_at ASC;

-- name: GetReactionCounts :many
SELECT emoji, COUNT(*)::int AS count
FROM reactions
WHERE message_id = $1
GROUP BY emoji
ORDER BY count DESC;

-- name: HasUserReacted :one
SELECT EXISTS (
    SELECT 1 FROM reactions
    WHERE message_id = $1 AND user_id = $2 AND emoji = $3
);

-- name: CountUserReactionsInWindow :one
SELECT COUNT(*)::int FROM reactions
WHERE user_id = $1
  AND created_at > NOW() - INTERVAL '1 minute';

-- name: GetUserReactionsForMessage :many
SELECT emoji FROM reactions
WHERE message_id = $1 AND user_id = $2;
