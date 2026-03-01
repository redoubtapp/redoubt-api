-- Drop typing indicators
DROP TABLE IF EXISTS typing_indicators;

-- Drop voice connections
DROP TABLE IF EXISTS voice_connections;

-- Drop WebSocket connections
DROP TABLE IF EXISTS ws_connections;

-- Remove presence columns from users
ALTER TABLE users DROP COLUMN IF EXISTS last_seen_at;
ALTER TABLE users DROP COLUMN IF EXISTS presence;

-- Drop presence status enum
DROP TYPE IF EXISTS presence_status;

-- Remove max_participants from channels
ALTER TABLE channels DROP COLUMN IF EXISTS max_participants;
