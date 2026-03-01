-- Add disabled_at column to users table (distinct from deleted_at soft delete)
ALTER TABLE users ADD COLUMN disabled_at TIMESTAMPTZ;
CREATE INDEX idx_users_disabled ON users(disabled_at) WHERE disabled_at IS NOT NULL;

-- Admin sessions table (separate from API JWT sessions)
CREATE TABLE admin_sessions (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token       VARCHAR(64) NOT NULL UNIQUE,
    ip_address  INET,
    user_agent  TEXT,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_admin_sessions_token ON admin_sessions(token);
CREATE INDEX idx_admin_sessions_expires ON admin_sessions(expires_at);
