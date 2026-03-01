-- name: CreatePasswordReset :one
INSERT INTO password_resets (user_id, token, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetPasswordResetByToken :one
SELECT * FROM password_resets
WHERE token = $1
  AND expires_at > NOW()
  AND used_at IS NULL;

-- name: MarkPasswordResetUsed :exec
UPDATE password_resets
SET used_at = NOW()
WHERE id = $1;

-- name: DeleteUserPasswordResets :exec
DELETE FROM password_resets
WHERE user_id = $1;

-- name: CleanupExpiredPasswordResets :exec
DELETE FROM password_resets
WHERE expires_at < NOW();
