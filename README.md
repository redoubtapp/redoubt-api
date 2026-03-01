# Redoubt

> Private, self-hosted voice, video, and chat — built for people who want the \*\*\*\*\*\* experience without handing their conversations to anyone else.

Redoubt uses a familiar **Spaces → Channels** model. You spin up a server, share an invite link, and your friends join. Voice and video run through your own [LiveKit](https://livekit.io) SFU. Text is delivered over WebSockets. Everything lives on infrastructure you control.

---

## Why Redoubt?

- **No third-party servers.** Voice and video traffic flows through your own LiveKit instance, not a vendor's cloud.
- **End-to-end encrypted DMs.** Direct messages use the Signal double-ratchet protocol. The server sees only ciphertext.
- **Dead-simple setup.** One `curl | bash`, one domain, one DNS record. Under 10 minutes for a non-technical host.
- **Honest cost.** You pay your VPS bill. For ~20 concurrent users, that's roughly $15–35/month — split among friends, it's nearly nothing.
- **No telemetry.** Zero license callbacks, zero analytics, fully air-gap compatible.

---

## Stack

| Layer | Technology |
|---|---|
| Management API | Go (`net/http`, `sqlc`, PostgreSQL) |
| Voice & Video | LiveKit (WebRTC SFU) |
| Text Chat | Go WebSocket server (`gorilla/websocket`) |
| Desktop Client | Tauri (Windows / macOS / Linux) |
| Mobile Client | React Native (iOS / Android) |
| Reverse Proxy + TLS | Caddy (automatic Let's Encrypt) |
| Infrastructure | Docker Compose (single VPS) |

---

## Quick Start (Self-Hosting)

**Prerequisites:** A VPS (Ubuntu 22.04+ / Debian 12+) with a public IP and a domain name pointing at it.

```bash
# 1. Run the installer
curl -fsSL https://raw.githubusercontent.com/redoubtapp/redoubt-api/main/install.sh | bash

# 2. Follow the prompts — domain name, email for TLS, done.
#    Caddy provisions TLS automatically.

# 3. Share the bootstrap invite code with friends.
#    They install the app, register with the code, and they're in.
```

Upgrades are a single command:

```bash
cd /opt/redoubt && docker compose pull && docker compose up -d
```

See [docs/INSTALL.md](docs/INSTALL.md) for the full installation guide.

---

## Cost Estimate

Approximate monthly cost for ~20 concurrent users on a single VPS:

| Service | Cost |
|---|---|
| VPS (4 vCPU / 8 GB RAM) — e.g. Hetzner CX31 | $12 – $20 |
| S3-compatible backup storage (Backblaze B2) | $1 – $3 |
| Domain name | ~$1 |
| Bandwidth overage (rare — only if UDP blocked) | $0 – $10 |
| **Total** | **~$15 – $34 / month** |

Split among a group of friends, this is pennies per person.

---

## Project Status

Redoubt is in active early development. See [SPEC.md](./SPEC.md) for the full technical design and build phases.

**Documentation:**
- [Installation Guide](docs/INSTALL.md)
- [Configuration Reference](docs/CONFIGURATION.md)
- [Upgrading](docs/UPGRADING.md)
- [Troubleshooting](docs/TROUBLESHOOTING.md)

---

## Development Setup

### Prerequisites

- Go 1.22+
- Docker and Docker Compose
- [just](https://github.com/casey/just) (command runner)
- [golangci-lint](https://golangci-lint.run/) (optional, for linting)
- [sqlc](https://sqlc.dev/) (optional, for regenerating database code)

### Quick Start

```bash
# Clone the repository
git clone https://github.com/redoubtapp/redoubt-api.git
cd redoubt

# Start all development services (PostgreSQL, Redis, LocalStack, Mailpit, Grafana)
just dev

# The API will be available at http://localhost:8080
# Mailpit UI at http://localhost:8025 (for testing emails)
# Grafana at http://localhost:3000 (for observability)
```

### Project Structure

```
redoubt/
├── cmd/redoubt-api/     # Application entrypoint
├── internal/
│   ├── admin/           # Admin panel (HTMX + Pico CSS, port 9091)
│   ├── api/             # HTTP handlers and middleware
│   ├── audit/           # Audit logging
│   ├── auth/            # Authentication (JWT, passwords, sessions)
│   ├── cache/           # Redis client wrapper
│   ├── channels/        # Channel business logic
│   ├── config/          # Configuration loading
│   ├── db/              # Database migrations and sqlc queries
│   ├── email/           # Email client (Resend)
│   ├── errors/          # Error definitions and RFC 9457 responses
│   ├── invites/         # Invite code management
│   ├── livekit/         # LiveKit SFU integration
│   ├── messages/        # Message and reaction services
│   ├── opengraph/       # OpenGraph link previews
│   ├── presence/        # WebSocket presence hub
│   ├── ratelimit/       # Redis-backed rate limiting
│   ├── spaces/          # Space business logic
│   ├── storage/         # S3 storage with encryption
│   ├── telemetry/       # OpenTelemetry tracing and metrics
│   └── voice/           # Voice channel management
├── config/              # Configuration files
├── docker/              # Docker-related files
├── docs/                # Documentation and API spec
├── install.sh           # Production installer script
└── justfile             # Development task runner
```

### Common Commands

```bash
# Development
just dev                   # Start development stack with hot reload
just down                  # Stop all services
just logs                  # View logs (default: redoubt-api)
just logs postgres         # View specific service logs

# Database
just migrate-up            # Run migrations
just migrate-down          # Rollback last migration
just migrate-create name   # Create new migration
just sqlc                  # Regenerate sqlc code

# Testing
just test                  # Run unit tests
just test-integration      # Run integration tests
just lint                  # Run golangci-lint

# Building
just build                 # Build binary
just swagger               # Generate OpenAPI spec
```

### Configuration

Configuration is loaded from YAML files with environment variable overrides.

**Development**: `config/config.dev.yaml`
**Production**: `config/config.yaml`

Key environment variables for production:

| Variable | Description |
|----------|-------------|
| `POSTGRES_PASSWORD` | PostgreSQL password |
| `JWT_SECRET` | Secret for signing JWTs (min 32 bytes) |
| `RESEND_API_KEY` | Resend API key for transactional emails |
| `S3_ACCESS_KEY` | S3-compatible storage access key |
| `S3_SECRET_KEY` | S3-compatible storage secret key |
| `STORAGE_MASTER_KEY` | Master encryption key (32 bytes, base64) |
| `LIVEKIT_API_KEY` | LiveKit API key |
| `LIVEKIT_API_SECRET` | LiveKit API secret |
| `ADMIN_SESSION_SECRET` | Admin panel session signing secret |

See [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for the full configuration reference.

### API Documentation

The API is documented using OpenAPI 3.1. See [docs/openapi.yaml](./docs/openapi.yaml).

Key endpoints:

| Endpoint | Description |
|----------|-------------|
| `POST /api/v1/auth/register` | Register with invite code |
| `POST /api/v1/auth/login` | Login and receive tokens |
| `GET /api/v1/users/me` | Get current user profile |
| `GET /api/v1/spaces` | List user's spaces |
| `GET /api/v1/health` | Health check with components |

All error responses use RFC 9457 Problem Details format.

### Observability

The development stack includes Grafana LGTM (Loki, Grafana, Tempo, Mimir) for full observability:

- **Grafana UI**: http://localhost:3000 (no login required in dev)
- **Traces**: Available in Grafana → Explore → Tempo
- **Logs**: Available in Grafana → Explore → Loki
- **Metrics**: Available in Grafana → Explore → Mimir

The API exposes Prometheus metrics at `/metrics` (port 9090).

### Testing

```bash
# Run all unit tests
go test -v -race ./...

# Run with coverage
go test -v -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run integration tests (requires Docker)
go test -v -race -tags=integration ./...
```

### First User Setup

When the API starts for the first time:

1. A bootstrap invite code is generated and logged
2. Use this code to register the first user
3. The first user automatically becomes the instance admin
4. The instance admin can create spaces and generate new invite codes

---

## License

[AGPL-3.0](./LICENSE). Self-host for free. A commercial license is available for operators who embed Redoubt in a product and cannot open-source their modifications.