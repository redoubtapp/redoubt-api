-- Media files table for storing encrypted file metadata
CREATE TABLE media_files (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id        UUID NOT NULL REFERENCES users(id),
    s3_key          TEXT NOT NULL,
    encryption_key  BYTEA NOT NULL,  -- encrypted with master key
    encryption_iv   BYTEA NOT NULL,
    content_type    VARCHAR(100) NOT NULL,
    size_bytes      BIGINT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_media_files_owner ON media_files(owner_id);
CREATE INDEX idx_media_files_s3_key ON media_files(s3_key);
