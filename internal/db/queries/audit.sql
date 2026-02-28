-- name: CreateAuditLog :one
INSERT INTO audit_logs (actor_id, action, target_type, target_id, metadata, ip_address)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListAuditLogsByActor :many
SELECT * FROM audit_logs
WHERE actor_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListAuditLogsByTarget :many
SELECT * FROM audit_logs
WHERE target_type = $1 AND target_id = $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: ListRecentAuditLogs :many
SELECT al.*, u.username as actor_username
FROM audit_logs al
INNER JOIN users u ON al.actor_id = u.id
ORDER BY al.created_at DESC
LIMIT $1 OFFSET $2;
