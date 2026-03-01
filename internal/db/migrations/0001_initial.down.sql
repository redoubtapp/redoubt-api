-- Drop triggers
DROP TRIGGER IF EXISTS channels_updated_at ON channels;
DROP TRIGGER IF EXISTS spaces_updated_at ON spaces;
DROP TRIGGER IF EXISTS users_updated_at ON users;

-- Drop trigger function
DROP FUNCTION IF EXISTS update_updated_at();

-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS bootstrap_state;
DROP TABLE IF EXISTS login_attempts;
DROP TABLE IF EXISTS invites;
DROP TABLE IF EXISTS channels;
DROP TABLE IF EXISTS memberships;
DROP TABLE IF EXISTS spaces;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS password_resets;
DROP TABLE IF EXISTS email_verifications;
DROP TABLE IF EXISTS users;

-- Drop custom types
DROP TYPE IF EXISTS channel_type;
DROP TYPE IF EXISTS membership_role;

-- Drop extension
DROP EXTENSION IF EXISTS "uuid-ossp";
