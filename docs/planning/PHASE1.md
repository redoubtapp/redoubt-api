# Redoubt — Phase 1 Implementation Document

**Status:** Ready for implementation
**Last updated:** 2026-02-22
**Author:** Michael

This document defines the complete scope, technical decisions, and implementation details for Phase 1 (Foundation) of Redoubt.

---

## Table of Contents

- [1. Phase 1 Scope Summary](#1-phase-1-scope-summary)
- [2. Architecture Decisions](#2-architecture-decisions)
- [3. Project Structure](#3-project-structure)
- [4. Infrastructure Stack](#4-infrastructure-stack)
- [5. Database Schema](#5-database-schema)
- [6. Authentication System](#6-authentication-system)
- [7. Authorization Model](#7-authorization-model)
- [8. API Design](#8-api-design)
- [9. Media Storage & Encryption](#9-media-storage--encryption)
- [10. Rate Limiting & Security](#10-rate-limiting--security)
- [11. Observability](#11-observability)
- [12. Configuration](#12-configuration)
- [13. Development Tooling](#13-development-tooling)
- [14. Testing Strategy](#14-testing-strategy)
- [15. CI/CD Pipeline](#15-cicd-pipeline)
- [16. Implementation Tasks](#16-implementation-tasks)
- [17. Acceptance Criteria](#17-acceptance-criteria)

---

## 1. Phase 1 Scope Summary

Phase 1 establishes the foundation for Redoubt with the following deliverables:

| Component | Scope |
|-----------|-------|
| Go API | Project scaffold, routing, middleware, graceful shutdown |
| Database | PostgreSQL 16, migrations, sqlc-generated queries |
| Authentication | Registration (invite-only), login, JWT + refresh tokens, email verification (blocking), password reset |
| Authorization | Instance admin, Space roles (owner/admin/member), permission checks |
| CRUD | Spaces, Channels (text/voice types), Memberships, Invites |
| Media | S3-compatible storage with client-side + server-side encryption |
| Caching | Redis for rate limiting and general cache |
| Observability | OpenTelemetry tracing + Prometheus metrics via ootel |
| Docker | Compose stack with Caddy, health checks, dev/prod separation |
| Tooling | Justfile, air hot reload, golangci-lint |
| Testing | Integration + unit tests with testcontainers |
| CI/CD | GitHub Actions for lint, test, build |

---

## 2. Architecture Decisions

### Core Libraries & Frameworks

| Concern | Choice | Rationale |
|---------|--------|-----------|
| HTTP Router | `gorilla/mux` | Well-established, good middleware support |
| Logging | `log/slog` | Stdlib, structured logging, zero dependencies |
| Config | `viper` | YAML config files, env overrides, feature-rich |
| Validation | `go-playground/validator` | Struct tags, comprehensive rules, good errors |
| DB Driver | `pgx/v5` | Native PostgreSQL driver, built-in pooling |
| Query Gen | `sqlc` | Type-safe queries from SQL |
| Migrations | `golang-migrate` | CLI + library, sequential numbered files |
| Password Hash | `argon2id` | Memory-hard, recommended over bcrypt |
| JWT | `golang-jwt/jwt/v5` | Standard JWT library |
| Redis | `redis/go-redis/v9` | Full-featured Redis client |
| S3 | `aws-sdk-go-v2` | Works with any S3-compatible storage |
| Email | Resend API | Simple transactional email |
| OpenAPI | `swaggo/swag` | Auto-generate OpenAPI from comments |
| Errors | `alpineworks/rfc9457` | RFC 9457 Problem Details responses |
| Telemetry | `alpineworks/ootel` | OpenTelemetry initialization (traces + metrics) |
| HTTP Tracing | `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` | HTTP middleware instrumentation |
| PostgreSQL Tracing | `github.com/exaring/otelpgx` | pgx database instrumentation |
| Redis Tracing | `github.com/redis/go-redis/extra/redisotel/v9` | Redis client instrumentation |

### Key Design Decisions

| Decision | Choice |
|----------|--------|
| Error handling | Sentinel errors internally, RFC 9457 responses externally |
| Timestamps | UTC always (stored and returned) |
| Pagination | Cursor-based with `has_more` boolean |
| Request tracing | X-Request-ID header, included in logs |
| Graceful shutdown | Handle SIGTERM, drain connections |
| Unknown JSON fields | Ignore silently |
| Duplicate registration | Generic error (don't reveal existence) |
| Username case | Case-sensitive (`Alice` ≠ `alice`) |
| Email normalization | Lowercase on registration |
| Space/Channel names | 1-50 chars, alphanumeric + spaces only |
| Password changes | Email reset required (no direct change) |
| Session limits | No limit per user |
| Bulk operations | Not supported in Phase 1 |
| Rate limit headers | IETF draft (`RateLimit-*`) |
| Audit logging | Basic logging for admin actions |

---

## 3. Project Structure

```
redoubt/
├── cmd/
│   └── redoubt-api/
│       └── main.go                 # Entry point
├── internal/
│   ├── api/
│   │   ├── router.go               # Route definitions
│   │   ├── middleware/
│   │   │   ├── auth.go             # JWT validation
│   │   │   ├── cors.go             # CORS handling
│   │   │   ├── logging.go          # Request logging
│   │   │   ├── ratelimit.go        # Rate limiting
│   │   │   └── requestid.go        # X-Request-ID
│   │   └── handlers/
│   │       ├── auth.go             # Auth endpoints
│   │       ├── users.go            # User endpoints
│   │       ├── spaces.go           # Space endpoints
│   │       ├── channels.go         # Channel endpoints
│   │       ├── invites.go          # Invite endpoints
│   │       ├── sessions.go         # Session management
│   │       └── health.go           # Health check
│   ├── auth/
│   │   ├── jwt.go                  # Token generation/validation
│   │   ├── password.go             # Argon2id hashing
│   │   ├── session.go              # Session management
│   │   └── tokens.go               # Refresh token handling
│   ├── config/
│   │   └── config.go               # Viper configuration
│   ├── db/
│   │   ├── migrations/             # SQL migration files
│   │   │   ├── 0001_initial.up.sql
│   │   │   ├── 0001_initial.down.sql
│   │   │   └── ...
│   │   ├── queries/                # sqlc query files
│   │   │   ├── users.sql
│   │   │   ├── spaces.sql
│   │   │   ├── channels.sql
│   │   │   ├── memberships.sql
│   │   │   ├── invites.sql
│   │   │   └── sessions.sql
│   │   ├── sqlc.yaml               # sqlc config
│   │   └── generated/              # sqlc output
│   ├── email/
│   │   ├── client.go               # Resend client
│   │   └── templates.go            # Email templates
│   ├── errors/
│   │   ├── errors.go               # Sentinel errors
│   │   └── problems.go             # RFC 9457 problem responses
│   ├── invite/
│   │   ├── generator.go            # Word-based code generation
│   │   └── wordlist.go             # EFF wordlist
│   ├── ratelimit/
│   │   └── redis.go                # Redis-backed rate limiter
│   ├── space/
│   │   └── service.go              # Space business logic
│   ├── storage/
│   │   ├── s3.go                   # S3 client
│   │   └── encryption.go           # Client-side encryption
│   ├── cache/
│   │   └── redis.go                # Redis cache wrapper
│   └── telemetry/
│       └── otel.go                 # OpenTelemetry initialization (ootel)
├── config/
│   ├── config.yaml                 # Default configuration
│   └── config.dev.yaml             # Development overrides
├── docker/
│   ├── Dockerfile                  # Multi-stage production build
│   ├── Dockerfile.dev              # Development with air
│   ├── .air.toml                   # Air hot reload config
│   ├── alloy/
│   │   └── config.alloy            # Grafana Alloy pipeline config
│   └── grafana/
│       ├── dashboards.yaml         # Dashboard provisioning
│       └── dashboards/             # Custom Grafana dashboards (JSON)
├── docker-compose.yml              # Production compose
├── docker-compose.dev.yml          # Development overlay
├── Caddyfile                       # Reverse proxy config
├── justfile                        # Task runner
├── sqlc.yaml                       # sqlc configuration
├── .golangci.yml                   # Linter configuration
├── .github/
│   └── workflows/
│       └── ci.yml                  # GitHub Actions
├── go.mod
├── go.sum
└── README.md
```

---

## 4. Infrastructure Stack

### Docker Compose Services

| Service | Image | Purpose | Ports (internal) |
|---------|-------|---------|------------------|
| `redoubt-api` | Custom Go build | Main API server | 8080 |
| `postgres` | postgres:16-alpine | Primary database | 5432 |
| `redis` | redis:7-alpine | Rate limiting + cache | 6379 |
| `caddy` | caddy:2-alpine | Reverse proxy + TLS | 80, 443 |
| `localstack` | localstack/localstack | S3-compatible storage (dev) | 4566 |
| `mailpit` | axllent/mailpit | Email testing (dev) | 1025, 8025 |
| `lgtm` | grafana/otel-lgtm | Observability stack (dev) | 3000, 4317, 4318 |
| `alloy` | grafana/alloy | Telemetry collection (dev) | 12345, 9411 |

### Production Compose (`docker-compose.yml`)

```yaml
services:
  redoubt-api:
    build:
      context: .
      dockerfile: docker/Dockerfile
    environment:
      - CONFIG_PATH=/etc/redoubt/config.yaml
    volumes:
      - ./config/config.yaml:/etc/redoubt/config.yaml:ro
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 3
    restart: unless-stopped

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: redoubt
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: redoubt
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U redoubt"]
      interval: 5s
      timeout: 5s
      retries: 5
    restart: unless-stopped

  redis:
    image: redis:7-alpine
    command: redis-server --appendonly yes
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5
    restart: unless-stopped

  caddy:
    image: caddy:2-alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    depends_on:
      - redoubt-api
    restart: unless-stopped

volumes:
  postgres_data:
  redis_data:
  caddy_data:
  caddy_config:
```

### Development Compose Overlay (`docker-compose.dev.yml`)

```yaml
services:
  redoubt-api:
    build:
      context: .
      dockerfile: docker/Dockerfile.dev
    volumes:
      - .:/app
      - go_mod_cache:/go/pkg/mod
    environment:
      - CONFIG_PATH=/app/config/config.dev.yaml

  localstack:
    image: localstack/localstack
    environment:
      - SERVICES=s3
      - DEFAULT_REGION=us-east-1
    volumes:
      - localstack_data:/var/lib/localstack

  mailpit:
    image: axllent/mailpit
    ports:
      - "8025:8025"  # Web UI

  lgtm:
    image: grafana/otel-lgtm
    ports:
      - "3000:3000"   # Grafana UI
      - "4317:4317"   # OTLP gRPC
      - "4318:4318"   # OTLP HTTP
    volumes:
      - ./docker/grafana/dashboards:/var/lib/grafana/dashboards
      - ./docker/grafana/dashboards.yaml:/otel-lgtm/grafana/conf/provisioning/dashboards/grafana-dashboards.yaml
    environment:
      - GF_AUTH_ANONYMOUS_ENABLED=true
      - GF_AUTH_ANONYMOUS_ORG_ROLE=Admin
      - GF_AUTH_DISABLE_LOGIN_FORM=true

  alloy:
    image: grafana/alloy:v1.4.2
    command:
      - run
      - --server.http.listen-addr
      - 0.0.0.0:12345
      - /config.alloy
      - --stability.level=experimental
    volumes:
      - ./docker/alloy/config.alloy:/config.alloy
      - /var/run/docker.sock:/var/run/docker.sock
    ports:
      - "12345:12345"  # Alloy UI
      - "9411:9411"    # Zipkin receiver

volumes:
  go_mod_cache:
  localstack_data:
```

### Caddyfile

```
{$DOMAIN:localhost} {
    handle /api/v1/* {
        reverse_proxy redoubt-api:8080
    }

    handle /health {
        reverse_proxy redoubt-api:8080
    }

    handle {
        respond "Redoubt API" 200
    }
}
```

---

## 5. Database Schema

### Migration: `0001_initial.up.sql`

```sql
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

    CONSTRAINT users_username_unique UNIQUE (username) WHERE deleted_at IS NULL,
    CONSTRAINT users_email_unique UNIQUE (email) WHERE deleted_at IS NULL,
    CONSTRAINT users_username_format CHECK (username ~ '^[a-zA-Z0-9_]{3,32}$'),
    CONSTRAINT users_email_lowercase CHECK (email = LOWER(email))
);

CREATE INDEX idx_users_email ON users(email) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_username ON users(username) WHERE deleted_at IS NULL;

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
```

---

## 6. Authentication System

### Registration Flow

```
1. User submits: username, email, password, invite_code
2. Validate invite code exists, not expired, not revoked, under max_uses
3. Validate username (3-32 chars, alphanumeric + underscore)
4. Validate email format
5. Validate password (12+ chars)
6. Check username/email uniqueness → generic error if exists
7. Hash password with Argon2id
8. Create user record (email_verified = false)
9. If first user ever → set is_instance_admin = true
10. Increment invite uses count
11. Generate email verification token (expires: 24 hours)
12. Send verification email via Resend
13. Return success (user cannot login until verified)
```

### Email Verification Flow

```
1. User clicks link: /verify-email?token=xxx
2. Find token, check not expired
3. Set user.email_verified = true
4. Delete verification token
5. Return success
```

### Login Flow

```
1. Check IP/email lockout status (5 failed attempts → 15 min lockout)
2. Find user by email
3. Verify password with Argon2id
4. Check email_verified = true
5. Check deleted_at IS NULL
6. Record login attempt (success/failure)
7. Generate JWT access token (15 min expiry)
8. Generate refresh token, store in sessions table
9. Return tokens + user info
```

### Token Refresh Flow

```
1. Validate refresh token exists in sessions
2. Check not expired, not revoked
3. Update last_used_at
4. Generate new JWT access token
5. Return new access token (same refresh token)
```

### Password Reset Flow

```
1. User submits email
2. If user exists and email_verified → generate reset token (1 hour expiry)
3. Send reset email via Resend
4. Always return success (don't reveal if email exists)

Reset:
1. User submits token + new password
2. Validate token not expired, not used
3. Hash new password
4. Update user password
5. Mark token as used
6. Revoke all existing sessions
7. Return success
```

### Session Management

```
GET /api/v1/sessions           → List active sessions
DELETE /api/v1/sessions/:id    → Revoke specific session
DELETE /api/v1/sessions        → Revoke all sessions (logout everywhere)
```

### JWT Structure

```json
{
  "sub": "user-uuid",
  "iat": 1708617600,
  "exp": 1708618500,
  "jti": "unique-token-id",
  "admin": true
}
```

### Argon2id Parameters

```go
Time:    1
Memory:  64 * 1024  // 64 MB
Threads: 4
KeyLen:  32
```

---

## 7. Authorization Model

### Role Hierarchy

| Level | Role | Description |
|-------|------|-------------|
| Instance | `instance_admin` | First registered user. Can create Spaces, see all Spaces, manage instance. |
| Space | `owner` | Created the Space (always the instance admin). Full Space control. |
| Space | `admin` | Can manage channels, invites, kick members. |
| Space | `member` | Can view and participate in channels. |

### Permission Matrix

| Action | Instance Admin | Space Owner | Space Admin | Space Member |
|--------|----------------|-------------|-------------|--------------|
| Create Space | Yes | - | - | - |
| Delete Space | Yes | Yes | No | No |
| Update Space | Yes | Yes | Yes | No |
| Create Channel | Yes | Yes | Yes | No |
| Delete Channel | Yes | Yes | Yes | No |
| Update Channel | Yes | Yes | Yes | No |
| Create Invite | Yes | Yes | Yes | No |
| Revoke Invite | Yes | Yes | Yes | No |
| Kick Member | Yes | Yes | Yes | No |
| Change Roles | Yes | Yes | No | No |
| View Space | Yes | Yes | Yes | Yes |
| View Channels | Yes | Yes | Yes | Yes |

### Permission Check Middleware

```go
// RequireAuth - validates JWT, sets user in context
// RequireInstanceAdmin - checks is_instance_admin flag
// RequireSpaceRole(minRole) - checks membership role >= minRole
// RequireSpaceMember - checks any membership exists
```

---

## 8. API Design

### Base URL

```
https://{domain}/api/v1
```

### Request/Response Format

- Content-Type: `application/json`
- All timestamps: ISO 8601 UTC (`2026-02-22T10:30:00Z`)
- All IDs: UUID v4
- Request ID: `X-Request-ID` header (auto-generated if not provided)

### Error Response Format (RFC 9457)

All error responses conform to [RFC 9457 Problem Details for HTTP APIs](https://www.rfc-editor.org/rfc/rfc9457.html) using the `github.com/alpineworks/rfc9457` library.

**Standard Fields:**

| Field | Description |
|-------|-------------|
| `type` | URI reference identifying the problem type |
| `title` | Short, human-readable summary |
| `status` | HTTP status code |
| `detail` | Human-readable explanation specific to this occurrence |
| `instance` | URI reference identifying the specific occurrence |

**Extension Fields (added by Redoubt):**

| Field | Description |
|-------|-------------|
| `request_id` | X-Request-ID for tracing |
| `errors` | Array of field-level validation errors (when applicable) |

**Example: Validation Error**

```json
{
  "type": "https://redoubt.app/problems/validation-error",
  "title": "Validation Failed",
  "status": 400,
  "detail": "The request body contains invalid fields",
  "instance": "/api/v1/auth/register",
  "request_id": "abc-123-def",
  "errors": [
    {
      "field": "username",
      "message": "must be between 3 and 32 characters"
    },
    {
      "field": "password",
      "message": "must be at least 12 characters"
    }
  ]
}
```

**Example: Authentication Error**

```json
{
  "type": "https://redoubt.app/problems/invalid-credentials",
  "title": "Invalid Credentials",
  "status": 401,
  "detail": "The email or password provided is incorrect",
  "instance": "/api/v1/auth/login",
  "request_id": "abc-123-def"
}
```

**Example: Rate Limited**

```json
{
  "type": "https://redoubt.app/problems/rate-limited",
  "title": "Too Many Requests",
  "status": 429,
  "detail": "Rate limit exceeded. Try again in 45 seconds.",
  "instance": "/api/v1/auth/login",
  "request_id": "abc-123-def"
}
```

### Problem Types

| Type URI | HTTP Status | Title |
|----------|-------------|-------|
| `.../validation-error` | 400 | Validation Failed |
| `.../invalid-credentials` | 401 | Invalid Credentials |
| `.../unauthorized` | 401 | Unauthorized |
| `.../email-not-verified` | 403 | Email Not Verified |
| `.../forbidden` | 403 | Forbidden |
| `.../not-found` | 404 | Not Found |
| `.../conflict` | 409 | Conflict |
| `.../rate-limited` | 429 | Too Many Requests |
| `.../account-locked` | 423 | Account Locked |
| `.../internal-error` | 500 | Internal Server Error |

Base URI: `https://redoubt.app/problems/`

### RFC 9457 Implementation

```go
// internal/errors/problems.go

package errors

import (
    "net/http"
    "github.com/alpineworks/rfc9457"
)

const ProblemBaseURI = "https://redoubt.app/problems/"

// ValidationError returns a 400 response with field errors
func ValidationError(w http.ResponseWriter, r *http.Request, fieldErrors []FieldError) {
    rfc9457.NewRFC9457(
        rfc9457.WithType(ProblemBaseURI+"validation-error"),
        rfc9457.WithTitle("Validation Failed"),
        rfc9457.WithStatus(http.StatusBadRequest),
        rfc9457.WithDetail("The request body contains invalid fields"),
        rfc9457.WithInstance(r.URL.Path),
        rfc9457.WithExtension("request_id", GetRequestID(r)),
        rfc9457.WithExtension("errors", fieldErrors),
    ).ServeHTTP(w, r)
}

// InvalidCredentials returns a 401 response for bad login
func InvalidCredentials(w http.ResponseWriter, r *http.Request) {
    rfc9457.NewRFC9457(
        rfc9457.WithType(ProblemBaseURI+"invalid-credentials"),
        rfc9457.WithTitle("Invalid Credentials"),
        rfc9457.WithStatus(http.StatusUnauthorized),
        rfc9457.WithDetail("The email or password provided is incorrect"),
        rfc9457.WithInstance(r.URL.Path),
        rfc9457.WithExtension("request_id", GetRequestID(r)),
    ).ServeHTTP(w, r)
}

// NotFound returns a 404 response
func NotFound(w http.ResponseWriter, r *http.Request, resource string) {
    rfc9457.NewRFC9457(
        rfc9457.WithType(ProblemBaseURI+"not-found"),
        rfc9457.WithTitle("Not Found"),
        rfc9457.WithStatus(http.StatusNotFound),
        rfc9457.WithDetail(resource+" not found"),
        rfc9457.WithInstance(r.URL.Path),
        rfc9457.WithExtension("request_id", GetRequestID(r)),
    ).ServeHTTP(w, r)
}

// RateLimited returns a 429 response
func RateLimited(w http.ResponseWriter, r *http.Request, retryAfter int) {
    w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
    rfc9457.NewRFC9457(
        rfc9457.WithType(ProblemBaseURI+"rate-limited"),
        rfc9457.WithTitle("Too Many Requests"),
        rfc9457.WithStatus(http.StatusTooManyRequests),
        rfc9457.WithDetail(fmt.Sprintf("Rate limit exceeded. Try again in %d seconds.", retryAfter)),
        rfc9457.WithInstance(r.URL.Path),
        rfc9457.WithExtension("request_id", GetRequestID(r)),
    ).ServeHTTP(w, r)
}

// FieldError represents a single validation error
type FieldError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
}
```

### Pagination

Request:
```
GET /api/v1/spaces/:id/channels?cursor=xxx&limit=20
```

Response:
```json
{
  "data": [...],
  "pagination": {
    "next_cursor": "eyJpZCI6Inh4eCJ9",
    "has_more": true
  }
}
```

### Endpoints

#### Authentication

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| POST | `/auth/register` | Register with invite code | No |
| POST | `/auth/login` | Login, get tokens | No |
| POST | `/auth/refresh` | Refresh access token | Refresh token |
| POST | `/auth/logout` | Invalidate refresh token | Yes |
| POST | `/auth/verify-email` | Verify email with token | No |
| POST | `/auth/forgot-password` | Request password reset | No |
| POST | `/auth/reset-password` | Reset password with token | No |

#### Users

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/users/me` | Get current user | Yes |
| PATCH | `/users/me` | Update current user | Yes |
| DELETE | `/users/me` | Soft delete account | Yes |
| PUT | `/users/me/avatar` | Upload avatar | Yes |
| DELETE | `/users/me/avatar` | Remove avatar | Yes |

#### Sessions

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/sessions` | List active sessions | Yes |
| DELETE | `/sessions/:id` | Revoke specific session | Yes |
| DELETE | `/sessions` | Revoke all sessions | Yes |

#### Spaces

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/spaces` | List user's spaces | Yes |
| POST | `/spaces` | Create space | Instance Admin |
| GET | `/spaces/:id` | Get space details | Member |
| PATCH | `/spaces/:id` | Update space | Admin+ |
| DELETE | `/spaces/:id` | Soft delete space | Owner |
| GET | `/spaces/:id/members` | List members | Member |
| DELETE | `/spaces/:id/members/:userId` | Kick member | Admin+ |
| PATCH | `/spaces/:id/members/:userId` | Change role | Owner |

#### Channels

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/spaces/:id/channels` | List channels | Member |
| POST | `/spaces/:id/channels` | Create channel | Admin+ |
| GET | `/channels/:id` | Get channel details | Member |
| PATCH | `/channels/:id` | Update channel | Admin+ |
| DELETE | `/channels/:id` | Soft delete channel | Admin+ |
| PATCH | `/spaces/:id/channels/reorder` | Reorder channels | Admin+ |

#### Invites

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/spaces/:id/invites` | List invites | Admin+ |
| POST | `/spaces/:id/invites` | Create invite | Admin+ |
| DELETE | `/invites/:id` | Revoke invite | Admin+ |
| POST | `/invites/:code/join` | Join space via invite | Yes |
| GET | `/invites/:code` | Get invite info (public) | No |

#### Health

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/health` | Health check (detailed) | No |
| GET | `/health/live` | Liveness probe | No |
| GET | `/health/ready` | Readiness probe | No |

### Request/Response Examples

#### Register

```http
POST /api/v1/auth/register
Content-Type: application/json

{
  "username": "alice",
  "email": "alice@example.com",
  "password": "correct-horse-battery-staple",
  "invite_code": "alpha-bravo-charlie-delta"
}
```

```json
{
  "message": "Registration successful. Please check your email to verify your account."
}
```

#### Login

```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "email": "alice@example.com",
  "password": "correct-horse-battery-staple"
}
```

```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIs...",
  "refresh_token": "dGhpcyBpcyBhIHJlZnJlc2ggdG9rZW4...",
  "expires_in": 900,
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "username": "alice",
    "email": "alice@example.com",
    "avatar_url": null,
    "is_instance_admin": true,
    "created_at": "2026-02-22T10:30:00Z"
  }
}
```

#### Create Space

```http
POST /api/v1/spaces
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
Content-Type: application/json

{
  "name": "My Community"
}
```

```json
{
  "id": "660e8400-e29b-41d4-a716-446655440001",
  "name": "My Community",
  "icon_url": null,
  "owner_id": "550e8400-e29b-41d4-a716-446655440000",
  "created_at": "2026-02-22T10:35:00Z",
  "channels": [
    {
      "id": "770e8400-e29b-41d4-a716-446655440002",
      "name": "general",
      "type": "text",
      "position": 0
    },
    {
      "id": "770e8400-e29b-41d4-a716-446655440003",
      "name": "General",
      "type": "voice",
      "position": 1
    }
  ]
}
```

#### Health Check

```http
GET /api/v1/health
```

```json
{
  "status": "healthy",
  "version": "0.1.0",
  "components": {
    "database": {
      "status": "healthy",
      "latency_ms": 2
    },
    "redis": {
      "status": "healthy",
      "latency_ms": 1
    },
    "storage": {
      "status": "healthy"
    }
  }
}
```

---

## 9. Media Storage & Encryption

### Architecture

```
Client                     Go API                    S3 (LocalStack/MinIO/R2)
  │                          │                              │
  │ PUT /users/me/avatar     │                              │
  ├─────────────────────────►│                              │
  │                          │                              │
  │                          │ 1. Generate encryption key   │
  │                          │ 2. Encrypt file (AES-256-GCM)│
  │                          │ 3. Upload encrypted blob     │
  │                          ├─────────────────────────────►│
  │                          │                              │ SSE enabled
  │                          │ 4. Store key reference       │
  │                          │                              │
  │ 200 OK (avatar_url)      │                              │
  │◄─────────────────────────┤                              │
```

### Encryption Layers

1. **Client-side encryption (Go API):**
   - AES-256-GCM encryption before upload
   - Unique key per file, stored in database
   - Key derivation from master key (stored in config/env)

2. **Server-side encryption (S3):**
   - SSE-S3 or SSE-C enabled on bucket
   - Additional encryption at rest

### Upload Constraints

| Constraint | Value |
|------------|-------|
| Max file size | 5 MB |
| Allowed formats | PNG, JPEG, WebP, GIF |
| Allowed MIME types | `image/png`, `image/jpeg`, `image/webp`, `image/gif` |

### File Storage Schema

```sql
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
```

### Upload Flow

1. Receive file from client
2. Validate file type and size
3. Generate random AES-256 key
4. Encrypt file content with AES-256-GCM
5. Upload encrypted blob to S3 with SSE-S3
6. Encrypt the file key with master key
7. Store metadata + encrypted key in database
8. Return public URL (requires API to decrypt/proxy)

### Download Flow

1. Client requests media
2. API fetches encrypted blob from S3
3. Retrieve file key from database
4. Decrypt file key with master key
5. Decrypt file content
6. Return to client

### Configuration

```yaml
storage:
  endpoint: "http://localstack:4566"  # or s3.amazonaws.com
  bucket: "redoubt-media"
  region: "us-east-1"
  access_key: "${S3_ACCESS_KEY}"
  secret_key: "${S3_SECRET_KEY}"
  encryption:
    master_key: "${STORAGE_MASTER_KEY}"  # 32 bytes, base64
```

---

## 10. Rate Limiting & Security

### Rate Limits

| Endpoint | Limit | Window | Scope |
|----------|-------|--------|-------|
| `POST /auth/register` | 5 | 1 hour | IP |
| `POST /auth/login` | 10 | 15 min | IP + Email |
| `POST /auth/forgot-password` | 3 | 1 hour | IP |
| `POST /auth/verify-email` | 10 | 1 hour | IP |
| General API | 100 | 1 min | User |
| File upload | 10 | 1 hour | User |

### Rate Limit Response Headers (IETF Draft)

All rate-limited endpoints include these headers:

| Header | Description |
|--------|-------------|
| `RateLimit-Limit` | Maximum requests allowed in window |
| `RateLimit-Remaining` | Requests remaining in current window |
| `RateLimit-Reset` | Seconds until the window resets |
| `Retry-After` | Seconds to wait (only when rate limited) |

Example response headers:
```
RateLimit-Limit: 100
RateLimit-Remaining: 42
RateLimit-Reset: 58
```

### Redis Rate Limiting Implementation

```go
// Sliding window rate limiter using Redis
key := fmt.Sprintf("ratelimit:%s:%s", scope, identifier)
// ZREMRANGEBYSCORE key 0 (now - window)
// ZADD key now now
// ZCARD key
// Compare with limit
```

### Account Lockout

- **Trigger:** 5 failed login attempts within 15 minutes
- **Duration:** 15-minute lockout
- **Scope:** Per email address
- **Storage:** PostgreSQL `login_attempts` table

### CORS Configuration

```yaml
cors:
  allowed_origins:
    - "https://app.example.com"
    - "tauri://localhost"  # Tauri desktop app
  allowed_methods:
    - GET
    - POST
    - PUT
    - PATCH
    - DELETE
  allowed_headers:
    - Authorization
    - Content-Type
    - X-Request-ID
  max_age: 86400
```

### Security Headers

Applied via middleware:

```
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
X-XSS-Protection: 1; mode=block
Referrer-Policy: strict-origin-when-cross-origin
```

---

## 11. Observability

All metrics and traces use OpenTelemetry via `github.com/alpineworks/ootel` with proper instrumentation libraries for each component.

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         redoubt-api                              │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │
│  │  otelhttp   │  │   otelpgx   │  │  redisotel  │              │
│  │ (HTTP mid)  │  │  (DB traces)│  │(Redis traces)│             │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘              │
│         │                │                │                      │
│         └────────────────┼────────────────┘                      │
│                          ▼                                       │
│                 ┌─────────────────┐                              │
│                 │  ootel client   │                              │
│                 │  (initializer)  │                              │
│                 └────────┬────────┘                              │
│                          │                                       │
│         ┌────────────────┼────────────────┐                      │
│         ▼                ▼                ▼                      │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │
│  │   /metrics  │  │ OTLP gRPC   │  │ /healthcheck│              │
│  │ (Prometheus)│  │  (traces)   │  │   (ootel)   │              │
│  └─────────────┘  └─────────────┘  └─────────────┘              │
└─────────────────────────────────────────────────────────────────┘
         │                │
         ▼                ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Grafana Alloy                                │
│         (metrics scraping, log collection, trace relay)          │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Grafana LGTM Stack                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌────────┐  │
│  │    Loki     │  │   Grafana   │  │    Tempo    │  │ Mimir  │  │
│  │   (logs)    │  │    (UI)     │  │  (traces)   │  │(metrics)│ │
│  └─────────────┘  └─────────────┘  └─────────────┘  └────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### Instrumentation Libraries

| Component | Library | Purpose |
|-----------|---------|---------|
| Initialization | `alpineworks/ootel` | OpenTelemetry setup, Prometheus server |
| HTTP Server | `otelhttp` | Trace incoming requests, record latency metrics |
| PostgreSQL | `otelpgx` | Trace database queries, record query metrics |
| Redis | `redisotel` | Trace Redis commands |
| HTTP Client | `otelhttp` | Trace outgoing HTTP requests (S3, Resend) |

### Metrics Exposed

All metrics are exposed on `/metrics` in Prometheus format via ootel's built-in server.

**HTTP Metrics:**

| Metric | Type | Description |
|--------|------|-------------|
| `http_server_request_duration_seconds` | Histogram | Request latency by route, method, status |
| `http_server_requests_total` | Counter | Total requests by route, method, status |
| `http_server_active_requests` | Gauge | Currently active requests |

**Database Metrics:**

| Metric | Type | Description |
|--------|------|-------------|
| `db_client_connections_usage` | Gauge | Connection pool usage |
| `db_client_connections_max` | Gauge | Max pool size |

**Application Metrics (custom):**

| Metric | Type | Description |
|--------|------|-------------|
| `redoubt_auth_logins_total` | Counter | Login attempts by status (success/failure) |
| `redoubt_auth_registrations_total` | Counter | Registrations by status |
| `redoubt_spaces_total` | Gauge | Total active spaces |
| `redoubt_users_total` | Gauge | Total active users |
| `redoubt_invites_redeemed_total` | Counter | Invite code redemptions |

**Rate Limiting Metrics:**

| Metric | Type | Description |
|--------|------|-------------|
| `redoubt_ratelimit_hits_total` | Counter | Rate limit checks by endpoint |
| `redoubt_ratelimit_exceeded_total` | Counter | Rate limit exceeded events |

### Trace Spans

All traces include the following attributes:

| Attribute | Description |
|-----------|-------------|
| `service.name` | "redoubt-api" |
| `service.version` | Build version |
| `http.method` | HTTP method |
| `http.route` | Route pattern (e.g., `/api/v1/spaces/:id`) |
| `http.status_code` | Response status |
| `user.id` | Authenticated user ID (when applicable) |
| `request.id` | X-Request-ID |

**Database Spans:**

| Attribute | Description |
|-----------|-------------|
| `db.system` | "postgresql" |
| `db.name` | Database name |
| `db.statement` | SQL query (sanitized) |
| `db.operation` | Query type (SELECT, INSERT, etc.) |

### Initialization

```go
// internal/telemetry/otel.go

package telemetry

import (
    "context"
    "github.com/alpineworks/ootel"
)

type Config struct {
    ServiceName    string
    ServiceVersion string
    TraceEnabled   bool
    TraceSampleRate float64
    MetricsEnabled bool
    MetricsPort    int
}

func Initialize(ctx context.Context, cfg Config) (func(context.Context) error, error) {
    client := ootel.NewOotelClient(
        ootel.WithTraceConfig(ootel.NewTraceConfig(
            cfg.TraceEnabled,
            cfg.TraceSampleRate,
            cfg.ServiceName,
            cfg.ServiceVersion,
        )),
        ootel.WithMetricConfig(ootel.NewMetricConfig(
            cfg.MetricsEnabled,
            cfg.MetricsPort,
        )),
    )

    return client.Init(ctx)
}
```

### HTTP Middleware Integration

```go
// internal/api/router.go

import (
    "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func NewRouter(handlers *Handlers) http.Handler {
    r := mux.NewRouter()

    // ... route definitions ...

    // Wrap entire router with OpenTelemetry
    return otelhttp.NewHandler(r, "redoubt-api",
        otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
            return r.Method + " " + r.URL.Path
        }),
    )
}
```

### Database Instrumentation

```go
// internal/db/postgres.go

import (
    "github.com/exaring/otelpgx"
    "github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, connString string) (*pgxpool.Pool, error) {
    cfg, err := pgxpool.ParseConfig(connString)
    if err != nil {
        return nil, err
    }

    // Add OpenTelemetry tracer
    cfg.ConnConfig.Tracer = otelpgx.NewTracer()

    return pgxpool.NewWithConfig(ctx, cfg)
}
```

### Redis Instrumentation

```go
// internal/cache/redis.go

import (
    "github.com/redis/go-redis/extra/redisotel/v9"
    "github.com/redis/go-redis/v9"
)

func NewClient(opts *redis.Options) *redis.Client {
    client := redis.NewClient(opts)

    // Add OpenTelemetry instrumentation
    if err := redisotel.InstrumentTracing(client); err != nil {
        panic(err)
    }
    if err := redisotel.InstrumentMetrics(client); err != nil {
        panic(err)
    }

    return client
}
```

### Development Stack (LGTM + Alloy)

For local development, the observability stack uses Grafana's LGTM (Loki, Grafana, Tempo, Mimir) all-in-one image with Grafana Alloy for collection.

Add to `docker-compose.dev.yml`:

```yaml
services:
  lgtm:
    image: grafana/otel-lgtm
    ports:
      - "3000:3000"   # Grafana UI
      - "4317:4317"   # OTLP gRPC
      - "4318:4318"   # OTLP HTTP
    volumes:
      - ./docker/grafana/dashboards:/var/lib/grafana/dashboards
      - ./docker/grafana/dashboards.yaml:/otel-lgtm/grafana/conf/provisioning/dashboards/grafana-dashboards.yaml
    environment:
      - GF_AUTH_ANONYMOUS_ENABLED=true
      - GF_AUTH_ANONYMOUS_ORG_ROLE=Admin
      - GF_AUTH_DISABLE_LOGIN_FORM=true

  alloy:
    image: grafana/alloy:v1.4.2
    command:
      - run
      - --server.http.listen-addr
      - 0.0.0.0:12345
      - /config.alloy
      - --stability.level=experimental
    volumes:
      - ./docker/alloy/config.alloy:/config.alloy
      - /var/run/docker.sock:/var/run/docker.sock
    ports:
      - "12345:12345"  # Alloy UI
      - "9411:9411"    # Zipkin receiver
```

### Alloy Configuration

Create `docker/alloy/config.alloy`:

```hcl
// Enable live debugging
livedebugging {
  enabled = true
}

// Docker service discovery
discovery.docker "containers" {
  host = "unix:///var/run/docker.sock"
}

// Relabel discovered containers
discovery.relabel "containers" {
  targets = discovery.docker.containers.targets
  rule {
    source_labels = ["__meta_docker_container_label_com_docker_compose_service"]
    target_label  = "service_name"
  }
}

// Prometheus metrics scraping
prometheus.scrape "containers" {
  targets    = discovery.relabel.containers.output
  forward_to = [prometheus.remote_write.lgtm.receiver]
}

prometheus.remote_write "lgtm" {
  endpoint {
    url = "http://lgtm:9090/api/v1/write"
  }
}

// Loki log collection
loki.source.docker "containers" {
  host       = "unix:///var/run/docker.sock"
  targets    = discovery.relabel.containers.output
  forward_to = [loki.write.lgtm.receiver]
}

loki.write "lgtm" {
  endpoint {
    url = "http://lgtm:3100/loki/api/v1/push"
  }
}

// OTLP trace receiver (for Zipkin compatibility)
otelcol.receiver.zipkin "default" {
  endpoint = "0.0.0.0:9411"
  output {
    traces = [otelcol.processor.batch.default.input]
  }
}

otelcol.processor.batch "default" {
  output {
    traces = [otelcol.exporter.otlp.lgtm.input]
  }
}

otelcol.exporter.otlp "lgtm" {
  client {
    endpoint = "lgtm:4317"
    tls {
      insecure = true
    }
  }
}
```

### Grafana Dashboard Provisioning

Create `docker/grafana/dashboards.yaml`:

```yaml
apiVersion: 1
providers:
  - name: "default"
    org_id: 1
    folder: ""
    type: "file"
    options:
      path: /var/lib/grafana/dashboards
```

Create custom dashboards in `docker/grafana/dashboards/` as JSON files. The LGTM stack includes pre-built dashboards for:

- **Tempo** - Trace exploration and service maps
- **Loki** - Log aggregation and search
- **Mimir** - Prometheus-compatible metrics

### Accessing Observability Tools

| Tool | URL | Purpose |
|------|-----|---------|
| Grafana | http://localhost:3000 | Unified UI for metrics, logs, traces |
| Alloy | http://localhost:12345 | Pipeline status and debugging |

In Grafana, use:
- **Explore → Tempo** for distributed traces
- **Explore → Loki** for container logs
- **Explore → Mimir** for Prometheus metrics

---

## 12. Configuration

### Config File Structure (`config/config.yaml`)

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  shutdown_timeout: 30s

database:
  host: "postgres"
  port: 5432
  name: "redoubt"
  user: "redoubt"
  password: "${POSTGRES_PASSWORD}"
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 5m

redis:
  host: "redis"
  port: 6379
  password: ""
  db: 0

auth:
  jwt_secret: "${JWT_SECRET}"
  jwt_expiry: 15m
  refresh_expiry: 720h  # 30 days
  password_min_length: 12
  lockout_threshold: 5
  lockout_duration: 15m

email:
  provider: "resend"
  api_key: "${RESEND_API_KEY}"
  from_address: "noreply@example.com"
  from_name: "Redoubt"
  verification_expiry: 24h
  reset_expiry: 1h

storage:
  endpoint: "http://localstack:4566"
  bucket: "redoubt-media"
  region: "us-east-1"
  access_key: "${S3_ACCESS_KEY}"
  secret_key: "${S3_SECRET_KEY}"
  encryption:
    master_key: "${STORAGE_MASTER_KEY}"

cors:
  allowed_origins:
    - "https://app.example.com"
  allowed_methods:
    - GET
    - POST
    - PUT
    - PATCH
    - DELETE
  allowed_headers:
    - Authorization
    - Content-Type
    - X-Request-ID

logging:
  level: "info"
  format: "json"

telemetry:
  service_name: "redoubt-api"
  service_version: "${VERSION:-0.1.0}"
  tracing:
    enabled: true
    sample_rate: 0.1  # 10% sampling in production
    otlp_endpoint: "lgtm:4317"  # OTLP gRPC endpoint (Grafana LGTM)
  metrics:
    enabled: true
    port: 9090  # Prometheus metrics port
```

### Environment Variables

Required in production:

```bash
POSTGRES_PASSWORD=
JWT_SECRET=
RESEND_API_KEY=
S3_ACCESS_KEY=
S3_SECRET_KEY=
STORAGE_MASTER_KEY=
```

---

## 13. Development Tooling

### Justfile

```just
# Default recipe
default:
    @just --list

# Run development server with hot reload
dev:
    docker compose -f docker-compose.yml -f docker-compose.dev.yml up

# Run production stack
up:
    docker compose up -d

# Stop all services
down:
    docker compose down

# View logs
logs service="redoubt-api":
    docker compose logs -f {{service}}

# Run database migrations
migrate-up:
    docker compose exec redoubt-api migrate -path /app/internal/db/migrations -database "$DATABASE_URL" up

migrate-down:
    docker compose exec redoubt-api migrate -path /app/internal/db/migrations -database "$DATABASE_URL" down 1

migrate-create name:
    migrate create -ext sql -dir internal/db/migrations -seq {{name}}

# Generate sqlc code
sqlc:
    sqlc generate

# Run linter
lint:
    golangci-lint run ./...

# Run tests
test:
    go test -v -race -cover ./...

# Run integration tests
test-integration:
    go test -v -race -tags=integration ./...

# Build binary
build:
    go build -o bin/redoubt-api ./cmd/redoubt-api

# Generate OpenAPI spec
swagger:
    swag init -g cmd/redoubt-api/main.go -o docs

# Clean build artifacts
clean:
    rm -rf bin/ docs/
```

### Air Configuration (`.air.toml`)

```toml
root = "."
tmp_dir = "tmp"

[build]
cmd = "go build -o ./tmp/redoubt-api ./cmd/redoubt-api"
bin = "./tmp/redoubt-api"
include_ext = ["go", "yaml"]
exclude_dir = ["tmp", "vendor", "docs"]
delay = 1000

[log]
time = false

[color]
main = "magenta"
watcher = "cyan"
build = "yellow"
runner = "green"
```

### golangci-lint Configuration (`.golangci.yml`)

```yaml
run:
  timeout: 5m
  modules-download-mode: readonly

linters:
  enable:
    - errcheck
    - gosimple
    - govet
    - ineffassign
    - staticcheck
    - typecheck
    - unused
    - gofmt
    - goimports
    - misspell
    - unconvert
    - gocritic
    - revive
    - exportloopref
    - gosec
    - prealloc

linters-settings:
  govet:
    check-shadowing: true
  revive:
    rules:
      - name: exported
        disabled: true
  gosec:
    excludes:
      - G104  # Unhandled errors (too noisy)

issues:
  exclude-use-default: false
  max-issues-per-linter: 0
  max-same-issues: 0
```

---

## 14. Testing Strategy

### Test Structure

```
internal/
├── api/handlers/
│   ├── auth_test.go          # Handler unit tests
│   └── auth_integration_test.go
├── auth/
│   ├── jwt_test.go
│   └── password_test.go
└── testutil/
    ├── database.go           # testcontainers setup
    ├── fixtures.go           # Test data generators
    └── assertions.go         # Custom assertions
```

### Integration Tests with Testcontainers

```go
// +build integration

func TestAuthFlow(t *testing.T) {
    ctx := context.Background()

    // Start Postgres container
    pgContainer, err := postgres.RunContainer(ctx,
        testcontainers.WithImage("postgres:16-alpine"),
        postgres.WithDatabase("redoubt_test"),
    )
    require.NoError(t, err)
    defer pgContainer.Terminate(ctx)

    // Run migrations
    // Create app instance
    // Run tests
}
```

### Unit Tests

- Mock database with `sqlc` interfaces
- Mock Redis with `miniredis`
- Mock S3 with in-memory implementation
- Test business logic in isolation

### Test Coverage Goals

| Package | Target |
|---------|--------|
| `auth` | 90% |
| `handlers` | 80% |
| `space` | 85% |
| `storage` | 75% |

---

## 15. CI/CD Pipeline

### GitHub Actions (`.github/workflows/ci.yml`)

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v4
        with:
          version: latest

  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16-alpine
        env:
          POSTGRES_USER: test
          POSTGRES_PASSWORD: test
          POSTGRES_DB: redoubt_test
        ports:
          - 5432:5432
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
      redis:
        image: redis:7-alpine
        ports:
          - 6379:6379
        options: >-
          --health-cmd "redis-cli ping"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5

    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Install dependencies
        run: go mod download
      - name: Run tests
        env:
          DATABASE_URL: postgres://test:test@localhost:5432/redoubt_test?sslmode=disable
          REDIS_URL: redis://localhost:6379
        run: go test -v -race -coverprofile=coverage.out ./...
      - name: Upload coverage
        uses: codecov/codecov-action@v4
        with:
          files: ./coverage.out

  build:
    runs-on: ubuntu-latest
    needs: [lint, test]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Build
        run: go build -o bin/redoubt-api ./cmd/redoubt-api
      - name: Build Docker image
        run: docker build -t redoubt-api:${{ github.sha }} -f docker/Dockerfile .
```

---

## 16. Implementation Tasks

### Milestone 1: Project Setup

- [x] Initialize Go module (`go mod init github.com/redoubtapp/redoubt-api`)
- [x] Create directory structure
- [x] Set up Viper configuration
- [x] Create Docker Compose files (prod + dev)
- [x] Create Dockerfile (prod + dev)
- [x] Set up Air hot reload
- [x] Create Justfile
- [x] Set up golangci-lint
- [x] Initialize GitHub Actions CI
- [x] Create Grafana Alloy config (`docker/alloy/config.alloy`)
- [x] Create Grafana dashboard provisioning (`docker/grafana/dashboards.yaml`)

### Milestone 2: Database Layer

- [x] Write initial migration (`0001_initial.up.sql`)
- [x] Configure sqlc (`sqlc.yaml`)
- [x] Write sqlc queries for all tables
- [x] Generate sqlc code
- [x] Implement database connection with pgx
- [x] Add migration runner to startup

### Milestone 3: Core API & Observability

- [x] Implement HTTP server with graceful shutdown
- [x] Create gorilla/mux router
- [x] Implement middleware: logging, request ID, CORS
- [x] Implement middleware: security headers, panic recovery
- [x] Implement RFC 9457 error response helpers (`alpineworks/rfc9457`)
- [x] Initialize OpenTelemetry via `alpineworks/ootel`
- [x] Add `otelhttp` middleware for HTTP tracing and metrics
- [x] Instrument pgx with `otelpgx` for database tracing
- [x] Instrument Redis with `redisotel` for cache tracing
- [ ] Define custom application metrics (logins, registrations, etc.)
- [x] Implement health check endpoints (`/health`, `/health/live`, `/health/ready`)
- [x] Set up slog structured logging
- [ ] Add Swagger/OpenAPI generation

### Milestone 4: Authentication

- [x] Implement Argon2id password hashing
- [x] Implement JWT generation and validation
- [x] Implement refresh token handling
- [x] Create auth middleware
- [x] Implement registration endpoint
- [x] Implement login endpoint
- [x] Implement token refresh endpoint
- [x] Implement logout endpoint
- [x] Integrate Resend for email
- [x] Implement email verification flow
- [x] Implement password reset flow
- [x] Implement resend verification endpoint
- [x] Add account lockout logic
- [x] Implement auth service with full business logic

### Milestone 5: Rate Limiting & Redis

- [x] Add Redis to Docker Compose
- [x] Implement Redis client wrapper with OpenTelemetry
- [x] Implement sliding window rate limiter
- [x] Add rate limit middleware with IETF draft headers
- [x] Configure per-endpoint rate limits (register, login, forgot-password, verify-email)
- [ ] Implement general cache layer

### Milestone 6: Session Management

- [x] Implement session list endpoint (`GET /api/v1/sessions`)
- [x] Implement session revoke endpoint (`DELETE /api/v1/sessions/:id`)
- [x] Implement revoke all sessions endpoint (`DELETE /api/v1/sessions`)
- [x] Add session ID to JWT claims for current session detection
- [x] Wire up session handler and routes

### Milestone 7: Spaces & Channels

- [x] Implement Space CRUD endpoints
- [x] Implement auto-create default channels
- [x] Implement Channel CRUD endpoints
- [x] Implement channel reordering
- [x] Implement membership management
- [x] Implement role-based permissions

### Milestone 8: Invites

- [x] Implement EFF wordlist loader
- [x] Implement 4-word invite code generator
- [x] Implement invite creation endpoint
- [x] Implement invite join endpoint
- [x] Implement invite revocation
- [x] Implement bootstrap invite on first run

### Milestone 9: Media Storage

- [x] Add LocalStack to dev compose
- [x] Implement S3 client wrapper
- [x] Implement client-side encryption (AES-256-GCM)
- [x] Implement avatar upload endpoint
- [x] Implement avatar retrieval/proxy
- [x] Configure SSE on bucket (via client-side encryption)

### Milestone 10: User Management

- [x] Implement get current user endpoint
- [x] Implement update user endpoint
- [x] Implement soft delete account endpoint
- [x] Implement avatar management

### Milestone 11: Audit Logging

- [x] Create audit_logs table migration (included in 0001_initial)
- [x] Implement audit logging service
- [x] Log member kick events
- [x] Log role change events
- [x] Log channel/space delete events
- [x] Log invite revocation events

### Milestone 12: Testing

- [x] Set up testcontainers (dependencies added)
- [x] Write auth package unit tests (password, jwt, session, tokens)
- [x] Write middleware unit tests (auth, security, requestid, ratelimit)
- [x] Write config unit tests
- [x] Write cache unit tests
- [x] Write database unit tests (postgres, migrate)
- [x] Write errors package unit tests
- [x] Write handler tests (auth, health)
- [x] Write storage package tests (encryption, content type detection)
- [ ] Achieve coverage targets

### Milestone 13: Documentation

- [x] Generate OpenAPI spec (docs/openapi.yaml)
- [x] Write API usage examples (in OpenAPI spec)
- [x] Document configuration options (in README)
- [x] Write development setup guide (in README)

---

## 17. Acceptance Criteria

### Authentication

- [x] User can register with a valid invite code
- [x] User receives verification email via Resend
- [x] User cannot login until email is verified
- [x] User can login with correct credentials
- [x] User receives access + refresh tokens on login
- [x] Access token expires after 15 minutes
- [x] Refresh token works for 30 days
- [x] User can request password reset
- [x] Password reset email is sent
- [x] Password reset token expires after 1 hour
- [x] Account locks after 5 failed login attempts
- [x] Lockout lasts 15 minutes
- [x] First registered user becomes instance admin

### Sessions

- [x] User can list their active sessions
- [x] User can revoke individual sessions
- [x] User can revoke all sessions

### Spaces

- [x] Instance admin can create spaces
- [x] Space is created with default general text and General voice channels
- [x] Non-admin users cannot create spaces
- [x] Space admins can update space name/icon
- [x] Space owner can delete space (soft delete)
- [x] Deleted spaces are not visible in listings

### Channels

- [x] Space admins can create channels
- [x] Channel position can be set on creation
- [x] Channels can be reordered
- [x] Channels can be updated (name, type)
- [x] Channels can be deleted (soft delete)

### Memberships

- [x] Users can join spaces via invite code
- [x] Members are listed correctly
- [x] Admins can kick members
- [x] Owners can change member roles
- [x] Membership is removed when user is kicked

### Invites

- [x] Admins can create invite codes
- [x] Invite codes are 4 words from EFF wordlist
- [x] Invite codes are case-sensitive
- [x] Invites track usage count
- [x] Invites can have max uses limit
- [x] Invites can have expiration
- [x] Expired invites are rejected
- [x] Used-up invites are rejected
- [x] Admins can revoke invites
- [x] Bootstrap invite is generated on first run

### Media

- [x] Users can upload avatars
- [x] Avatars are encrypted before S3 upload
- [x] S3 has SSE enabled (via client-side AES-256-GCM encryption)
- [x] Avatars can be retrieved via API
- [x] Avatars are decrypted on retrieval
- [x] Users can delete their avatar

### Rate Limiting

- [x] Auth endpoints are rate limited
- [ ] General API is rate limited per user
- [x] Rate limit headers are returned
- [x] Exceeded limits return 429

### Error Responses

- [x] All error responses conform to RFC 9457 format
- [x] Error responses include `type`, `title`, `status`, `detail`, `instance`
- [x] Error responses include `request_id` extension
- [x] Validation errors include `errors` array with field-level details

### Observability

- [x] OpenTelemetry initialized via `alpineworks/ootel`
- [x] `/metrics` endpoint exposes Prometheus metrics
- [x] HTTP requests generate trace spans with correct attributes
- [x] Database queries generate trace spans via `otelpgx`
- [x] Redis commands generate trace spans via `redisotel`
- [ ] Custom application metrics are recorded (logins, registrations, etc.)
- [x] Trace context propagates through request lifecycle
- [ ] `X-Request-ID` is included in all trace spans

### Audit Logging

- [x] Admin actions are logged to `audit_logs` table
- [x] Member kicks are recorded with actor, target, and metadata
- [x] Role changes are recorded
- [x] Channel/Space deletions are recorded
- [x] Invite revocations are recorded
- [x] Logs include IP address of actor

### Health Checks

- [x] `/health` returns component status
- [x] `/health/live` returns 200 if process is running
- [x] `/health/ready` returns 200 if DB + Redis are connected

### Infrastructure

- [x] `docker compose up` starts all services
- [x] Services have health checks
- [x] Caddy terminates TLS and routes correctly
- [x] Hot reload works in development

---

## Summary

Phase 1 establishes a solid foundation for Redoubt with:

- **Secure authentication** using Argon2id, JWT, invite-only registration, email verification via Resend
- **Single instance admin model** with Space-level role delegation
- **Encrypted media storage** with client-side + server-side encryption
- **Redis-backed rate limiting** and general caching
- **Full observability** via OpenTelemetry tracing + Prometheus metrics (`alpineworks/ootel`)
- **Clean project structure** following Go best practices
- **Comprehensive testing** with testcontainers
- **Full CI/CD pipeline** with GitHub Actions
- **Developer-friendly tooling** with Justfile, Air, and golangci-lint

The architecture is designed to be simple for single-VPS deployment while maintaining security and extensibility for future phases.
