-- name: UpdateUserPresence :exec
UPDATE users
SET presence = $2,
    last_seen_at = NOW(),
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserPresence :one
SELECT id, username, avatar_url, presence, last_seen_at
FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUsersPresenceByIDs :many
SELECT id, username, avatar_url, presence, last_seen_at
FROM users
WHERE id = ANY($1::uuid[]) AND deleted_at IS NULL;

-- name: GetSpaceMembersPresence :many
SELECT u.id, u.username, u.avatar_url, u.presence, u.last_seen_at
FROM users u
JOIN memberships m ON u.id = m.user_id
WHERE m.space_id = $1 AND u.deleted_at IS NULL
ORDER BY u.presence, u.username;

-- name: SetUsersOfflineByLastSeen :exec
UPDATE users
SET presence = 'offline',
    updated_at = NOW()
WHERE last_seen_at < $1 AND presence != 'offline' AND deleted_at IS NULL;
