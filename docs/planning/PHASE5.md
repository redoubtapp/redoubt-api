# Redoubt — Phase 5 Implementation Document

**Status:** Ready for implementation
**Last updated:** 2026-02-27
**Author:** Michael

This document defines the complete scope, technical decisions, and implementation details for Phase 5 (Polish & Ship) of Redoubt.

---

## Table of Contents

- [1. Phase 5 Scope Summary](#1-phase-5-scope-summary)
- [2. Architecture Decisions](#2-architecture-decisions)
- [3. Admin Panel](#3-admin-panel)
- [4. Admin Panel Pages](#4-admin-panel-pages)
- [5. Admin Panel Authentication](#5-admin-panel-authentication)
- [6. Admin Panel Database Queries](#6-admin-panel-database-queries)
- [7. Installer Script](#7-installer-script)
- [8. Installer Script Flow](#8-installer-script-flow)
- [9. Production Monitoring Overlay](#9-production-monitoring-overlay)
- [10. CI/CD Release Pipeline](#10-cicd-release-pipeline)
- [11. Documentation](#11-documentation)
- [12. Configuration](#12-configuration)
- [13. Testing Strategy](#13-testing-strategy)
- [14. Implementation Tasks](#14-implementation-tasks)
- [15. Acceptance Criteria](#15-acceptance-criteria)

---

## 1. Phase 5 Scope Summary

Phase 5 prepares Redoubt for public release with the following deliverables:

| Component | Scope |
|-----------|-------|
| Admin Panel | Go + HTMX web panel on separate port, instance admin only, dashboard + user/space/audit management |
| Installer Script | `install.sh` for Ubuntu/Debian, Docker install, secret generation, DNS check, ufw, full bootstrap |
| Monitoring Overlay | Optional Prometheus + Grafana via Docker Compose profiles |
| CI/CD | GitHub Actions auto-build + push Docker images to GHCR on semver tags |
| Documentation | docs/INSTALL.md, docs/CONFIGURATION.md, docs/UPGRADING.md, docs/TROUBLESHOOTING.md |

**Deferred:**
- Backup sidecar (deferred to post-v1)
- Static docs site / GitHub Pages (deferred — separate docs/ files for now)
- Non-interactive / unattended installer mode
- Multi-distro support (RHEL, Arch, macOS)

---

## 2. Architecture Decisions

### Core Libraries & Frameworks

| Concern | Choice | Rationale |
|---------|--------|-----------|
| Admin templating | `html/template` (stdlib) | Zero dependencies, auto-escaping, well-understood |
| Admin styling | Pico CSS | Classless CSS, ~10KB, clean defaults, no build step |
| Admin interactivity | HTMX | Server-rendered HTML fragments, no JS framework needed |
| Admin auth | Separate session cookie | Decoupled from API JWT system, httpOnly cookie |
| Installer target | Ubuntu 22.04+ / Debian 12+ | Covers majority of VPS providers |
| Container registry | GitHub Container Registry (ghcr.io) | Free for public repos, integrated with GitHub Actions |

### Key Design Decisions

| Decision | Choice |
|----------|--------|
| Admin access | Instance admin only (no Space-scoped admin views) |
| Admin port | Separate internal port (9091), not exposed through Caddy |
| Admin access method | SSH tunnel or direct IP — not publicly routable |
| Admin CSS framework | Pico CSS (classless, single CSS file include) |
| Admin live updates | HTMX polling (`hx-trigger="every 30s"`) for dashboard stats |
| Admin auth mechanism | Separate session cookie, own login form |
| Admin user management | View, disable/ban, password reset, revoke sessions (no impersonation) |
| Admin metrics scope | Application metrics only (no host/Docker system stats) |
| Admin audit log | Paginated chronological list, click-to-expand detail |
| Admin config changes | Read-only — no config editing from UI, config stays in YAML/env |
| Installer Docker handling | Always install Docker via get.docker.com |
| Installer admin setup | Print bootstrap invite code after startup |
| Installer interactivity | Interactive only (no --non-interactive flag for v1) |
| Installer file source | Fetch from raw.githubusercontent.com (main branch) |
| Installer location | `/opt/redoubt` |
| Installer auto-updates | Manual only — documented upgrade command |
| Installer auto-start | Docker `restart: unless-stopped` policy (no systemd unit) |
| Installer firewall | Configure ufw: allow 80, 443, 22, 7881, WebRTC UDP range |
| Installer DNS check | Resolve domain, compare to server public IP, block on mismatch |
| Installer min specs | 2 vCPU / 4 GB RAM (warn if below) |
| Monitoring overlay | Docker Compose profiles (`--profile monitoring`) |
| Release artifacts | Docker images on GHCR only (no binary or tarball releases) |
| Release CI trigger | Auto-build on semver tags (v*.*.*) |
| Install URL | `https://raw.githubusercontent.com/redoubtapp/redoubt-api/main/install.sh` |

---

## 3. Admin Panel

### 3.1 Architecture

The admin panel runs as a separate HTTP server within the `redoubt-api` binary, listening on port 9091. It is **not** routed through Caddy and is not publicly accessible. Admins access it via SSH tunnel (`ssh -L 9091:localhost:9091 user@server`) or direct IP on a private network.

```
┌─────────────────────────────────────────────────┐
│                 redoubt-api binary                │
│                                                   │
│  ┌─────────────────┐   ┌──────────────────────┐  │
│  │  REST API :8080  │   │  Admin Panel :9091    │  │
│  │  (JSON, JWT)     │   │  (HTML, session cookie)│  │
│  └─────────────────┘   └──────────────────────┘  │
└─────────────────────────────────────────────────┘
         │                         │
    Caddy → public            SSH tunnel → private
```

### 3.2 Package Structure

```
internal/
├── admin/
│   ├── server.go          # HTTP server setup, routes, middleware
│   ├── handlers.go        # Page handlers (dashboard, users, spaces, audit)
│   ├── session.go         # Session cookie management
│   ├── templates.go       # Template loading and rendering helpers
│   └── templates/
│       ├── layout.html    # Base layout (head, nav, footer)
│       ├── login.html     # Login page
│       ├── dashboard.html # Dashboard with stats
│       ├── users.html     # User list + detail
│       ├── spaces.html    # Space list + detail
│       ├── audit.html     # Audit log viewer
│       └── partials/
│           ├── stats.html     # Dashboard stats (HTMX partial)
│           ├── user_row.html  # User table row
│           ├── space_row.html # Space table row
│           └── audit_row.html # Audit log entry row
```

### 3.3 Technology Stack

**Pico CSS** is included via a vendored copy in the binary (embedded via `//go:embed`). No CDN dependency. HTMX is similarly embedded — a single `htmx.min.js` file.

```go
//go:embed static/pico.min.css static/htmx.min.js
var staticFiles embed.FS
```

**Template rendering** uses `html/template` with a base layout pattern:

```go
// internal/admin/templates.go

type TemplateData struct {
    Title     string
    Nav       string      // active nav item
    Content   interface{} // page-specific data
    CSRFToken string
}

func (s *Server) render(w http.ResponseWriter, tmpl string, data TemplateData) {
    t := template.Must(template.ParseFS(templateFS, "templates/layout.html", "templates/"+tmpl))
    t.Execute(w, data)
}

// For HTMX partials (no layout wrapper)
func (s *Server) renderPartial(w http.ResponseWriter, tmpl string, data interface{}) {
    t := template.Must(template.ParseFS(templateFS, "templates/partials/"+tmpl))
    t.Execute(w, data)
}
```

### 3.4 Routes

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/login` | Login page | No |
| POST | `/login` | Authenticate, set session cookie | No |
| POST | `/logout` | Clear session cookie | Yes |
| GET | `/` | Dashboard (redirects to /login if unauthenticated) | Yes |
| GET | `/users` | User list | Yes |
| GET | `/users/:id` | User detail | Yes |
| POST | `/users/:id/disable` | Disable user account | Yes |
| POST | `/users/:id/enable` | Re-enable user account | Yes |
| POST | `/users/:id/reset-password` | Trigger password reset email | Yes |
| POST | `/users/:id/revoke-sessions` | Revoke all sessions for user | Yes |
| GET | `/spaces` | Space list | Yes |
| GET | `/spaces/:id` | Space detail (members, channels, invites) | Yes |
| POST | `/spaces/:id/delete` | Delete space | Yes |
| POST | `/spaces/:id/invites/:inviteId/revoke` | Revoke invite | Yes |
| GET | `/audit` | Audit log list | Yes |
| GET | `/partials/stats` | Dashboard stats (HTMX partial) | Yes |

---

## 4. Admin Panel Pages

### 4.1 Dashboard

The dashboard is the landing page after login. It displays application-level statistics with HTMX polling for live updates.

**Stats displayed:**

| Stat | Source | Update |
|------|--------|--------|
| Total users | `SELECT COUNT(*) FROM users WHERE deleted_at IS NULL` | HTMX poll 30s |
| Online users | WebSocket hub connection count | HTMX poll 30s |
| Total spaces | `SELECT COUNT(*) FROM spaces WHERE deleted_at IS NULL` | HTMX poll 30s |
| Total channels | `SELECT COUNT(*) FROM channels WHERE deleted_at IS NULL` | HTMX poll 30s |
| Active voice participants | LiveKit room service participant count | HTMX poll 30s |
| Messages today | `SELECT COUNT(*) FROM messages WHERE created_at > NOW() - INTERVAL '24 hours'` | HTMX poll 30s |
| Active WebSocket connections | WebSocket hub connection count | HTMX poll 30s |
| Server uptime | Go `time.Since(startTime)` | HTMX poll 30s |
| Go version | `runtime.Version()` | Static |
| API version | Build-time version string | Static |

**HTMX polling implementation:**

```html
<!-- dashboard.html -->
<div id="live-stats" hx-get="/partials/stats" hx-trigger="every 30s" hx-swap="innerHTML">
    {{template "partials/stats.html" .Content}}
</div>
```

### 4.2 User Management

**User list page:**
- Paginated table: username, email, status (active/disabled/deleted), role (instance admin or not), created date
- 25 users per page, cursor-based pagination
- Click username → user detail page

**User detail page:**
- User info: username, email, avatar, created_at, last login
- Status badge: active / disabled / deleted
- Instance admin badge
- Active sessions list (with "Revoke All" button)
- Space memberships list
- Action buttons:
  - Disable / Enable account
  - Send password reset email
  - Revoke all sessions

**Disable behavior:**
- Sets a `disabled_at` timestamp on the user record
- Revokes all active sessions immediately
- User cannot log in while disabled
- Logged in audit trail

```sql
-- Migration: 0005_admin_panel.up.sql

ALTER TABLE users ADD COLUMN disabled_at TIMESTAMPTZ;
CREATE INDEX idx_users_disabled ON users(disabled_at) WHERE disabled_at IS NOT NULL;
```

### 4.3 Space Management

**Space list page:**
- Paginated table: name, owner, member count, channel count, created date
- 25 spaces per page
- Click name → space detail page

**Space detail page:**
- Space info: name, icon, owner, created_at
- Member list with roles
- Channel list with types
- Active invites with usage stats
- Action buttons:
  - Delete space (with confirmation)
  - Revoke individual invites

### 4.4 Audit Log Viewer

**Audit log page:**
- Paginated chronological list (newest first)
- 50 entries per page, cursor-based pagination
- Each entry shows: timestamp, actor username, action, target type, target identifier
- Click entry → expanded detail with full metadata JSON

**Entry display format:**
```
2026-02-27 14:30:22  alice  member.kick  user  bob
2026-02-27 14:28:15  alice  channel.delete  channel  off-topic
2026-02-27 14:25:00  admin  member.role_change  user  charlie
```

---

## 5. Admin Panel Authentication

### 5.1 Session Mechanism

The admin panel uses its own session system, completely separate from the API's JWT auth. Sessions are stored in a dedicated database table.

```sql
-- Part of migration: 0005_admin_panel.up.sql

CREATE TABLE admin_sessions (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token       VARCHAR(64) NOT NULL UNIQUE,
    ip_address  INET,
    user_agent  TEXT,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_admin_sessions_token ON admin_sessions(token);
CREATE INDEX idx_admin_sessions_expires ON admin_sessions(expires_at);
```

### 5.2 Login Flow

```
1. Admin navigates to http://localhost:9091/login (via SSH tunnel)
2. Submits username/email + password
3. Server verifies credentials (same Argon2id check as API)
4. Server checks is_instance_admin = true
5. If not instance admin → reject with "Access denied"
6. Generate random 64-byte session token
7. Store in admin_sessions table (expires: 24 hours)
8. Set httpOnly, Secure, SameSite=Strict cookie: redoubt_admin_session=<token>
9. Redirect to /
```

### 5.3 Session Middleware

```go
// internal/admin/session.go

func (s *Server) requireAdmin(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        cookie, err := r.Cookie("redoubt_admin_session")
        if err != nil {
            http.Redirect(w, r, "/login", http.StatusSeeOther)
            return
        }

        session, err := s.queries.GetAdminSession(r.Context(), cookie.Value)
        if err != nil || session.ExpiresAt.Before(time.Now()) {
            http.Redirect(w, r, "/login", http.StatusSeeOther)
            return
        }

        user, err := s.queries.GetUserByID(r.Context(), session.UserID)
        if err != nil || !user.IsInstanceAdmin {
            http.Redirect(w, r, "/login", http.StatusSeeOther)
            return
        }

        ctx := context.WithValue(r.Context(), adminUserKey, user)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

### 5.4 Security Considerations

- Session cookie flags: `httpOnly`, `SameSite=Strict`, `Path=/`
- `Secure` flag set only when not on localhost (to support SSH tunnel access)
- Session expiry: 24 hours (no refresh mechanism — re-login required)
- CSRF protection: per-session CSRF token embedded in forms, validated on POST
- Rate limit login attempts: 5 per 15 minutes per IP (reuse existing rate limiter)
- Expired sessions cleaned up by a periodic background goroutine (every hour)

---

## 6. Admin Panel Database Queries

### 6.1 Dashboard Queries

```sql
-- queries/admin.sql

-- name: AdminCountUsers :one
SELECT COUNT(*) FROM users WHERE deleted_at IS NULL;

-- name: AdminCountSpaces :one
SELECT COUNT(*) FROM spaces WHERE deleted_at IS NULL;

-- name: AdminCountChannels :one
SELECT COUNT(*) FROM channels WHERE deleted_at IS NULL;

-- name: AdminCountMessagesToday :one
SELECT COUNT(*) FROM messages
WHERE created_at > NOW() - INTERVAL '24 hours'
  AND deleted_at IS NULL;
```

### 6.2 User Management Queries

```sql
-- name: AdminListUsers :many
SELECT id, username, email, avatar_url, is_instance_admin,
       email_verified, disabled_at, deleted_at, created_at
FROM users
WHERE deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: AdminGetUser :one
SELECT id, username, email, avatar_url, is_instance_admin,
       email_verified, disabled_at, deleted_at, created_at, updated_at
FROM users
WHERE id = $1;

-- name: AdminDisableUser :exec
UPDATE users SET disabled_at = NOW(), updated_at = NOW()
WHERE id = $1 AND disabled_at IS NULL;

-- name: AdminEnableUser :exec
UPDATE users SET disabled_at = NULL, updated_at = NOW()
WHERE id = $1 AND disabled_at IS NOT NULL;

-- name: AdminGetUserSpaces :many
SELECT s.id, s.name, s.icon_url, m.role, m.joined_at
FROM memberships m
JOIN spaces s ON s.id = m.space_id
WHERE m.user_id = $1 AND s.deleted_at IS NULL
ORDER BY m.joined_at DESC;

-- name: AdminGetUserSessions :many
SELECT id, user_agent, ip_address, last_used_at, created_at
FROM sessions
WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > NOW()
ORDER BY last_used_at DESC;
```

### 6.3 Space Management Queries

```sql
-- name: AdminListSpaces :many
SELECT s.id, s.name, s.icon_url, s.owner_id, s.created_at,
       u.username AS owner_username,
       (SELECT COUNT(*) FROM memberships WHERE space_id = s.id) AS member_count,
       (SELECT COUNT(*) FROM channels WHERE space_id = s.id AND deleted_at IS NULL) AS channel_count
FROM spaces s
JOIN users u ON u.id = s.owner_id
WHERE s.deleted_at IS NULL
ORDER BY s.created_at DESC
LIMIT $1 OFFSET $2;

-- name: AdminGetSpaceDetail :one
SELECT s.id, s.name, s.icon_url, s.owner_id, s.created_at,
       u.username AS owner_username
FROM spaces s
JOIN users u ON u.id = s.owner_id
WHERE s.id = $1;

-- name: AdminGetSpaceMembers :many
SELECT u.id, u.username, u.email, u.avatar_url, m.role, m.joined_at
FROM memberships m
JOIN users u ON u.id = m.user_id
WHERE m.space_id = $1
ORDER BY m.role ASC, m.joined_at ASC;

-- name: AdminGetSpaceChannels :many
SELECT id, name, type, position, created_at
FROM channels
WHERE space_id = $1 AND deleted_at IS NULL
ORDER BY position ASC;

-- name: AdminGetSpaceInvites :many
SELECT i.id, i.code, i.uses, i.max_uses, i.expires_at, i.revoked_at,
       i.created_at, u.username AS created_by_username
FROM invites i
JOIN users u ON u.id = i.created_by
WHERE i.space_id = $1
ORDER BY i.created_at DESC;
```

### 6.4 Audit Log Queries

```sql
-- name: AdminListAuditLogs :many
SELECT a.id, a.action, a.target_type, a.target_id, a.metadata,
       a.ip_address, a.created_at,
       u.username AS actor_username
FROM audit_logs a
JOIN users u ON u.id = a.actor_id
ORDER BY a.created_at DESC
LIMIT $1 OFFSET $2;

-- name: AdminGetAuditLogEntry :one
SELECT a.id, a.actor_id, a.action, a.target_type, a.target_id,
       a.metadata, a.ip_address, a.created_at,
       u.username AS actor_username
FROM audit_logs a
JOIN users u ON u.id = a.actor_id
WHERE a.id = $1;

-- name: AdminCountAuditLogs :one
SELECT COUNT(*) FROM audit_logs;
```

### 6.5 Admin Session Queries

```sql
-- name: CreateAdminSession :one
INSERT INTO admin_sessions (user_id, token, ip_address, user_agent, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: GetAdminSession :one
SELECT id, user_id, token, expires_at, created_at
FROM admin_sessions
WHERE token = $1 AND expires_at > NOW();

-- name: DeleteAdminSession :exec
DELETE FROM admin_sessions WHERE token = $1;

-- name: DeleteExpiredAdminSessions :exec
DELETE FROM admin_sessions WHERE expires_at < NOW();
```

---

## 7. Installer Script

### 7.1 Overview

The installer is a single Bash script (`install.sh`) hosted at the repo root. It targets Ubuntu 22.04+ and Debian 12+ on amd64 and arm64 architectures. The script is interactive — it prompts the user for required information and provides clear, colored output.

**Install command:**
```bash
curl -fsSL https://raw.githubusercontent.com/redoubtapp/redoubt-api/main/install.sh | bash
```

**Install directory:** `/opt/redoubt`

### 7.2 Prerequisites Checked

| Check | Action on Failure |
|-------|-------------------|
| Running as root or with sudo | Exit with error |
| OS is Ubuntu 22.04+ or Debian 12+ | Exit with error |
| Architecture is amd64 or arm64 | Exit with error |
| Minimum 2 vCPU | Warn and continue |
| Minimum 4 GB RAM | Warn and continue |
| Port 80 is available | Exit with error (suggest checking for existing web servers) |
| Port 443 is available | Exit with error |
| `curl` is installed | Install via apt |

### 7.3 What the Script Does

1. **System checks** — OS, architecture, memory, CPU, port availability
2. **Install Docker** — via `get.docker.com` (skips if already installed and recent enough)
3. **Prompt for domain** — e.g., `chat.example.com`
4. **DNS verification** — resolve domain, compare to server's public IP (via `curl -s ifconfig.me`), block on mismatch
5. **Prompt for email** — used for Let's Encrypt certificate notifications
6. **Generate secrets** — all cryptographic material generated locally:
   - `POSTGRES_PASSWORD` — 32 random alphanumeric chars
   - `JWT_SECRET` — 64 random bytes, base64-encoded
   - `STORAGE_MASTER_KEY` — 32 random bytes, base64-encoded
   - `LIVEKIT_API_KEY` — `API` + 12 random alphanumeric chars
   - `LIVEKIT_API_SECRET` — 32 random bytes, base64-encoded
   - `ADMIN_SESSION_SECRET` — 32 random bytes, base64-encoded
7. **Create install directory** — `/opt/redoubt`
8. **Download files** — from `raw.githubusercontent.com/redoubtapp/redoubt-api/main/`:
   - `docker-compose.yml`
   - `Caddyfile`
   - `config/config.yaml`
   - `config/livekit.yaml`
9. **Write `.env` file** — populated with domain, email, and generated secrets
10. **Configure Caddy** — substitute domain placeholder in Caddyfile
11. **Configure ufw** — allow ports 22, 80, 443, 7881 (TCP), 60000-60100 (UDP for WebRTC)
12. **Pull images** — `docker compose pull`
13. **Start services** — `docker compose up -d`
14. **Wait for health** — poll the health endpoint until all services are healthy (timeout: 120s)
15. **Print bootstrap info** — read bootstrap invite code from API logs, display connection instructions

### 7.4 Secret Generation

All secrets are generated using `openssl rand` or `/dev/urandom`:

```bash
generate_secret() {
    local length=${1:-32}
    openssl rand -base64 "$length" | tr -d '\n'
}

generate_alphanumeric() {
    local length=${1:-32}
    tr -dc 'A-Za-z0-9' < /dev/urandom | head -c "$length"
}

POSTGRES_PASSWORD=$(generate_alphanumeric 32)
JWT_SECRET=$(generate_secret 64)
STORAGE_MASTER_KEY=$(generate_secret 32)
LIVEKIT_API_KEY="API$(generate_alphanumeric 12)"
LIVEKIT_API_SECRET=$(generate_secret 32)
ADMIN_SESSION_SECRET=$(generate_secret 32)
```

### 7.5 UFW Configuration

```bash
configure_firewall() {
    if ! command -v ufw &>/dev/null; then
        apt-get install -y ufw
    fi

    # Reset to defaults
    ufw default deny incoming
    ufw default allow outgoing

    # Allow SSH (prevent lockout)
    ufw allow 22/tcp comment "SSH"

    # Allow HTTP and HTTPS (Caddy)
    ufw allow 80/tcp comment "HTTP"
    ufw allow 443/tcp comment "HTTPS"

    # Allow LiveKit RTC over TCP
    ufw allow 7881/tcp comment "LiveKit RTC TCP"

    # Allow LiveKit TURN/UDP
    ufw allow 7882/udp comment "LiveKit TURN UDP"

    # Allow WebRTC UDP media port range
    ufw allow 60000:60100/udp comment "WebRTC media"

    # Enable firewall
    ufw --force enable
}
```

### 7.6 DNS Verification

```bash
verify_dns() {
    local domain="$1"

    local server_ip
    server_ip=$(curl -s --max-time 5 ifconfig.me)

    local domain_ip
    domain_ip=$(dig +short "$domain" A | head -1)

    if [[ -z "$domain_ip" ]]; then
        error "Could not resolve '$domain'. Make sure the DNS A record is configured."
        error "Point '$domain' to this server's IP: $server_ip"
        exit 1
    fi

    if [[ "$domain_ip" != "$server_ip" ]]; then
        error "DNS mismatch: '$domain' resolves to $domain_ip, but this server's IP is $server_ip"
        error "Update the DNS A record for '$domain' to point to $server_ip"
        error "DNS changes can take up to 48 hours to propagate, but usually take 5-15 minutes."
        exit 1
    fi

    success "DNS verified: $domain → $server_ip"
}
```

### 7.7 Generated `.env` File

```bash
# Generated by Redoubt installer on $(date -u +"%Y-%m-%dT%H:%M:%SZ")
# Do not edit unless you know what you're doing.

# Domain
DOMAIN=${DOMAIN}

# Email (for Let's Encrypt notifications)
ACME_EMAIL=${ACME_EMAIL}

# Database
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}

# Authentication
JWT_SECRET=${JWT_SECRET}

# Storage encryption
STORAGE_MASTER_KEY=${STORAGE_MASTER_KEY}

# LiveKit
LIVEKIT_API_KEY=${LIVEKIT_API_KEY}
LIVEKIT_API_SECRET=${LIVEKIT_API_SECRET}

# Admin panel
ADMIN_SESSION_SECRET=${ADMIN_SESSION_SECRET}

# Email (configure for password resets and email verification)
# RESEND_API_KEY=re_your_api_key_here

# S3 storage (configure for avatar/file uploads)
# S3_ACCESS_KEY=your_access_key
# S3_SECRET_KEY=your_secret_key
```

### 7.8 Post-Install Output

After successful installation, the script prints:

```
╔══════════════════════════════════════════════════════════════╗
║                    Redoubt is running!                       ║
╠══════════════════════════════════════════════════════════════╣
║                                                              ║
║  Your instance is live at:                                   ║
║    https://chat.example.com                                  ║
║                                                              ║
║  Bootstrap invite code:                                      ║
║    alpha-bravo-charlie-delta                                 ║
║                                                              ║
║  Use this code to register the first user.                   ║
║  The first user becomes the instance administrator.          ║
║                                                              ║
║  Admin panel (via SSH tunnel):                               ║
║    ssh -L 9091:localhost:9091 user@your-server               ║
║    Then open: http://localhost:9091                           ║
║                                                              ║
║  Useful commands:                                            ║
║    cd /opt/redoubt                                           ║
║    docker compose logs -f          # View logs               ║
║    docker compose restart          # Restart services         ║
║    docker compose pull && \                                   ║
║      docker compose up -d          # Upgrade                 ║
║                                                              ║
║  Config: /opt/redoubt/.env                                   ║
║  Data:   Docker volumes (postgres_data, redis_data, etc.)    ║
║                                                              ║
║  Documentation:                                              ║
║    https://github.com/redoubtapp/redoubt-api/docs            ║
║                                                              ║
╚══════════════════════════════════════════════════════════════╝
```

### 7.9 Idempotency

The script is safe to re-run:

| Action | Behavior on Re-run |
|--------|--------------------|
| Docker install | Skipped if Docker >= 24.0 is present |
| Directory creation | Skipped if `/opt/redoubt` exists |
| File download | Overwrites existing files (preserves `.env`) |
| `.env` generation | **Skipped** if `.env` already exists (preserves secrets) |
| UFW rules | Idempotent (ufw handles duplicates) |
| `docker compose up` | Recreates containers if images changed |

The script detects an existing installation and asks the user: "An existing Redoubt installation was found. Do you want to upgrade? (y/n)". On upgrade, it preserves `.env`, pulls new images, and restarts.

---

## 8. Installer Script Flow

### 8.1 Complete Flow Diagram

```
start
  │
  ├─ Check: running as root?
  │   └─ No → exit "Run with sudo"
  │
  ├─ Check: OS = Ubuntu 22.04+ or Debian 12+?
  │   └─ No → exit "Unsupported OS"
  │
  ├─ Check: arch = amd64 or arm64?
  │   └─ No → exit "Unsupported architecture"
  │
  ├─ Check: 2+ vCPU?
  │   └─ No → warn "Recommended: 2+ vCPU"
  │
  ├─ Check: 4+ GB RAM?
  │   └─ No → warn "Recommended: 4+ GB RAM"
  │
  ├─ Check: port 80 available?
  │   └─ No → exit "Port 80 in use by: <process>"
  │
  ├─ Check: port 443 available?
  │   └─ No → exit "Port 443 in use by: <process>"
  │
  ├─ Check: existing install at /opt/redoubt?
  │   └─ Yes → prompt "Upgrade existing install? (y/n)"
  │       ├─ y → upgrade flow (pull, restart, preserve .env)
  │       └─ n → exit
  │
  ├─ Install Docker (if not present)
  │   └─ curl -fsSL https://get.docker.com | bash
  │
  ├─ Install dependencies (if not present)
  │   └─ apt-get install -y curl dnsutils ufw
  │
  ├─ Prompt: "Enter your domain name:"
  │
  ├─ Verify DNS → domain resolves to this server's IP
  │   └─ Mismatch → exit with instructions
  │
  ├─ Prompt: "Enter email for TLS certificates:"
  │
  ├─ Generate all secrets
  │
  ├─ Create /opt/redoubt
  │
  ├─ Download: docker-compose.yml, Caddyfile, configs
  │
  ├─ Write .env
  │
  ├─ Configure ufw
  │
  ├─ docker compose pull
  │
  ├─ docker compose up -d
  │
  ├─ Wait for health check (timeout: 120s)
  │   └─ Timeout → show logs, suggest checking Docker logs
  │
  ├─ Extract bootstrap invite code from API logs
  │
  └─ Print success banner with invite code + instructions
```

### 8.2 Error Handling

All errors are printed in red with clear next-step instructions:

```bash
error() { echo -e "\033[0;31m✗ ERROR: $1\033[0m"; }
warn()  { echo -e "\033[0;33m⚠ WARNING: $1\033[0m"; }
info()  { echo -e "\033[0;36m→ $1\033[0m"; }
success() { echo -e "\033[0;32m✓ $1\033[0m"; }
```

If any critical step fails, the script exits immediately with a descriptive error and does not leave partial state. Docker containers are stopped and removed on failure during first install.

### 8.3 Upgrade Flow

When an existing installation is detected:

```
1. Confirm with user
2. cd /opt/redoubt
3. docker compose down (graceful stop)
4. Re-download docker-compose.yml, Caddyfile, configs (NOT .env)
5. docker compose pull
6. docker compose up -d
7. Wait for health check
8. Print "Upgrade complete" with version info
```

---

## 9. Production Monitoring Overlay

### 9.1 Docker Compose Profiles

Monitoring services are included in the main `docker-compose.yml` but gated behind a Docker Compose profile. They only start when explicitly requested:

```bash
# Start with monitoring
docker compose --profile monitoring up -d

# Start without monitoring (default)
docker compose up -d
```

### 9.2 Monitoring Services

```yaml
# Added to docker-compose.yml

  prometheus:
    image: prom/prometheus:v2.50.1
    profiles: ["monitoring"]
    volumes:
      - ./config/prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus_data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.retention.time=30d'
    restart: unless-stopped

  grafana:
    image: grafana/grafana:10.3.1
    profiles: ["monitoring"]
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=${GRAFANA_PASSWORD:-admin}
      - GF_USERS_ALLOW_SIGN_UP=false
    volumes:
      - grafana_data:/var/lib/grafana
      - ./config/grafana/provisioning:/etc/grafana/provisioning:ro
    depends_on:
      - prometheus
    restart: unless-stopped

volumes:
  prometheus_data:
  grafana_data:
```

### 9.3 Prometheus Configuration

```yaml
# config/prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'redoubt-api'
    static_configs:
      - targets: ['redoubt-api:9090']

  - job_name: 'livekit'
    static_configs:
      - targets: ['livekit:7880']
    metrics_path: '/metrics'

  - job_name: 'caddy'
    static_configs:
      - targets: ['caddy:2019']
    metrics_path: '/metrics'
```

### 9.4 Grafana Provisioning

Pre-configured data source and dashboard:

```yaml
# config/grafana/provisioning/datasources/prometheus.yml
apiVersion: 1
datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://prometheus:9090
    isDefault: true
```

---

## 10. CI/CD Release Pipeline

### 10.1 GitHub Actions Workflow

A new workflow file builds and pushes Docker images to GHCR on semver tag pushes:

```yaml
# .github/workflows/release.yml
name: Release

on:
  push:
    tags:
      - 'v*.*.*'

permissions:
  contents: read
  packages: write

jobs:
  build-and-push:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Extract version
        id: version
        run: echo "version=${GITHUB_REF_NAME#v}" >> "$GITHUB_OUTPUT"

      - name: Build and push
        uses: docker/build-push-action@v5
        with:
          context: .
          file: docker/Dockerfile
          push: true
          platforms: linux/amd64,linux/arm64
          tags: |
            ghcr.io/redoubtapp/redoubt-api:${{ steps.version.outputs.version }}
            ghcr.io/redoubtapp/redoubt-api:latest
          build-args: |
            VERSION=${{ steps.version.outputs.version }}
            COMMIT=${{ github.sha }}
          cache-from: type=gha
          cache-to: type=gha,mode=max

      - name: Create GitHub Release
        uses: softprops/action-gh-release@v1
        with:
          generate_release_notes: true
```

### 10.2 Docker Image Tagging

| Tag | When |
|-----|------|
| `ghcr.io/redoubtapp/redoubt-api:1.0.0` | On push of `v1.0.0` tag |
| `ghcr.io/redoubtapp/redoubt-api:latest` | Always updated to the latest semver tag |

### 10.3 Build Arguments

The Dockerfile accepts build-time version info:

```dockerfile
ARG VERSION=dev
ARG COMMIT=unknown

RUN go build -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /redoubt-api ./cmd/redoubt-api
```

### 10.4 Production docker-compose.yml Image Reference

For production (installed via `install.sh`), the docker-compose.yml references the GHCR image instead of building locally:

```yaml
services:
  redoubt-api:
    image: ghcr.io/redoubtapp/redoubt-api:latest
    # ... rest of config
```

The installer downloads a production-specific `docker-compose.yml` that uses `image:` instead of `build:`.

---

## 11. Documentation

### 11.1 Documentation Files

| File | Purpose |
|------|---------|
| `docs/INSTALL.md` | Step-by-step installation guide |
| `docs/CONFIGURATION.md` | All configuration options reference |
| `docs/UPGRADING.md` | How to upgrade between versions |
| `docs/TROUBLESHOOTING.md` | Common problems and solutions |

### 11.2 INSTALL.md Structure

```
# Installation Guide

## Requirements
- VPS with Ubuntu 22.04+ or Debian 12+ (amd64 or arm64)
- 2+ vCPU, 4+ GB RAM
- Public IP address
- Domain name with A record pointing to the server
- Ports 80, 443 available

## Quick Install
curl -fsSL https://raw.githubusercontent.com/redoubtapp/redoubt-api/main/install.sh | bash

## What the Installer Does
(summary of each step)

## Manual Installation
(step-by-step for users who want to understand or customize)
1. Install Docker
2. Create /opt/redoubt directory
3. Download files
4. Generate secrets
5. Configure .env
6. Configure firewall
7. Start services

## First User Setup
1. Copy the bootstrap invite code from the installer output
2. Download the Redoubt client app
3. Register with the invite code
4. First registered user becomes the instance administrator

## Admin Panel Access
1. SSH tunnel: ssh -L 9091:localhost:9091 user@your-server
2. Open http://localhost:9091 in your browser
3. Log in with your instance admin credentials

## Post-Install Configuration
- Setting up email (Resend API key)
- Setting up S3 storage for avatars/attachments
- Enabling monitoring

## Uninstalling
(clean removal steps)
```

### 11.3 CONFIGURATION.md Structure

```
# Configuration Reference

## Environment Variables (.env)
(table of all env vars with descriptions, defaults, required/optional)

## Config File (config.yaml)
(YAML config file structure with all options documented)

## LiveKit Configuration (livekit.yaml)
(LiveKit-specific settings)

## Caddy Configuration (Caddyfile)
(Reverse proxy settings, custom routes)

## Docker Compose Overrides
(How to customize the compose stack)

## Monitoring
(How to enable the monitoring profile)
```

### 11.4 UPGRADING.md Structure

```
# Upgrading Redoubt

## Standard Upgrade
cd /opt/redoubt
docker compose pull
docker compose up -d

## Upgrade with Monitoring
cd /opt/redoubt
docker compose --profile monitoring pull
docker compose --profile monitoring up -d

## Checking the Current Version
docker compose exec redoubt-api /redoubt-api --version

## Breaking Changes
(version-specific migration notes, if any)

## Rolling Back
docker compose down
# Edit docker-compose.yml to pin previous version
docker compose up -d
```

### 11.5 TROUBLESHOOTING.md Structure

```
# Troubleshooting

## Installation Issues

### DNS not resolving
- Symptoms: Installer says "Could not resolve domain"
- Cause: DNS A record not yet configured or not propagated
- Fix: Add A record, wait 5-15 minutes, retry

### Port already in use
- Symptoms: Installer says "Port 80 in use"
- Cause: Another web server (Apache, nginx) is running
- Fix: Stop the other server or remove it

### Docker installation fails
- Symptoms: get.docker.com script errors
- Cause: Unsupported OS or existing broken Docker install
- Fix: Remove old Docker packages, retry

### TLS certificate not provisioning
- Symptoms: Site shows certificate error after install
- Cause: DNS not pointing to server, or port 80 blocked
- Fix: Verify DNS, check ufw status, check Caddy logs

## Runtime Issues

### Cannot connect to voice channels
- Symptoms: Voice connection fails or drops
- Cause: UDP ports blocked by hosting provider or firewall
- Fix: Ensure 60000-60100/udp is open, check ufw status
- Workaround: LiveKit falls back to TCP via port 7881

### WebSocket disconnects frequently
- Symptoms: Chat messages stop appearing, presence shows offline
- Cause: Proxy timeout or keepalive misconfiguration
- Fix: Check Caddy logs, ensure WebSocket upgrade is working

### Out of memory
- Symptoms: Containers restarting, OOM killed in dmesg
- Cause: Server has insufficient RAM for workload
- Fix: Upgrade VPS to 4+ GB RAM, or reduce LiveKit max participants

### Database connection errors
- Symptoms: API returns 500, logs show "connection refused"
- Cause: PostgreSQL crashed or out of disk space
- Fix: Check docker compose logs postgres, check disk space

### LiveKit not starting
- Symptoms: Voice channels unavailable, LiveKit container restarting
- Cause: Redis not ready, or misconfigured LIVEKIT_KEYS
- Fix: Check that LIVEKIT_API_KEY and LIVEKIT_API_SECRET match in .env

## Viewing Logs

docker compose logs -f                  # All services
docker compose logs -f redoubt-api      # API only
docker compose logs -f livekit          # LiveKit only
docker compose logs -f caddy            # Caddy only
docker compose logs -f postgres         # Database only

## Resetting the Instance

WARNING: This destroys all data.
cd /opt/redoubt
docker compose down -v
rm .env
# Re-run installer
```

---

## 12. Configuration

### 12.1 New Config Options

```yaml
# config/config.yaml additions

admin:
  enabled: true
  port: 9091
  session_secret: "${ADMIN_SESSION_SECRET}"
  session_expiry: 24h
```

### 12.2 New Environment Variables

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `ADMIN_SESSION_SECRET` | Secret for signing admin session cookies | Yes | Generated by installer |
| `GRAFANA_PASSWORD` | Grafana admin password (monitoring profile only) | No | `admin` |

---

## 13. Testing Strategy

### 13.1 Admin Panel Tests

| Test Area | Type | Description |
|-----------|------|-------------|
| Session management | Unit | Create, validate, expire, delete sessions |
| Auth middleware | Unit | Require admin, reject non-admin, reject expired sessions |
| CSRF validation | Unit | Accept valid tokens, reject invalid/missing tokens |
| Template rendering | Unit | Templates render without errors with various data states |
| Dashboard queries | Integration | Stats queries return correct counts |
| User management | Integration | Disable/enable user, revoke sessions, trigger password reset |
| Space management | Integration | List spaces with counts, view detail, delete space |
| Audit log listing | Integration | Pagination, entry detail retrieval |
| Login flow | Integration | Correct credentials + admin → session cookie, non-admin rejected |

### 13.2 Installer Tests

| Test Area | Type | Description |
|-----------|------|-------------|
| OS detection | Unit (bash) | Correctly identifies Ubuntu/Debian, rejects others |
| Secret generation | Unit (bash) | Generated secrets have correct length and format |
| DNS verification | Unit (bash) | Handles match, mismatch, and DNS failure cases |
| Port checking | Unit (bash) | Detects occupied ports correctly |
| Idempotency | Integration | Re-run preserves `.env`, updates other files |
| Full install | Integration (VM) | End-to-end install on a fresh Ubuntu 22.04 VM |

### 13.3 CI Pipeline Tests

| Test Area | Type | Description |
|-----------|------|-------------|
| Docker build | CI | Multi-platform build succeeds (amd64/arm64) |
| Image push | CI | Image tags are correct on GHCR |
| Release creation | CI | GitHub Release is created with notes |

---

## 14. Implementation Tasks

### Milestone 1: Admin Panel Foundation

- [ ] Add `disabled_at` column to users table (migration 0005)
- [ ] Create `admin_sessions` table (migration 0005)
- [ ] Create `internal/admin/` package structure
- [ ] Embed Pico CSS and HTMX via `//go:embed`
- [ ] Implement template loading with base layout pattern
- [ ] Implement admin session management (create, validate, delete, cleanup)
- [ ] Implement CSRF token generation and validation
- [ ] Implement admin login page and authentication handler
- [ ] Implement admin auth middleware (check session cookie + is_instance_admin)
- [ ] Start admin HTTP server on port 9091 alongside main API server

### Milestone 2: Admin Panel Pages

- [ ] Implement dashboard page with application stats
- [ ] Implement HTMX partial for live-updating dashboard stats (30s poll)
- [ ] Implement user list page with pagination
- [ ] Implement user detail page with sessions and memberships
- [ ] Implement user disable/enable actions
- [ ] Implement user password reset trigger
- [ ] Implement user session revocation
- [ ] Implement space list page with member/channel counts
- [ ] Implement space detail page with members, channels, invites
- [ ] Implement space deletion from admin panel
- [ ] Implement invite revocation from admin panel
- [ ] Implement audit log list page with pagination
- [ ] Implement audit log entry detail view (expanded metadata)
- [ ] Write sqlc queries for all admin panel data access

### Milestone 3: Installer Script

- [ ] Write `install.sh` with colored output helpers
- [ ] Implement OS and architecture detection
- [ ] Implement CPU and RAM checks (warn on below minimum)
- [ ] Implement port availability checking
- [ ] Implement Docker installation via get.docker.com
- [ ] Implement domain prompt and DNS verification
- [ ] Implement secret generation functions
- [ ] Implement file download from raw.githubusercontent.com
- [ ] Implement `.env` file generation (with idempotency check)
- [ ] Implement UFW firewall configuration
- [ ] Implement `docker compose pull && up -d` with progress output
- [ ] Implement health check polling with timeout
- [ ] Implement bootstrap invite code extraction from logs
- [ ] Implement success banner output
- [ ] Implement upgrade flow (detect existing install, preserve .env)
- [ ] Implement error handling and cleanup on failure
- [ ] Create production `docker-compose.yml` (uses `image:` not `build:`)

### Milestone 4: CI/CD Release Pipeline

- [ ] Create `.github/workflows/release.yml`
- [ ] Configure Docker Buildx for multi-platform builds (amd64/arm64)
- [ ] Configure GHCR login and push
- [ ] Add build-time version/commit injection via `-ldflags`
- [ ] Configure automatic GitHub Release creation on tag push
- [ ] Update Dockerfile to accept VERSION and COMMIT build args

### Milestone 5: Production Monitoring Overlay

- [ ] Add Prometheus and Grafana services to `docker-compose.yml` with `profiles: ["monitoring"]`
- [ ] Create `config/prometheus.yml` with scrape configs
- [ ] Create Grafana provisioning files (datasource + dashboard)
- [ ] Add `prometheus_data` and `grafana_data` volumes
- [ ] Enable Caddy admin API for Prometheus scraping (metrics endpoint)

### Milestone 6: Documentation

- [ ] Write `docs/INSTALL.md` — full installation guide with quick install and manual steps
- [ ] Write `docs/CONFIGURATION.md` — all env vars and config.yaml options
- [ ] Write `docs/UPGRADING.md` — standard upgrade, rollback, breaking changes
- [ ] Write `docs/TROUBLESHOOTING.md` — DNS, ports, TLS, voice, memory, logs, reset
- [ ] Update README.md with correct install URL and link to docs files

### Milestone 7: Testing

- [ ] Write admin session management unit tests
- [ ] Write admin auth middleware unit tests
- [ ] Write CSRF validation unit tests
- [ ] Write admin handler integration tests (login, dashboard, user management)
- [ ] Test installer OS detection and secret generation (bash unit tests)
- [ ] Test full installer on Ubuntu 22.04 VM (manual or CI)
- [ ] Test release pipeline with a test tag
- [ ] Test monitoring profile starts and scrapes correctly

---

## 15. Acceptance Criteria

### Admin Panel

- [ ] Admin panel starts on port 9091 alongside the main API
- [ ] Admin panel is not accessible through Caddy (separate port only)
- [ ] Login page authenticates with username/email + password
- [ ] Only instance admin users can log in
- [ ] Non-admin users see "Access denied" on login attempt
- [ ] Session cookie is httpOnly and SameSite=Strict
- [ ] Sessions expire after 24 hours
- [ ] CSRF tokens are validated on all POST requests
- [ ] Dashboard shows correct user count, space count, channel count
- [ ] Dashboard shows active WebSocket connections and voice participants
- [ ] Dashboard stats auto-refresh every 30 seconds via HTMX
- [ ] User list shows username, email, status, role, created date
- [ ] User list paginates at 25 per page
- [ ] User detail page shows sessions, memberships
- [ ] Admin can disable a user account
- [ ] Disabling a user revokes all their active sessions
- [ ] Admin can re-enable a disabled user account
- [ ] Admin can trigger a password reset email for a user
- [ ] Admin can revoke all sessions for a user
- [ ] Space list shows name, owner, member count, channel count
- [ ] Space detail shows members, channels, active invites
- [ ] Admin can delete a space from the admin panel
- [ ] Admin can revoke invites from the space detail page
- [ ] Audit log shows entries in reverse chronological order
- [ ] Audit log paginates at 50 per page
- [ ] Clicking an audit entry shows full metadata detail

### Installer Script

- [ ] Script runs successfully on a fresh Ubuntu 22.04 server
- [ ] Script runs successfully on a fresh Debian 12 server
- [ ] Script rejects unsupported operating systems
- [ ] Script rejects unsupported architectures
- [ ] Script warns on insufficient CPU/RAM but continues
- [ ] Script installs Docker if not present
- [ ] Script skips Docker install if Docker >= 24.0 is present
- [ ] Script prompts for domain name
- [ ] Script verifies DNS resolves to the server's public IP
- [ ] Script blocks on DNS mismatch with clear error message
- [ ] Script prompts for email (Let's Encrypt)
- [ ] Script generates cryptographically secure secrets
- [ ] Script creates `/opt/redoubt` directory
- [ ] Script downloads docker-compose.yml, Caddyfile, configs
- [ ] Script writes `.env` with all required variables
- [ ] Script configures UFW with correct port rules
- [ ] Script pulls and starts Docker containers
- [ ] Script waits for health check to pass
- [ ] Script prints bootstrap invite code
- [ ] Script prints admin panel SSH tunnel instructions
- [ ] Script is safe to re-run (idempotent)
- [ ] Re-running preserves existing `.env` file
- [ ] Re-running pulls latest images and restarts

### CI/CD Pipeline

- [ ] Pushing a `v*.*.*` tag triggers the release workflow
- [ ] Docker image is built for both amd64 and arm64
- [ ] Docker image is pushed to `ghcr.io/redoubtapp/redoubt-api:<version>`
- [ ] Docker image is tagged as `latest`
- [ ] Image includes correct version and commit metadata
- [ ] GitHub Release is created automatically with release notes

### Monitoring

- [ ] `docker compose --profile monitoring up -d` starts Prometheus and Grafana
- [ ] Prometheus scrapes metrics from redoubt-api, LiveKit, and Caddy
- [ ] Grafana starts with pre-configured Prometheus data source
- [ ] Default `docker compose up -d` does **not** start monitoring services

### Documentation

- [ ] `docs/INSTALL.md` covers quick install and manual install steps
- [ ] `docs/CONFIGURATION.md` documents all env vars and config options
- [ ] `docs/UPGRADING.md` documents the upgrade process
- [ ] `docs/TROUBLESHOOTING.md` covers DNS, ports, TLS, voice, memory, and log viewing
- [ ] README.md links to docs files and has correct install URL

---

## Summary

Phase 5 prepares Redoubt for public release with:

- **Web-based admin panel** using Go + HTMX + Pico CSS on a separate port, accessible via SSH tunnel
- **One-command installer** for Ubuntu/Debian that handles Docker, DNS verification, secret generation, firewall configuration, and full bootstrap
- **Automated release pipeline** that builds multi-arch Docker images and pushes to GHCR on semver tags
- **Optional monitoring stack** with Prometheus + Grafana via Docker Compose profiles
- **Comprehensive documentation** covering installation, configuration, upgrading, and troubleshooting

The focus is on making Redoubt trivially easy to deploy for non-technical users while maintaining security best practices (auto-generated secrets, firewall configuration, DNS verification, SSH-only admin access).
