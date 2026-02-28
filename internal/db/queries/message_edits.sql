-- name: CreateMessageEdit :one
INSERT INTO message_edits (message_id, previous_content, edited_by)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetMessageEditHistory :many
SELECT * FROM message_edits
WHERE message_id = $1
ORDER BY created_at DESC;

-- name: CountRecentEdits :one
SELECT COUNT(*)::int FROM message_edits
WHERE message_id = $1
  AND created_at > NOW() - INTERVAL '1 minute';
