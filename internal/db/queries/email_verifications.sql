-- name: CreateEmailVerification :one
INSERT INTO email_verifications (user_id, token, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetEmailVerificationByToken :one
SELECT * FROM email_verifications
WHERE token = $1 AND expires_at > NOW();

-- name: DeleteEmailVerification :exec
DELETE FROM email_verifications
WHERE id = $1;

-- name: DeleteUserEmailVerifications :exec
DELETE FROM email_verifications
WHERE user_id = $1;

-- name: CleanupExpiredEmailVerifications :exec
DELETE FROM email_verifications
WHERE expires_at < NOW();
