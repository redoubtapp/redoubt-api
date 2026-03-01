-- Remove message_id from media_files
DROP INDEX IF EXISTS idx_media_files_message;
ALTER TABLE media_files DROP COLUMN IF EXISTS message_id;

-- Drop message_attachments table
DROP TABLE IF EXISTS message_attachments;
