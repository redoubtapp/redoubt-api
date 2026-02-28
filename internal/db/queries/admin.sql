-- Admin Dashboard Queries

-- name: AdminCountUsers :one
SELECT COUNT(*) FROM users WHERE deleted_at IS NULL;

-- name: AdminCountSpaces :one
SELECT COUNT(*) FROM spaces WHERE deleted_at IS NULL;

-- name: AdminCountChannels :one
SELECT COUNT(*) FROM channels WHERE deleted_at IS NULL;

-- name: AdminCountMessagesToday :one
SELECT COUNT(*) FROM messages
WHERE created_at > NOW() - INTERVAL '24 hours'
  AND deleted_at IS NULL;

-- Admin User Management

-- name: AdminListUsers :many
SELECT id, username, email, avatar_url, is_instance_admin,
       email_verified, disabled_at, deleted_at, created_at
FROM users
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: AdminGetUser :one
SELECT * FROM users WHERE id = $1;

-- name: AdminDisableUser :exec
UPDATE users SET disabled_at = NOW(), updated_at = NOW()
WHERE id = $1 AND disabled_at IS NULL AND deleted_at IS NULL;

-- name: AdminEnableUser :exec
UPDATE users SET disabled_at = NULL, updated_at = NOW()
WHERE id = $1 AND disabled_at IS NOT NULL AND deleted_at IS NULL;

-- name: AdminGetUserSpaces :many
SELECT s.id, s.name, s.icon_url, m.role, m.joined_at
FROM memberships m
JOIN spaces s ON s.id = m.space_id
WHERE m.user_id = $1 AND s.deleted_at IS NULL
ORDER BY m.joined_at DESC;

-- name: AdminGetUserSessions :many
SELECT id, user_agent, ip_address, last_used_at, created_at
FROM sessions
WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > NOW()
ORDER BY last_used_at DESC;

-- Admin Space Management

-- name: AdminListSpaces :many
SELECT s.id, s.name, s.icon_url, s.owner_id, s.created_at,
       u.username AS owner_username,
       (SELECT COUNT(*) FROM memberships WHERE space_id = s.id) AS member_count,
       (SELECT COUNT(*) FROM channels WHERE space_id = s.id AND deleted_at IS NULL) AS channel_count
FROM spaces s
JOIN users u ON u.id = s.owner_id
WHERE s.deleted_at IS NULL
ORDER BY s.created_at DESC
LIMIT $1 OFFSET $2;

-- name: AdminGetSpaceDetail :one
SELECT s.id, s.name, s.icon_url, s.owner_id, s.created_at,
       u.username AS owner_username
FROM spaces s
JOIN users u ON u.id = s.owner_id
WHERE s.id = $1;

-- name: AdminGetSpaceMembers :many
SELECT u.id, u.username, u.email, u.avatar_url, m.role, m.joined_at
FROM memberships m
JOIN users u ON u.id = m.user_id
WHERE m.space_id = $1
ORDER BY m.role ASC, m.joined_at ASC;

-- name: AdminGetSpaceChannels :many
SELECT id, name, type, position, created_at
FROM channels
WHERE space_id = $1 AND deleted_at IS NULL
ORDER BY position ASC;

-- name: AdminGetSpaceInvites :many
SELECT i.id, i.code, i.uses, i.max_uses, i.expires_at, i.revoked_at,
       i.created_at, u.username AS created_by_username
FROM invites i
JOIN users u ON u.id = i.created_by
WHERE i.space_id = $1
ORDER BY i.created_at DESC;

-- Admin Audit Log

-- name: AdminListAuditLogs :many
SELECT a.id, a.action, a.target_type, a.target_id, a.metadata,
       a.ip_address, a.created_at,
       u.username AS actor_username
FROM audit_logs a
JOIN users u ON u.id = a.actor_id
ORDER BY a.created_at DESC
LIMIT $1 OFFSET $2;

-- name: AdminGetAuditLogEntry :one
SELECT a.id, a.actor_id, a.action, a.target_type, a.target_id,
       a.metadata, a.ip_address, a.created_at,
       u.username AS actor_username
FROM audit_logs a
JOIN users u ON u.id = a.actor_id
WHERE a.id = $1;

-- name: AdminCountAuditLogs :one
SELECT COUNT(*) FROM audit_logs;

-- Admin Session Management

-- name: CreateAdminSession :one
INSERT INTO admin_sessions (user_id, token, ip_address, user_agent, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetAdminSession :one
SELECT * FROM admin_sessions
WHERE token = $1 AND expires_at > NOW();

-- name: DeleteAdminSession :exec
DELETE FROM admin_sessions WHERE token = $1;

-- name: DeleteExpiredAdminSessions :exec
DELETE FROM admin_sessions WHERE expires_at < NOW();
