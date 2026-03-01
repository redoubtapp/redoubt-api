-- Allow hyphens and underscores in channel names
ALTER TABLE channels DROP CONSTRAINT channels_name_format;
ALTER TABLE channels ADD CONSTRAINT channels_name_format CHECK (name ~ '^[a-zA-Z0-9 _-]{1,50}$');
