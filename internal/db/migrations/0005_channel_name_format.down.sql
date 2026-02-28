-- Revert to original channel name format (alphanumeric and spaces only)
ALTER TABLE channels DROP CONSTRAINT channels_name_format;
ALTER TABLE channels ADD CONSTRAINT channels_name_format CHECK (name ~ '^[a-zA-Z0-9 ]{1,50}$');
