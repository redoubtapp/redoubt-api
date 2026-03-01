-- name: CreateLoginAttempt :exec
INSERT INTO login_attempts (email, ip_address, success)
VALUES ($1, $2, $3);

-- name: CountRecentFailedAttemptsByEmail :one
SELECT COUNT(*) FROM login_attempts
WHERE email = $1
  AND success = FALSE
  AND created_at > NOW() - INTERVAL '15 minutes';

-- name: CountRecentFailedAttemptsByIP :one
SELECT COUNT(*) FROM login_attempts
WHERE ip_address = $1
  AND success = FALSE
  AND created_at > NOW() - INTERVAL '15 minutes';

-- name: CountRecentFailedLoginAttempts :one
SELECT COUNT(*) FROM login_attempts
WHERE email = $1
  AND ip_address = $2
  AND success = FALSE
  AND created_at > $3;

-- name: CleanupOldLoginAttempts :exec
DELETE FROM login_attempts
WHERE created_at < NOW() - INTERVAL '24 hours';
