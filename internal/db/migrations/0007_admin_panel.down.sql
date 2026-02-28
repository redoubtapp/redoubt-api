DROP TABLE IF EXISTS admin_sessions;
DROP INDEX IF EXISTS idx_users_disabled;
ALTER TABLE users DROP COLUMN IF EXISTS disabled_at;
