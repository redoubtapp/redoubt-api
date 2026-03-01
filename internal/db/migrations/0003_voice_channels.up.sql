-- Add max_participants to channels for voice channel capacity
ALTER TABLE channels ADD COLUMN max_participants INTEGER DEFAULT 25;

-- Presence status enum for users
CREATE TYPE presence_status AS ENUM ('online', 'idle', 'dnd', 'offline');

-- Add presence columns to users
ALTER TABLE users ADD COLUMN presence presence_status NOT NULL DEFAULT 'offline';
ALTER TABLE users ADD COLUMN last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- WebSocket connections tracking (for presence and notifications)
CREATE TABLE ws_connections (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    connection_id   VARCHAR(64) NOT NULL UNIQUE,
    server_id       VARCHAR(64) NOT NULL,  -- for horizontal scaling
    connected_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_ping_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ws_connections_user ON ws_connections(user_id);
CREATE INDEX idx_ws_connections_server ON ws_connections(server_id);

-- Voice connections tracking (who is in which voice channel)
CREATE TABLE voice_connections (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id      UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    space_id        UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    livekit_room    VARCHAR(100) NOT NULL,
    self_muted      BOOLEAN NOT NULL DEFAULT FALSE,
    self_deafened   BOOLEAN NOT NULL DEFAULT FALSE,
    server_muted    BOOLEAN NOT NULL DEFAULT FALSE,
    connected_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT voice_connections_user_unique UNIQUE (user_id)
);

CREATE INDEX idx_voice_connections_channel ON voice_connections(channel_id);
CREATE INDEX idx_voice_connections_space ON voice_connections(space_id);
CREATE INDEX idx_voice_connections_room ON voice_connections(livekit_room);

-- Typing indicators (ephemeral, but tracked in DB for multi-server sync)
CREATE TABLE typing_indicators (
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id      UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (user_id, channel_id)
);

CREATE INDEX idx_typing_indicators_channel ON typing_indicators(channel_id);
CREATE INDEX idx_typing_indicators_started ON typing_indicators(started_at);
