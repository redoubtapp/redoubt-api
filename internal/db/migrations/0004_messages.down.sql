-- Reverse membership extensions (must be done before dropping messages table)
ALTER TABLE memberships
    DROP COLUMN IF EXISTS last_read_at,
    DROP COLUMN IF EXISTS last_read_message_id;

-- Drop tables in dependency order
DROP TABLE IF EXISTS message_rate_limits;
DROP TABLE IF EXISTS emoji_set;
DROP TABLE IF EXISTS reactions;
DROP TABLE IF EXISTS message_edits;
DROP TABLE IF EXISTS messages;
