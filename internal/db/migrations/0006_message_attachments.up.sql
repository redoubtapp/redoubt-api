-- Message attachments table (links messages to media files)
CREATE TABLE message_attachments (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_id      UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    media_file_id   UUID NOT NULL REFERENCES media_files(id) ON DELETE CASCADE,
    filename        VARCHAR(255) NOT NULL,
    display_order   INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT message_attachments_unique_file UNIQUE (message_id, media_file_id)
);

CREATE INDEX idx_message_attachments_message ON message_attachments(message_id, display_order);
CREATE INDEX idx_message_attachments_media ON message_attachments(media_file_id);

-- Add message_id column to media_files for easier querying
-- This is optional since we have the join table, but helps with cleanup
ALTER TABLE media_files
    ADD COLUMN message_id UUID REFERENCES messages(id) ON DELETE SET NULL;

CREATE INDEX idx_media_files_message ON media_files(message_id) WHERE message_id IS NOT NULL;
