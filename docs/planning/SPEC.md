# Redoubt — Technical Specification

**Status:** Early design / pre-implementation  
**Last updated:** 2026-02-22  
**Author:** Michael

This document is the authoritative reference for how Redoubt is designed, why certain decisions were made, and what the build sequence looks like. It is written for the author's own reference — terseness over thoroughness.

---

## Table of Contents

- [Redoubt — Technical Specification](#redoubt--technical-specification)
  - [Table of Contents](#table-of-contents)
  - [1. Product Model](#1-product-model)
    - [Hierarchy](#hierarchy)
  - [2. Architecture Overview](#2-architecture-overview)
  - [3. Component Breakdown](#3-component-breakdown)
    - [3.1 Go Management API](#31-go-management-api)
    - [3.2 LiveKit SFU](#32-livekit-sfu)
    - [3.3 Text Chat (WebSocket)](#33-text-chat-websocket)
    - [3.4 Clients](#34-clients)
    - [3.5 Reverse Proxy (Caddy)](#35-reverse-proxy-caddy)
  - [4. Data Model](#4-data-model)
  - [5. Authentication \& Authorization](#5-authentication--authorization)
  - [6. Voice \& Video (LiveKit)](#6-voice--video-livekit)
  - [7. Text Chat \& Messaging](#7-text-chat--messaging)
  - [8. End-to-End Encrypted DMs](#8-end-to-end-encrypted-dms)
  - [9. Infrastructure \& Deployment](#9-infrastructure--deployment)
    - [Docker Compose Stack](#docker-compose-stack)
    - [Installer Script](#installer-script)
    - [Backup](#backup)
    - [Observability](#observability)
  - [10. Build Phases](#10-build-phases)
    - [Phase 1 — Foundation](#phase-1--foundation)
    - [Phase 2 — Voice \& Video](#phase-2--voice--video)
    - [Phase 3 — Text Chat](#phase-3--text-chat)
    - [Phase 4 — Client Apps](#phase-4--client-apps)
    - [Phase 5 — Polish \& Ship](#phase-5--polish--ship)
  - [11. Open Questions](#11-open-questions)

---

## 1. Product Model

Redoubt is organized around two primary concepts:

**Space** — the top-level container. A user creates or is invited to a Space. A Space has a name, icon, invite links, and a set of Channels.

**Channel** — a communication context within a Space. Two types:
- *Text channel* — persistent message history, threads, reactions.
- *Voice/video channel* — real-time audio/video room powered by LiveKit. No persistent media storage by default.

**Direct Message (DM)** — a 1:1 or small-group conversation outside of a Space. E2EE via Signal protocol.

### Hierarchy

```
User
└── Membership (role) → Space
    └── Channel (text | voice)
        ├── Message / Thread / Reaction  [text]
        └── VoiceSession / Participant   [voice]
```

---

## 2. Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                        Clients                          │
│          Tauri (desktop)   React Native (mobile)        │
└────────────┬───────────────────────┬────────────────────┘
             │ HTTPS / WSS           │ WebRTC (DTLS/SRTP)
┌────────────▼──────────────┐  ┌────▼──────────────────────┐
│     Go Management API     │  │     LiveKit SFU            │
│  REST + WebSocket server  │  │  (voice/video/screen share)│
│  net/http · sqlc · pq     │  │  bundled TURN/STUN         │
└────────────┬──────────────┘  └───────────────────────────┘
             │
     ┌───────▼────────┐
     │   PostgreSQL   │
     └───────────────-┘

Reverse proxy: Caddy (TLS termination, routes /api → Go, /livekit → LiveKit)
```

All components run in Docker Compose on a single VPS. Caddy handles HTTPS and terminates TLS — Go and LiveKit listen on internal ports only.

---

## 3. Component Breakdown

### 3.1 Go Management API

Responsibilities:
- User registration, login, session management (JWT + refresh tokens)
- CRUD for Spaces, Channels, Memberships, Invites
- Issuing LiveKit access tokens (short-lived, scoped to a room)
- WebSocket hub for text chat delivery and presence (online/offline/voice-active)
- Message persistence and retrieval (paginated)
- Admin endpoints (user bans, channel management)
- Webhook target for LiveKit room events

Package layout (rough):

```
cmd/
  redoubt-api/        ← main entry point
internal/
  auth/             ← JWT, session, password hashing (Argon2id)
  space/            ← space + channel business logic
  chat/             ← WebSocket hub, message store
  livekit/          ← token generation, room event handling
  db/               ← sqlc-generated queries + migrations (golang-migrate)
  config/           ← env-based config (no config files in repo)
```

### 3.2 LiveKit SFU

LiveKit runs as a separate Docker service. The Go API:
1. Creates rooms via the LiveKit server API when a user joins a voice channel.
2. Issues a signed JWT (using the LiveKit Go SDK) scoped to that room and user.
3. The client uses that token to connect directly to LiveKit over WebRTC.

The Go API never touches the media stream — it only manages tokens and room metadata.

LiveKit's built-in TURN/STUN server handles NAT traversal. No separate Coturn needed for basic setups.

### 3.3 Text Chat (WebSocket)

Each connected client maintains a WebSocket connection to the Go API. The hub pattern:

- Each Space has a set of subscribed connections.
- On message send, the API persists to Postgres, then fans out to all connections subscribed to that channel.
- Presence (typing indicators, online status) is delivered on the same connection.

No external message broker (no Redis, no NATS) in v1. If the deployment grows beyond a single instance this will need revisiting — noted in Open Questions.

### 3.4 Clients

**Desktop — Tauri:** Rust shell wrapping a web frontend (React + TypeScript). Tauri gives us native OS integration (system tray, notifications, file access) with a minimal binary. LiveKit's JavaScript SDK handles WebRTC in the webview.

**Mobile — React Native:** Shared business logic with the desktop web frontend where possible. LiveKit has an official React Native SDK. Push notifications via FCM/APNs — the server stores device tokens and calls the push gateway on new messages.

### 3.5 Reverse Proxy (Caddy)

```
Caddyfile (simplified):

redoubt.yourdomain.com {
    handle /api/* {
        reverse_proxy localhost:8080
    }
    handle /livekit/* {
        reverse_proxy localhost:7880
    }
    handle {
        reverse_proxy localhost:3000  # static client assets or SPA
    }
}
```

Caddy obtains and renews Let's Encrypt certificates automatically. No manual cert management.

---

## 4. Data Model

Simplified schema — exact types and indexes to be determined during implementation.

```sql
users
  id          uuid PK
  username    text UNIQUE NOT NULL
  email       text UNIQUE NOT NULL
  password    text NOT NULL          -- Argon2id hash
  avatar_url  text
  created_at  timestamptz

spaces
  id          uuid PK
  name        text NOT NULL
  icon_url    text
  owner_id    uuid → users.id
  created_at  timestamptz

memberships
  user_id     uuid → users.id
  space_id    uuid → spaces.id
  role        text                   -- owner | admin | moderator | member | guest
  joined_at   timestamptz
  PRIMARY KEY (user_id, space_id)

channels
  id          uuid PK
  space_id    uuid → spaces.id
  name        text NOT NULL
  type        text                   -- text | voice
  position    int                    -- display order
  created_at  timestamptz

messages
  id          uuid PK
  channel_id  uuid → channels.id
  author_id   uuid → users.id
  content     text                   -- plaintext for channel msgs; ciphertext for DMs
  thread_id   uuid → messages.id    -- nullable; identifies parent of a thread reply
  created_at  timestamptz
  edited_at   timestamptz

reactions
  message_id  uuid → messages.id
  user_id     uuid → users.id
  emoji       text
  PRIMARY KEY (message_id, user_id, emoji)

invites
  code        text PK
  space_id    uuid → spaces.id
  created_by  uuid → users.id
  uses        int DEFAULT 0
  max_uses    int                    -- nullable = unlimited
  expires_at  timestamptz           -- nullable = never
```

---

## 5. Authentication & Authorization

**Auth mechanism:** JWT (access token, 15 min expiry) + refresh token (httpOnly cookie, 30 day expiry). Standard rotation on refresh.

**Password hashing:** Argon2id with tuned memory/time parameters. No bcrypt.

**LiveKit tokens:** Signed with the LiveKit API secret. Scoped to a specific room and user identity. Issued by the Go API only after verifying the user is a member of the Space and has permission to join the channel.

**Role hierarchy (Space-level):**

```
Owner → Admin → Moderator → Member → Guest
```

Roles gate: channel creation/deletion, user kicks/bans, invite creation, and voice channel joins. Permissions are checked in the Go API — never trusted from the client.

---

## 6. Voice & Video (LiveKit)

LiveKit is the core of the real-time A/V system. Key concepts:

- **Room** — maps 1:1 to a Redoubt voice channel. Created on first join, destroyed when empty (configurable).
- **Participant** — a user in a room. Has tracks: microphone, camera, screen share.
- **Token** — issued by Go API, contains the user's identity and room permissions (can publish, can subscribe).

**Flow:**

1. Client requests a voice channel join → `POST /api/channels/{id}/join`
2. Go API verifies membership + permissions, calls LiveKit server API to ensure room exists, generates a short-lived participant token.
3. Client receives token, connects to LiveKit directly via WebRTC.
4. LiveKit sends room events (participant joined/left) to the Go API via webhook → Go API fans out presence updates to WebSocket clients.

**Features available via LiveKit SDK:** noise suppression, echo cancellation, adaptive bitrate, simulcast, screen share, dynacast (auto-disable video for inactive participants).

---

## 7. Text Chat & Messaging

**Message delivery:** WebSocket, fan-out within the Go API hub. No ordering guarantees beyond server insertion order (Postgres `created_at` + sequential scan on channel).

**Message features (v1):**
- Send, edit, delete
- Threaded replies (flat — one level deep)
- Reactions (emoji only)
- Basic markdown rendering on client

**Message features (future):**
- File/image attachments (stored in S3-compatible bucket)
- Link embeds / unfurls
- Search (pg_trgm or external index)

**Pagination:** cursor-based on `created_at` + `id`. Client fetches the last N messages on channel open, then receives subsequent messages via WebSocket.

---

## 8. End-to-End Encrypted DMs

DMs between two users use the Signal double-ratchet protocol, implemented via `libsignal-protocol-go` (or a maintained Go port — evaluate at implementation time).

**Key concepts:**
- Each user generates an identity key pair on first launch, stored locally (never uploaded as private key).
- Extended triple Diffie-Hellman (X3DH) for initial key agreement.
- Double-ratchet for forward secrecy on subsequent messages.
- The server stores and delivers encrypted message payloads only — it cannot decrypt them.

**Server role:** key distribution (public keys only), message relay, delivery receipts. No decryption capability.

**Tradeoff:** Message history is device-local. If you lose your device and keys, DM history is gone. This is intentional — it's the privacy guarantee. Group DMs will be Sender Keys (Signal's scalable group E2EE).

---

## 9. Infrastructure & Deployment

### Docker Compose Stack

Services:
- `redoubt-api` — Go binary
- `livekit` — LiveKit SFU
- `postgres` — database
- `caddy` — reverse proxy + TLS

All on a single Docker network. Only Caddy exposes ports 80 and 443 to the host.

### Installer Script

The `install.sh` script:
1. Checks for Docker + Compose.
2. Downloads `docker-compose.yml` and `.env.template`.
3. Prompts for: domain name, admin username/password.
4. Generates secrets (LiveKit API key/secret, JWT signing key) with `openssl rand`.
5. Writes `.env`.
6. Runs `docker compose up -d`.
7. Waits for health checks, prints the invite URL.

The script is idempotent — safe to re-run for upgrades.

### Backup

A `redoubt-backup` sidecar container runs `pg_dump` on a cron schedule and uploads to an S3-compatible bucket (Backblaze B2 recommended for cost). Retention policy configurable via env.

### Observability

- `redoubt-api` exposes `/metrics` in Prometheus format.
- Optional `docker-compose.monitoring.yml` overlay adds Prometheus + Grafana.
- LiveKit exposes its own Prometheus metrics endpoint.

---

## 10. Build Phases

### Phase 1 — Foundation
- Go project scaffold (`cmd/`, `internal/`, `db/`)
- PostgreSQL schema + migrations (golang-migrate)
- Auth: registration, login, JWT + refresh
- Space and Channel CRUD
- Docker Compose stack + Caddy config
- Basic health check endpoint

### Phase 2 — Voice & Video
- LiveKit integration: token issuance, room management
- Voice channel join/leave flow (API + client)
- LiveKit webhook handler → WebSocket presence fan-out
- Basic Tauri desktop client (voice only)

### Phase 3 — Text Chat
- WebSocket hub
- Message send/receive/persist
- Edit, delete, reactions
- Threaded replies
- Message history pagination

### Phase 4 — Client Apps
- React Native mobile client (iOS + Android)
- Full Tauri desktop client (text + voice)
- Push notifications (FCM/APNs)
- E2EE DMs (Signal protocol integration)

### Phase 5 — Polish & Ship
- Web-based admin panel (Go + HTMX — no separate frontend build)
- `install.sh` installer script
- Backup sidecar
- Prometheus + Grafana overlay
- Public GitHub release, docs site

---

## 11. Open Questions

- **Multi-instance scaling:** The WebSocket hub is in-process. If the API ever needs to run as multiple replicas, the hub needs an external pub/sub layer (Redis Streams, NATS). Not a concern for v1 single-VPS deployments.
- **Signal protocol library:** `libsignal-protocol-go` maintenance status needs evaluation. May need to wrap the Rust `libsignal` via CGo or maintain a pure-Go implementation.
- **Message search:** `pg_trgm` covers basic cases. Full-text search at scale would want a dedicated index (Meilisearch, Typesense). Defer until there's a reason.
- **File attachments:** Needs an S3-compatible upload flow (presigned URLs), virus scanning consideration, and CDN for delivery. Defer to post-v1.
- **Push notifications:** Requires APNs certificate (Apple) and FCM project (Google). Both are free but involve account setup. Need to decide if this is bundled in the installer or left to the operator.
- **Mobile E2EE key backup:** Lost device = lost DM history is a strong guarantee but a rough UX. Consider optional encrypted key backup to the server (password-derived key) as an opt-in.