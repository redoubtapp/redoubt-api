-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Users table
CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username        VARCHAR(32) NOT NULL,
    email           VARCHAR(255) NOT NULL,
    password_hash   TEXT NOT NULL,
    avatar_url      TEXT,
    is_instance_admin BOOLEAN NOT NULL DEFAULT FALSE,
    email_verified  BOOLEAN NOT NULL DEFAULT FALSE,
    deleted_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT users_username_format CHECK (username ~ '^[a-zA-Z0-9_]{3,32}$'),
    CONSTRAINT users_email_lowercase CHECK (email = LOWER(email))
);

-- Partial unique indexes for soft-delete support
CREATE UNIQUE INDEX idx_users_username_unique ON users(username) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_users_email_unique ON users(email) WHERE deleted_at IS NULL;

-- Email verification tokens
CREATE TABLE email_verifications (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token       VARCHAR(64) NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_email_verifications_token ON email_verifications(token);
CREATE INDEX idx_email_verifications_expires ON email_verifications(expires_at);

-- Password reset tokens
CREATE TABLE password_resets (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token       VARCHAR(64) NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_password_resets_token ON password_resets(token);

-- Refresh tokens / Sessions
CREATE TABLE sessions (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token   VARCHAR(64) NOT NULL UNIQUE,
    user_agent      TEXT,
    ip_address      INET,
    last_used_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_refresh_token ON sessions(refresh_token);

-- Spaces
CREATE TABLE spaces (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name        VARCHAR(50) NOT NULL,
    icon_url    TEXT,
    owner_id    UUID NOT NULL REFERENCES users(id),
    deleted_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT spaces_name_format CHECK (name ~ '^[a-zA-Z0-9 ]{1,50}$')
);

CREATE INDEX idx_spaces_owner ON spaces(owner_id) WHERE deleted_at IS NULL;

-- Memberships
CREATE TYPE membership_role AS ENUM ('owner', 'admin', 'member');

CREATE TABLE memberships (
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    space_id    UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    role        membership_role NOT NULL DEFAULT 'member',
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (user_id, space_id)
);

CREATE INDEX idx_memberships_space ON memberships(space_id);

-- Channels
CREATE TYPE channel_type AS ENUM ('text', 'voice');

CREATE TABLE channels (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    space_id    UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    name        VARCHAR(50) NOT NULL,
    type        channel_type NOT NULL,
    position    INTEGER NOT NULL DEFAULT 0,
    deleted_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT channels_name_format CHECK (name ~ '^[a-zA-Z0-9 ]{1,50}$')
);

CREATE INDEX idx_channels_space ON channels(space_id) WHERE deleted_at IS NULL;

-- Invites
CREATE TABLE invites (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    code        VARCHAR(100) NOT NULL UNIQUE,
    space_id    UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    created_by  UUID NOT NULL REFERENCES users(id),
    uses        INTEGER NOT NULL DEFAULT 0,
    max_uses    INTEGER,  -- NULL = unlimited
    expires_at  TIMESTAMPTZ,  -- NULL = never
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_invites_code ON invites(code);
CREATE INDEX idx_invites_space ON invites(space_id);

-- Login attempts (for lockout tracking)
CREATE TABLE login_attempts (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email       VARCHAR(255) NOT NULL,
    ip_address  INET NOT NULL,
    success     BOOLEAN NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_login_attempts_email_time ON login_attempts(email, created_at DESC);
CREATE INDEX idx_login_attempts_ip_time ON login_attempts(ip_address, created_at DESC);

-- Bootstrap invite (system-generated on first run)
CREATE TABLE bootstrap_state (
    id              INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    initialized     BOOLEAN NOT NULL DEFAULT FALSE,
    invite_code     VARCHAR(100),
    initialized_at  TIMESTAMPTZ
);

INSERT INTO bootstrap_state (id, initialized) VALUES (1, FALSE);

-- Audit log (admin actions)
CREATE TABLE audit_logs (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    actor_id    UUID NOT NULL REFERENCES users(id),
    action      VARCHAR(50) NOT NULL,  -- e.g., 'member.kick', 'member.role_change', 'channel.delete'
    target_type VARCHAR(50) NOT NULL,  -- e.g., 'user', 'channel', 'space'
    target_id   UUID NOT NULL,
    metadata    JSONB,                  -- additional context
    ip_address  INET,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_actor ON audit_logs(actor_id);
CREATE INDEX idx_audit_logs_target ON audit_logs(target_type, target_id);
CREATE INDEX idx_audit_logs_created ON audit_logs(created_at DESC);

-- Updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER spaces_updated_at BEFORE UPDATE ON spaces
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER channels_updated_at BEFORE UPDATE ON channels
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
