-- name: CreateMembership :one
INSERT INTO memberships (user_id, space_id, role)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetMembership :one
SELECT * FROM memberships
WHERE user_id = $1 AND space_id = $2;

-- name: ListSpaceMembers :many
SELECT m.*, u.username, u.email, u.avatar_url
FROM memberships m
INNER JOIN users u ON m.user_id = u.id
WHERE m.space_id = $1 AND u.deleted_at IS NULL
ORDER BY m.role, u.username;

-- name: ListSpaceMembersPaginated :many
SELECT m.*, u.username, u.email, u.avatar_url
FROM memberships m
INNER JOIN users u ON m.user_id = u.id
WHERE m.space_id = $1 AND u.deleted_at IS NULL
ORDER BY m.joined_at DESC, m.user_id
LIMIT $2 OFFSET $3;

-- name: CountSpaceMembers :one
SELECT COUNT(*) FROM memberships m
INNER JOIN users u ON m.user_id = u.id
WHERE m.space_id = $1 AND u.deleted_at IS NULL;

-- name: UpdateMembershipRole :exec
UPDATE memberships
SET role = $3
WHERE user_id = $1 AND space_id = $2;

-- name: DeleteMembership :exec
DELETE FROM memberships
WHERE user_id = $1 AND space_id = $2;

-- name: IsUserSpaceMember :one
SELECT EXISTS (
    SELECT 1 FROM memberships
    WHERE user_id = $1 AND space_id = $2
);

-- name: GetUserSpaceRole :one
SELECT role FROM memberships
WHERE user_id = $1 AND space_id = $2;

-- name: UpdateReadState :exec
UPDATE memberships
SET last_read_at = $3,
    last_read_message_id = $4
WHERE user_id = $1 AND space_id = $2;

-- name: GetReadState :one
SELECT last_read_at, last_read_message_id
FROM memberships
WHERE user_id = $1 AND space_id = $2;

-- name: GetUnreadCount :one
SELECT COUNT(*)::int AS unread_count
FROM messages m
JOIN channels c ON c.id = m.channel_id
WHERE c.id = $1
  AND m.deleted_at IS NULL
  AND m.created_at > COALESCE(
    (SELECT last_read_at FROM memberships WHERE user_id = $2 AND space_id = c.space_id),
    '1970-01-01'::timestamptz
  );

-- name: GetChannelUnreadCounts :many
SELECT
    c.id AS channel_id,
    COALESCE(
        (SELECT COUNT(*)::int
         FROM messages m
         WHERE m.channel_id = c.id
           AND m.deleted_at IS NULL
           AND m.created_at > COALESCE(mem.last_read_at, '1970-01-01'::timestamptz)
        ), 0
    ) AS unread_count
FROM channels c
JOIN memberships mem ON mem.space_id = c.space_id AND mem.user_id = $1
WHERE c.space_id = $2
  AND c.type = 'text'
  AND c.deleted_at IS NULL;
