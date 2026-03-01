-- name: CreateSession :one
INSERT INTO sessions (user_id, refresh_token, user_agent, ip_address, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetSessionByRefreshToken :one
SELECT * FROM sessions
WHERE refresh_token = $1
  AND revoked_at IS NULL
  AND expires_at > NOW();

-- name: GetSessionByID :one
SELECT * FROM sessions
WHERE id = $1 AND revoked_at IS NULL;

-- name: ListUserSessions :many
SELECT * FROM sessions
WHERE user_id = $1
  AND revoked_at IS NULL
  AND expires_at > NOW()
ORDER BY last_used_at DESC;

-- name: UpdateSessionLastUsed :exec
UPDATE sessions
SET last_used_at = NOW()
WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeSession :exec
UPDATE sessions
SET revoked_at = NOW()
WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeAllUserSessions :exec
UPDATE sessions
SET revoked_at = NOW()
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: RevokeOtherUserSessions :exec
UPDATE sessions
SET revoked_at = NOW()
WHERE user_id = $1
  AND id != $2
  AND revoked_at IS NULL;

-- name: CleanupExpiredSessions :exec
DELETE FROM sessions
WHERE expires_at < NOW() - INTERVAL '30 days';

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions
WHERE expires_at < NOW();
