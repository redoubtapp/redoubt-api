# Redoubt — Phase 3 Implementation Document

**Status:** Ready for implementation
**Last updated:** 2026-02-23
**Author:** Michael

This document defines the complete scope, technical decisions, and implementation details for Phase 3 (Text Chat) of Redoubt.

---

## Table of Contents

- [1. Phase 3 Scope Summary](#1-phase-3-scope-summary)
- [2. Architecture Decisions](#2-architecture-decisions)
- [3. Database Schema](#3-database-schema)
- [4. Message Content & Formatting](#4-message-content--formatting)
- [5. API Design](#5-api-design)
- [6. WebSocket Event Protocol](#6-websocket-event-protocol)
- [7. Threading Model](#7-threading-model)
- [8. Reactions System](#8-reactions-system)
- [9. Read State Tracking](#9-read-state-tracking)
- [10. Rate Limiting](#10-rate-limiting)
- [11. Client Components](#11-client-components)
- [12. Client State Management](#12-client-state-management)
- [13. Optimistic Updates](#13-optimistic-updates)
- [14. Configuration](#14-configuration)
- [15. Testing Strategy](#15-testing-strategy)
- [16. Implementation Tasks](#16-implementation-tasks)
- [17. Acceptance Criteria](#17-acceptance-criteria)

---

## 1. Phase 3 Scope Summary

Phase 3 adds text messaging to Redoubt with the following deliverables:

| Component | Scope |
|-----------|-------|
| Message CRUD | Send, edit (with history), delete (soft), retrieve with pagination |
| Threading | Flat threads (one level deep), inline expanded display |
| Reactions | Curated emoji set (~100), native system rendering |
| Markdown | Basic formatting (bold, italic, code, code blocks, links) |
| Read State | Channel-level unread tracking with last-read timestamp |
| Real-time | WebSocket message delivery, typing indicators (from Phase 2) |
| Client UI | MessageList, MessageInput, ThreadView, ReactionPicker, EditHistory |
| Optimistic Updates | Instant UI feedback with server confirmation rollback |

**Deferred to Phase 4:**
- @mentions and notifications
- File/image attachments
- Link previews/embeds
- Message search
- Push notifications

---

## 2. Architecture Decisions

### Core Libraries & Frameworks

| Concern | Choice | Rationale |
|---------|--------|-----------|
| Markdown Parser (Server) | `goldmark` | Fast, CommonMark compliant, extensible |
| Markdown Parser (Client) | `react-markdown` | React-native rendering, good security defaults |
| Emoji Data | Custom curated JSON | Minimal bundle, native rendering |
| Cursor Pagination | `created_at` + `id` composite | Stable pagination, handles concurrent inserts |

### Key Design Decisions

| Decision | Choice |
|----------|--------|
| Message storage | Plaintext in DB, render markdown on client |
| Edit history | Stored in separate table, viewable by author and admins only |
| Delete behavior | Soft delete with "message was deleted" placeholder |
| Thread model | Flat (one level), `thread_id` references parent message |
| Thread display | Inline with preview (first 3 replies visible, "load more" for rest) |
| Reactions | Curated set (~100 emoji), native system rendering |
| Read tracking | Channel-level only (last_read_at per membership) |
| Message length | 2,000 characters maximum |
| Edit window | 15 minutes (configurable) |
| Delete window | No limit (users can always delete own messages) |
| Pagination | 50 messages per page, cursor-based |
| Rate limiting | 5 messages per 5 seconds per user |
| Optimistic updates | Enabled with failure rollback |
| Mentions | Deferred to Phase 4 |
| Emoji shortcodes | `:name:` autocomplete enabled (e.g., `:thumbsup:`) |
| Deleted content | Truly deleted, not viewable by admins |

---

## 3. Database Schema

### Migration: `0003_messages.up.sql`

```sql
-- Messages table
CREATE TABLE messages (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    channel_id      UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    author_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content         TEXT NOT NULL,
    thread_id       UUID REFERENCES messages(id) ON DELETE CASCADE,
    is_thread_root  BOOLEAN NOT NULL DEFAULT FALSE,
    reply_count     INTEGER NOT NULL DEFAULT 0,
    edited_at       TIMESTAMPTZ,
    deleted_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT messages_content_length CHECK (char_length(content) <= 2000),
    CONSTRAINT messages_thread_not_self CHECK (thread_id != id),
    CONSTRAINT messages_thread_root_no_parent CHECK (
        NOT (is_thread_root = TRUE AND thread_id IS NOT NULL)
    )
);

-- Indexes for message queries
CREATE INDEX idx_messages_channel_created ON messages(channel_id, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_messages_channel_cursor ON messages(channel_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_messages_thread ON messages(thread_id, created_at ASC)
    WHERE deleted_at IS NULL AND thread_id IS NOT NULL;
CREATE INDEX idx_messages_author ON messages(author_id);

-- Message edit history
CREATE TABLE message_edits (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_id      UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    previous_content TEXT NOT NULL,
    edited_by       UUID NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_message_edits_message ON message_edits(message_id, created_at DESC);

-- Reactions table
CREATE TABLE reactions (
    message_id      UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji           VARCHAR(32) NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (message_id, user_id, emoji)
);

CREATE INDEX idx_reactions_message ON reactions(message_id);

-- Curated emoji set (reference table)
CREATE TABLE emoji_set (
    emoji           VARCHAR(32) PRIMARY KEY,
    name            VARCHAR(64) NOT NULL,
    category        VARCHAR(32) NOT NULL,
    sort_order      INTEGER NOT NULL DEFAULT 0
);

-- Channel read state (extends memberships)
ALTER TABLE memberships
    ADD COLUMN last_read_at TIMESTAMPTZ,
    ADD COLUMN last_read_message_id UUID REFERENCES messages(id) ON DELETE SET NULL;

-- Message rate limiting tracking
CREATE TABLE message_rate_limits (
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id      UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    message_count   INTEGER NOT NULL DEFAULT 1,
    window_start    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (user_id, channel_id)
);

CREATE INDEX idx_message_rate_limits_window ON message_rate_limits(window_start);
```

### Migration: `0003_messages.down.sql`

```sql
ALTER TABLE memberships
    DROP COLUMN IF EXISTS last_read_at,
    DROP COLUMN IF EXISTS last_read_message_id;

DROP TABLE IF EXISTS message_rate_limits;
DROP TABLE IF EXISTS emoji_set;
DROP TABLE IF EXISTS reactions;
DROP TABLE IF EXISTS message_edits;
DROP TABLE IF EXISTS messages;
```

### Seed Data: Curated Emoji Set

```sql
-- Insert curated emoji set (run after migration)
INSERT INTO emoji_set (emoji, name, category, sort_order) VALUES
-- Smileys & People
('😀', 'grinning', 'smileys', 1),
('😃', 'smiley', 'smileys', 2),
('😄', 'smile', 'smileys', 3),
('😁', 'grin', 'smileys', 4),
('😅', 'sweat_smile', 'smileys', 5),
('😂', 'joy', 'smileys', 6),
('🤣', 'rofl', 'smileys', 7),
('😊', 'blush', 'smileys', 8),
('😇', 'innocent', 'smileys', 9),
('🙂', 'slightly_smiling', 'smileys', 10),
('😉', 'wink', 'smileys', 11),
('😍', 'heart_eyes', 'smileys', 12),
('🥰', 'smiling_hearts', 'smileys', 13),
('😘', 'kissing_heart', 'smileys', 14),
('😋', 'yum', 'smileys', 15),
('😎', 'sunglasses', 'smileys', 16),
('🤔', 'thinking', 'smileys', 17),
('🤨', 'raised_eyebrow', 'smileys', 18),
('😐', 'neutral', 'smileys', 19),
('😑', 'expressionless', 'smileys', 20),
('😶', 'no_mouth', 'smileys', 21),
('🙄', 'roll_eyes', 'smileys', 22),
('😏', 'smirk', 'smileys', 23),
('😣', 'persevere', 'smileys', 24),
('😥', 'disappointed_relieved', 'smileys', 25),
('😮', 'open_mouth', 'smileys', 26),
('🤐', 'zipper_mouth', 'smileys', 27),
('😯', 'hushed', 'smileys', 28),
('😪', 'sleepy', 'smileys', 29),
('😫', 'tired', 'smileys', 30),
('🥱', 'yawning', 'smileys', 31),
('😴', 'sleeping', 'smileys', 32),
('😌', 'relieved', 'smileys', 33),
('😛', 'stuck_out_tongue', 'smileys', 34),
('😜', 'stuck_out_tongue_winking', 'smileys', 35),
('😝', 'stuck_out_tongue_closed_eyes', 'smileys', 36),
('🤤', 'drooling', 'smileys', 37),
('😒', 'unamused', 'smileys', 38),
('😓', 'downcast_sweat', 'smileys', 39),
('😔', 'pensive', 'smileys', 40),
('😕', 'confused', 'smileys', 41),
('🙃', 'upside_down', 'smileys', 42),
('🤑', 'money_mouth', 'smileys', 43),
('😲', 'astonished', 'smileys', 44),
('🙁', 'slightly_frowning', 'smileys', 45),
('😖', 'confounded', 'smileys', 46),
('😞', 'disappointed', 'smileys', 47),
('😟', 'worried', 'smileys', 48),
('😤', 'triumph', 'smileys', 49),
('😢', 'cry', 'smileys', 50),
('😭', 'sob', 'smileys', 51),
('😦', 'frowning', 'smileys', 52),
('😧', 'anguished', 'smileys', 53),
('😨', 'fearful', 'smileys', 54),
('😩', 'weary', 'smileys', 55),
('🤯', 'exploding_head', 'smileys', 56),
('😬', 'grimacing', 'smileys', 57),
('😰', 'cold_sweat', 'smileys', 58),
('😱', 'scream', 'smileys', 59),
('🥵', 'hot', 'smileys', 60),
('🥶', 'cold', 'smileys', 61),
('😳', 'flushed', 'smileys', 62),
('🤪', 'zany', 'smileys', 63),
('😵', 'dizzy', 'smileys', 64),
('🥴', 'woozy', 'smileys', 65),
('😠', 'angry', 'smileys', 66),
('😡', 'rage', 'smileys', 67),
('🤬', 'cursing', 'smileys', 68),
-- Gestures
('👍', 'thumbsup', 'gestures', 100),
('👎', 'thumbsdown', 'gestures', 101),
('👏', 'clap', 'gestures', 102),
('🙌', 'raised_hands', 'gestures', 103),
('🤝', 'handshake', 'gestures', 104),
('🙏', 'pray', 'gestures', 105),
('💪', 'muscle', 'gestures', 106),
('👋', 'wave', 'gestures', 107),
('✋', 'raised_hand', 'gestures', 108),
('🤚', 'raised_back_of_hand', 'gestures', 109),
('👌', 'ok_hand', 'gestures', 110),
('✌️', 'v', 'gestures', 111),
('🤞', 'crossed_fingers', 'gestures', 112),
('🤟', 'love_you', 'gestures', 113),
('🤘', 'metal', 'gestures', 114),
('👈', 'point_left', 'gestures', 115),
('👉', 'point_right', 'gestures', 116),
('👆', 'point_up', 'gestures', 117),
('👇', 'point_down', 'gestures', 118),
('☝️', 'point_up_2', 'gestures', 119),
-- Symbols & Objects
('❤️', 'heart', 'symbols', 200),
('🧡', 'orange_heart', 'symbols', 201),
('💛', 'yellow_heart', 'symbols', 202),
('💚', 'green_heart', 'symbols', 203),
('💙', 'blue_heart', 'symbols', 204),
('💜', 'purple_heart', 'symbols', 205),
('🖤', 'black_heart', 'symbols', 206),
('💔', 'broken_heart', 'symbols', 207),
('💯', '100', 'symbols', 208),
('💢', 'anger', 'symbols', 209),
('💥', 'boom', 'symbols', 210),
('💫', 'dizzy_symbol', 'symbols', 211),
('💬', 'speech_balloon', 'symbols', 212),
('👁️‍🗨️', 'eye_in_speech_bubble', 'symbols', 213),
('🔥', 'fire', 'symbols', 214),
('✨', 'sparkles', 'symbols', 215),
('⭐', 'star', 'symbols', 216),
('🌟', 'star2', 'symbols', 217),
('💡', 'bulb', 'symbols', 218),
('📌', 'pushpin', 'symbols', 219),
('✅', 'white_check_mark', 'symbols', 220),
('❌', 'x', 'symbols', 221),
('❓', 'question', 'symbols', 222),
('❗', 'exclamation', 'symbols', 223),
('⚠️', 'warning', 'symbols', 224),
('🚀', 'rocket', 'symbols', 225),
('🎉', 'tada', 'symbols', 226),
('🎊', 'confetti_ball', 'symbols', 227),
('🏆', 'trophy', 'symbols', 228),
('🔔', 'bell', 'symbols', 229);
```

---

## 4. Message Content & Formatting

### Supported Markdown Syntax

| Syntax | Example | Renders As |
|--------|---------|------------|
| Bold | `**bold**` | **bold** |
| Italic | `*italic*` or `_italic_` | *italic* |
| Bold+Italic | `***both***` | ***both*** |
| Strikethrough | `~~strike~~` | ~~strike~~ |
| Inline code | `` `code` `` | `code` |
| Code block | ` ```lang\ncode\n``` ` | Syntax highlighted block |
| Link | `[text](url)` | Clickable link |
| Auto-link | `https://example.com` | Clickable link |

### Not Supported (Phase 3)

- Headers (`#`, `##`, etc.)
- Lists (bulleted, numbered)
- Blockquotes (`>`)
- Tables
- Images
- Horizontal rules

### Content Validation

```go
// internal/messages/validation.go

package messages

import (
    "errors"
    "strings"
    "unicode/utf8"
)

const (
    MaxMessageLength    = 2000
    MinMessageLength    = 1
    MaxCodeBlockLength  = 1500
)

var (
    ErrMessageTooLong    = errors.New("message exceeds 2000 characters")
    ErrMessageEmpty      = errors.New("message cannot be empty")
    ErrCodeBlockTooLong  = errors.New("code block exceeds 1500 characters")
)

func ValidateContent(content string) error {
    content = strings.TrimSpace(content)

    if len(content) == 0 {
        return ErrMessageEmpty
    }

    if utf8.RuneCountInString(content) > MaxMessageLength {
        return ErrMessageTooLong
    }

    // Check for oversized code blocks
    if hasOversizedCodeBlock(content) {
        return ErrCodeBlockTooLong
    }

    return nil
}

func hasOversizedCodeBlock(content string) bool {
    // Simple check for ``` delimited blocks
    inBlock := false
    blockStart := 0

    for i := 0; i < len(content)-2; i++ {
        if content[i:i+3] == "```" {
            if !inBlock {
                inBlock = true
                blockStart = i + 3
                // Skip to end of opening line
                for blockStart < len(content) && content[blockStart] != '\n' {
                    blockStart++
                }
            } else {
                blockLen := i - blockStart
                if blockLen > MaxCodeBlockLength {
                    return true
                }
                inBlock = false
            }
        }
    }

    return false
}
```

### Client-Side Markdown Rendering

```tsx
// src/components/chat/MessageContent.tsx

import { useState } from 'react';
import ReactMarkdown from 'react-markdown';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism';
import { Copy, Check } from 'lucide-react';

interface MessageContentProps {
  content: string;
}

export function MessageContent({ content }: MessageContentProps) {
  return (
    <ReactMarkdown
      allowedElements={['p', 'strong', 'em', 'del', 'code', 'pre', 'a']}
      components={{
        code({ node, inline, className, children, ...props }) {
          const match = /language-(\w+)/.exec(className || '');
          const language = match ? match[1] : '';
          const codeString = String(children).replace(/\n$/, '');

          return !inline ? (
            <CodeBlock code={codeString} language={language} />
          ) : (
            <code className="bg-zinc-800 px-1 py-0.5 rounded text-sm" {...props}>
              {children}
            </code>
          );
        },
        a({ href, children }) {
          return (
            <a
              href={href}
              target="_blank"
              rel="noopener noreferrer"
              className="text-blue-400 hover:underline"
            >
              {children}
            </a>
          );
        },
      }}
    >
      {content}
    </ReactMarkdown>
  );
}

// Code block with copy button
function CodeBlock({ code, language }: { code: string; language: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    await navigator.clipboard.writeText(code);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="relative group">
      <button
        onClick={handleCopy}
        className="absolute right-2 top-2 p-1.5 rounded bg-zinc-700 opacity-0 group-hover:opacity-100 transition-opacity"
        title="Copy code"
      >
        {copied ? (
          <Check className="h-4 w-4 text-green-400" />
        ) : (
          <Copy className="h-4 w-4 text-zinc-300" />
        )}
      </button>
      <SyntaxHighlighter
        style={oneDark}
        language={language}
        PreTag="div"
      >
        {code}
      </SyntaxHighlighter>
    </div>
  );
}
```

---

## 5. API Design

### New Endpoints

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| POST | `/channels/:id/messages` | Send message | Member |
| GET | `/channels/:id/messages` | List messages (paginated) | Member |
| GET | `/messages/:id` | Get single message | Member |
| PATCH | `/messages/:id` | Edit message | Author (within 15 min) |
| DELETE | `/messages/:id` | Delete message | Author or Admin+ |
| GET | `/messages/:id/edits` | Get edit history | Author or Admin+ |
| POST | `/messages/:id/reactions` | Add reaction | Member |
| DELETE | `/messages/:id/reactions/:emoji` | Remove reaction | Author of reaction |
| GET | `/messages/:id/reactions` | List reactions | Member |
| GET | `/messages/:id/thread` | Get thread replies | Member |
| POST | `/messages/:id/thread` | Reply to thread | Member |
| PUT | `/channels/:id/read` | Mark channel as read | Member |
| GET | `/channels/:id/unread` | Get unread count | Member |
| GET | `/emoji` | Get curated emoji list | Yes |

### Request/Response Examples

#### Send Message

```http
POST /api/v1/channels/770e8400-e29b-41d4-a716-446655440002/messages
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
Content-Type: application/json

{
  "content": "Hello **world**! Check out this code:\n\n```go\nfmt.Println(\"hi\")\n```"
}
```

```json
{
  "id": "880e8400-e29b-41d4-a716-446655440000",
  "channel_id": "770e8400-e29b-41d4-a716-446655440002",
  "author": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "username": "alice",
    "avatar_url": null
  },
  "content": "Hello **world**! Check out this code:\n\n```go\nfmt.Println(\"hi\")\n```",
  "thread_id": null,
  "is_thread_root": false,
  "reply_count": 0,
  "reactions": [],
  "edited_at": null,
  "created_at": "2026-02-23T10:30:00Z"
}
```

#### List Messages (Paginated)

```http
GET /api/v1/channels/770e8400-e29b-41d4-a716-446655440002/messages?limit=50
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

```json
{
  "data": [
    {
      "id": "880e8400-e29b-41d4-a716-446655440000",
      "channel_id": "770e8400-e29b-41d4-a716-446655440002",
      "author": {
        "id": "550e8400-e29b-41d4-a716-446655440000",
        "username": "alice",
        "avatar_url": null
      },
      "content": "Hello **world**!",
      "thread_id": null,
      "is_thread_root": true,
      "reply_count": 3,
      "reactions": [
        { "emoji": "👍", "count": 2, "users": ["alice", "bob"] },
        { "emoji": "🎉", "count": 1, "users": ["charlie"] }
      ],
      "edited_at": null,
      "created_at": "2026-02-23T10:30:00Z",
      "thread_preview": [
        {
          "id": "880e8400-e29b-41d4-a716-446655440010",
          "author": { "id": "...", "username": "bob", "avatar_url": null },
          "content": "Great point!",
          "created_at": "2026-02-23T10:32:00Z"
        }
      ]
    }
  ],
  "pagination": {
    "next_cursor": "eyJjcmVhdGVkX2F0IjoiMjAyNi0wMi0yM1QxMDoyOTowMFoiLCJpZCI6Ijg4MGU4NDAwLTEifQ",
    "has_more": true
  }
}
```

#### Load More (Cursor)

```http
GET /api/v1/channels/770e8400/messages?limit=50&cursor=eyJjcmVhdGVkX2F0IjoiMjAyNi0wMi0yM1QxMDoyOTowMFoiLCJpZCI6Ijg4MGU4NDAwLTEifQ
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

#### Edit Message

```http
PATCH /api/v1/messages/880e8400-e29b-41d4-a716-446655440000
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
Content-Type: application/json

{
  "content": "Hello **world**! (edited)"
}
```

```json
{
  "id": "880e8400-e29b-41d4-a716-446655440000",
  "content": "Hello **world**! (edited)",
  "edited_at": "2026-02-23T10:35:00Z",
  "edit_count": 1
}
```

#### Get Edit History

```http
GET /api/v1/messages/880e8400-e29b-41d4-a716-446655440000/edits
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

```json
{
  "edits": [
    {
      "previous_content": "Hello **world**!",
      "edited_at": "2026-02-23T10:35:00Z"
    }
  ]
}
```

#### Delete Message

```http
DELETE /api/v1/messages/880e8400-e29b-41d4-a716-446655440000
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

```json
{
  "message": "Message deleted"
}
```

#### Add Reaction

```http
POST /api/v1/messages/880e8400-e29b-41d4-a716-446655440000/reactions
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
Content-Type: application/json

{
  "emoji": "👍"
}
```

```json
{
  "message": "Reaction added"
}
```

#### Mark Channel as Read

```http
PUT /api/v1/channels/770e8400-e29b-41d4-a716-446655440002/read
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
Content-Type: application/json

{
  "message_id": "880e8400-e29b-41d4-a716-446655440000"
}
```

```json
{
  "message": "Channel marked as read"
}
```

#### Get Unread Count

```http
GET /api/v1/channels/770e8400-e29b-41d4-a716-446655440002/unread
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

```json
{
  "channel_id": "770e8400-e29b-41d4-a716-446655440002",
  "unread_count": 12,
  "last_read_at": "2026-02-23T10:00:00Z",
  "last_message_at": "2026-02-23T10:45:00Z"
}
```

---

## 6. WebSocket Event Protocol

### New Event Types

| Event Type | Direction | Description |
|------------|-----------|-------------|
| `message.create` | Server -> Client | New message in subscribed channel |
| `message.update` | Server -> Client | Message edited |
| `message.delete` | Server -> Client | Message deleted |
| `reaction.add` | Server -> Client | Reaction added to message |
| `reaction.remove` | Server -> Client | Reaction removed from message |
| `thread.reply` | Server -> Client | New reply in a thread |

### Event Payloads

```go
// internal/presence/events.go additions

const (
    // ... existing events ...

    // Message events
    EventTypeMessageCreate = "message.create"
    EventTypeMessageUpdate = "message.update"
    EventTypeMessageDelete = "message.delete"
    EventTypeReactionAdd   = "reaction.add"
    EventTypeReactionRemove = "reaction.remove"
    EventTypeThreadReply   = "thread.reply"
)

// Message create payload
type MessageCreatePayload struct {
    ID         string         `json:"id"`
    ChannelID  string         `json:"channel_id"`
    SpaceID    string         `json:"space_id"`
    Author     UserBrief      `json:"author"`
    Content    string         `json:"content"`
    ThreadID   *string        `json:"thread_id,omitempty"`
    CreatedAt  time.Time      `json:"created_at"`
    Nonce      string         `json:"nonce,omitempty"` // For optimistic update matching
}

// Message update payload
type MessageUpdatePayload struct {
    ID         string    `json:"id"`
    ChannelID  string    `json:"channel_id"`
    SpaceID    string    `json:"space_id"`
    Content    string    `json:"content"`
    EditedAt   time.Time `json:"edited_at"`
    EditCount  int       `json:"edit_count"`
}

// Message delete payload
type MessageDeletePayload struct {
    ID        string `json:"id"`
    ChannelID string `json:"channel_id"`
    SpaceID   string `json:"space_id"`
}

// Reaction payload
type ReactionPayload struct {
    MessageID string    `json:"message_id"`
    ChannelID string    `json:"channel_id"`
    SpaceID   string    `json:"space_id"`
    UserID    string    `json:"user_id"`
    Username  string    `json:"username"`
    Emoji     string    `json:"emoji"`
}

// User brief for message author
type UserBrief struct {
    ID        string  `json:"id"`
    Username  string  `json:"username"`
    AvatarURL *string `json:"avatar_url,omitempty"`
}
```

### Hub Extensions

```go
// internal/presence/hub.go additions

// PublishMessage broadcasts a new message to all space subscribers
func (h *Hub) PublishMessage(ctx context.Context, spaceID string, payload MessageCreatePayload) {
    payload.SpaceID = spaceID
    h.BroadcastToSpace(spaceID, Event{
        Type:      EventTypeMessageCreate,
        Timestamp: time.Now(),
        Payload:   payload,
    })
}

// PublishMessageUpdate broadcasts a message edit
func (h *Hub) PublishMessageUpdate(ctx context.Context, spaceID string, payload MessageUpdatePayload) {
    payload.SpaceID = spaceID
    h.BroadcastToSpace(spaceID, Event{
        Type:      EventTypeMessageUpdate,
        Timestamp: time.Now(),
        Payload:   payload,
    })
}

// PublishMessageDelete broadcasts a message deletion
func (h *Hub) PublishMessageDelete(ctx context.Context, spaceID string, payload MessageDeletePayload) {
    payload.SpaceID = spaceID
    h.BroadcastToSpace(spaceID, Event{
        Type:      EventTypeMessageDelete,
        Timestamp: time.Now(),
        Payload:   payload,
    })
}

// PublishReaction broadcasts a reaction change
func (h *Hub) PublishReaction(ctx context.Context, spaceID string, add bool, payload ReactionPayload) {
    payload.SpaceID = spaceID
    eventType := EventTypeReactionAdd
    if !add {
        eventType = EventTypeReactionRemove
    }
    h.BroadcastToSpace(spaceID, Event{
        Type:      eventType,
        Timestamp: time.Now(),
        Payload:   payload,
    })
}
```

---

## 7. Threading Model

### Design

- **Flat threads:** One level deep only. Replies to thread messages go to the same thread.
- **Thread root:** First message that receives a reply becomes a thread root (`is_thread_root = true`).
- **Display:** First 3 replies visible inline, indented. "Load N more replies" button for the rest.
- **Reply count:** Cached on parent message for performance.
- **Thread preview:** API returns first 3 replies with parent message to avoid extra requests.

### Thread Queries

```sql
-- queries/messages.sql additions

-- name: GetThreadReplies :many
SELECT
    m.id,
    m.channel_id,
    m.author_id,
    m.content,
    m.thread_id,
    m.edited_at,
    m.created_at,
    u.username AS author_username,
    u.avatar_url AS author_avatar_url
FROM messages m
JOIN users u ON u.id = m.author_id
WHERE m.thread_id = $1
  AND m.deleted_at IS NULL
ORDER BY m.created_at ASC;

-- name: MarkAsThreadRoot :exec
UPDATE messages
SET is_thread_root = TRUE
WHERE id = $1;

-- name: IncrementReplyCount :exec
UPDATE messages
SET reply_count = reply_count + 1
WHERE id = $1;

-- name: DecrementReplyCount :exec
UPDATE messages
SET reply_count = GREATEST(reply_count - 1, 0)
WHERE id = $1;
```

### Thread Component

```tsx
// src/components/chat/Thread.tsx

interface ThreadProps {
  parentMessage: Message;
  previewReplies: Message[];  // First 3 replies from API
  totalReplyCount: number;
  onReply: (content: string) => void;
  onLoadMore: () => Promise<Message[]>;
}

export function Thread({
  parentMessage,
  previewReplies,
  totalReplyCount,
  onReply,
  onLoadMore
}: ThreadProps) {
  const [isReplying, setIsReplying] = useState(false);
  const [allReplies, setAllReplies] = useState<Message[] | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  const displayedReplies = allReplies || previewReplies;
  const hasMore = !allReplies && totalReplyCount > previewReplies.length;
  const remainingCount = totalReplyCount - previewReplies.length;

  const handleLoadMore = async () => {
    setIsLoading(true);
    try {
      const replies = await onLoadMore();
      setAllReplies(replies);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="mt-2 ml-8 border-l-2 border-zinc-700 pl-4">
      {displayedReplies.map((reply) => (
        <MessageItem
          key={reply.id}
          message={reply}
          compact
          isThreadReply
        />
      ))}

      {hasMore && (
        <button
          onClick={handleLoadMore}
          disabled={isLoading}
          className="text-sm text-blue-400 hover:text-blue-300 mt-2"
        >
          {isLoading ? 'Loading...' : `Load ${remainingCount} more ${remainingCount === 1 ? 'reply' : 'replies'}`}
        </button>
      )}

      {isReplying ? (
        <ThreadReplyInput
          onSubmit={(content) => {
            onReply(content);
            setIsReplying(false);
          }}
          onCancel={() => setIsReplying(false)}
        />
      ) : (
        <button
          onClick={() => setIsReplying(true)}
          className="text-sm text-zinc-400 hover:text-zinc-200 mt-2"
        >
          Reply to thread...
        </button>
      )}
    </div>
  );
}
```

---

## 8. Reactions System

### Curated Emoji Picker

```tsx
// src/components/chat/ReactionPicker.tsx

import { useState } from 'react';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { SmilePlus } from 'lucide-react';
import { useEmojiStore } from '@/store/emojiStore';

interface ReactionPickerProps {
  onSelect: (emoji: string) => void;
}

export function ReactionPicker({ onSelect }: ReactionPickerProps) {
  const [open, setOpen] = useState(false);
  const { emoji, categories } = useEmojiStore();

  const handleSelect = (e: string) => {
    onSelect(e);
    setOpen(false);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button className="p-1 hover:bg-zinc-700 rounded">
          <SmilePlus className="h-4 w-4 text-zinc-400" />
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-80 p-2" align="start">
        {categories.map((category) => (
          <div key={category} className="mb-4">
            <h4 className="text-xs text-zinc-400 uppercase mb-2">{category}</h4>
            <div className="grid grid-cols-8 gap-1">
              {emoji
                .filter((e) => e.category === category)
                .map((e) => (
                  <button
                    key={e.emoji}
                    onClick={() => handleSelect(e.emoji)}
                    className="p-1 text-xl hover:bg-zinc-700 rounded"
                    title={e.name}
                  >
                    {e.emoji}
                  </button>
                ))}
            </div>
          </div>
        ))}
      </PopoverContent>
    </Popover>
  );
}
```

### Reaction Display

```tsx
// src/components/chat/ReactionBar.tsx

interface Reaction {
  emoji: string;
  count: number;
  users: string[];
  hasReacted: boolean;
}

interface ReactionBarProps {
  reactions: Reaction[];
  onToggle: (emoji: string) => void;
}

export function ReactionBar({ reactions, onToggle }: ReactionBarProps) {
  if (reactions.length === 0) return null;

  return (
    <div className="flex flex-wrap gap-1 mt-1">
      {reactions.map((reaction) => (
        <button
          key={reaction.emoji}
          onClick={() => onToggle(reaction.emoji)}
          className={cn(
            'inline-flex items-center gap-1 px-2 py-0.5 rounded text-sm',
            'border transition-colors',
            reaction.hasReacted
              ? 'bg-blue-500/20 border-blue-500/50 text-blue-300'
              : 'bg-zinc-800 border-zinc-700 hover:border-zinc-600'
          )}
          title={reaction.users.join(', ')}
        >
          <span>{reaction.emoji}</span>
          <span className="text-xs">{reaction.count}</span>
        </button>
      ))}
    </div>
  );
}
```

### Emoji Shortcode Autocomplete

When users type `:` followed by characters, show an autocomplete popup with matching emoji:

```tsx
// src/components/chat/EmojiAutocomplete.tsx

interface EmojiAutocompleteProps {
  query: string;  // Text after the ":"
  onSelect: (emoji: string) => void;
  onClose: () => void;
}

export function EmojiAutocomplete({ query, onSelect, onClose }: EmojiAutocompleteProps) {
  const { emoji } = useEmojiStore();

  const matches = emoji
    .filter((e) => e.name.toLowerCase().includes(query.toLowerCase()))
    .slice(0, 8);  // Show max 8 suggestions

  if (matches.length === 0) return null;

  return (
    <div className="absolute bottom-full left-0 mb-2 bg-zinc-800 border border-zinc-700 rounded-lg shadow-lg p-1 w-64">
      {matches.map((e, index) => (
        <button
          key={e.emoji}
          onClick={() => onSelect(e.emoji)}
          className={cn(
            'w-full flex items-center gap-2 px-2 py-1 rounded text-left',
            'hover:bg-zinc-700'
          )}
        >
          <span className="text-lg">{e.emoji}</span>
          <span className="text-sm text-zinc-300">:{e.name}:</span>
        </button>
      ))}
    </div>
  );
}
```

**Trigger behavior:**
- Show popup when user types `:` followed by 2+ characters
- Filter emoji by name (e.g., `:thu` matches `thumbsup`, `thumbsdown`)
- Navigate with arrow keys, select with Enter or click
- Close on Escape or clicking outside
- Replace `:shortcode:` with actual emoji character on selection

### Reaction Validation

```go
// internal/messages/reactions.go

package messages

import (
    "context"
    "errors"
)

var ErrInvalidEmoji = errors.New("emoji not in curated set")

type ReactionService struct {
    queries     *db.Queries
    emojiSet    map[string]bool
    presenceHub PresenceHub
}

func (s *ReactionService) AddReaction(ctx context.Context, messageID, userID, emoji string) error {
    // Validate emoji is in curated set
    if !s.emojiSet[emoji] {
        return ErrInvalidEmoji
    }

    // Add to database (upsert - ignore if already exists)
    err := s.queries.AddReaction(ctx, db.AddReactionParams{
        MessageID: messageID,
        UserID:    userID,
        Emoji:     emoji,
    })
    if err != nil {
        return err
    }

    // Get message for space ID
    msg, err := s.queries.GetMessage(ctx, messageID)
    if err != nil {
        return err
    }

    // Broadcast to space
    s.presenceHub.PublishReaction(ctx, msg.SpaceID, true, ReactionPayload{
        MessageID: messageID,
        UserID:    userID,
        Emoji:     emoji,
    })

    return nil
}
```

---

## 9. Read State Tracking

### Design

- Track `last_read_at` timestamp and `last_read_message_id` per membership
- Update on:
  - Explicit "mark as read" action
  - Scrolling to bottom of channel
  - Switching away from channel
- Query unread count as messages after `last_read_at`

### Queries

```sql
-- queries/messages.sql additions

-- name: GetUnreadCount :one
SELECT COUNT(*)::int AS unread_count
FROM messages m
WHERE m.channel_id = $1
  AND m.deleted_at IS NULL
  AND m.created_at > COALESCE(
    (SELECT last_read_at FROM memberships WHERE user_id = $2 AND space_id = $3),
    '1970-01-01'::timestamptz
  );

-- name: UpdateReadState :exec
UPDATE memberships
SET last_read_at = $4,
    last_read_message_id = $5
WHERE user_id = $1
  AND space_id = $2;

-- name: GetChannelUnreadCounts :many
SELECT
    c.id AS channel_id,
    COALESCE(
        (SELECT COUNT(*)::int
         FROM messages m
         WHERE m.channel_id = c.id
           AND m.deleted_at IS NULL
           AND m.created_at > COALESCE(mem.last_read_at, '1970-01-01'::timestamptz)
        ), 0
    ) AS unread_count
FROM channels c
JOIN memberships mem ON mem.space_id = c.space_id AND mem.user_id = $1
WHERE c.space_id = $2
  AND c.type = 'text'
  AND c.deleted_at IS NULL;
```

### Client Unread Indicator

```tsx
// src/components/layout/ChannelListItem.tsx

interface ChannelListItemProps {
  channel: Channel;
  unreadCount: number;
  isSelected: boolean;
  onClick: () => void;
}

export function ChannelListItem({
  channel,
  unreadCount,
  isSelected,
  onClick,
}: ChannelListItemProps) {
  return (
    <button
      onClick={onClick}
      className={cn(
        'w-full flex items-center gap-2 px-2 py-1 rounded',
        isSelected ? 'bg-zinc-700' : 'hover:bg-zinc-800',
        unreadCount > 0 && 'font-semibold'
      )}
    >
      <Hash className="h-4 w-4 text-zinc-400" />
      <span className="flex-1 text-left truncate">{channel.name}</span>
      {unreadCount > 0 && (
        <span className="px-1.5 py-0.5 text-xs bg-blue-500 rounded-full">
          {unreadCount > 99 ? '99+' : unreadCount}
        </span>
      )}
    </button>
  );
}
```

---

## 10. Rate Limiting

### Message Rate Limit

| Scope | Limit | Window | Implementation |
|-------|-------|--------|----------------|
| Messages per user | 5 | 5 seconds | Sliding window in Redis |
| Edits per message | 3 | 1 minute | Sliding window in Redis |
| Reactions per user | 20 | 1 minute | Sliding window in Redis |

### Implementation

```go
// internal/ratelimit/messages.go

package ratelimit

import (
    "context"
    "fmt"
    "time"

    "github.com/redis/go-redis/v9"
)

type MessageRateLimiter struct {
    redis *redis.Client
}

func (r *MessageRateLimiter) AllowMessage(ctx context.Context, userID string) (bool, int, error) {
    key := fmt.Sprintf("ratelimit:msg:%s", userID)
    window := 5 * time.Second
    limit := 5

    return r.checkSlidingWindow(ctx, key, window, limit)
}

func (r *MessageRateLimiter) AllowEdit(ctx context.Context, messageID string) (bool, int, error) {
    key := fmt.Sprintf("ratelimit:edit:%s", messageID)
    window := 1 * time.Minute
    limit := 3

    return r.checkSlidingWindow(ctx, key, window, limit)
}

func (r *MessageRateLimiter) AllowReaction(ctx context.Context, userID string) (bool, int, error) {
    key := fmt.Sprintf("ratelimit:react:%s", userID)
    window := 1 * time.Minute
    limit := 20

    return r.checkSlidingWindow(ctx, key, window, limit)
}

func (r *MessageRateLimiter) checkSlidingWindow(ctx context.Context, key string, window time.Duration, limit int) (bool, int, error) {
    now := time.Now()
    windowStart := now.Add(-window)

    pipe := r.redis.Pipeline()

    // Remove old entries
    pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart.UnixNano()))

    // Add current request
    pipe.ZAdd(ctx, key, redis.Z{Score: float64(now.UnixNano()), Member: now.UnixNano()})

    // Count requests in window
    countCmd := pipe.ZCard(ctx, key)

    // Set TTL
    pipe.Expire(ctx, key, window)

    _, err := pipe.Exec(ctx)
    if err != nil {
        return false, 0, err
    }

    count := int(countCmd.Val())
    remaining := limit - count
    if remaining < 0 {
        remaining = 0
    }

    return count <= limit, remaining, nil
}
```

---

## 11. Client Components

### Component Hierarchy

```
ChatPanel
|-- MessageList
|   |-- MessageDateDivider
|   |-- MessageItem
|   |   |-- MessageContent (markdown rendered)
|   |   |-- ReactionBar
|   |   |-- MessageActions (edit, delete, react, reply)
|   |   +-- Thread (if is_thread_root)
|   |       |-- MessageItem (compact)
|   |       +-- ThreadReplyInput
|   +-- UnreadDivider
|-- TypingIndicator
+-- MessageInput
    |-- MarkdownToolbar
    +-- EmojiPicker

EditHistoryModal
ReactionPicker
```

### MessageList Component

```tsx
// src/components/chat/MessageList.tsx

import { useRef, useEffect, useCallback } from 'react';
import { useChatStore } from '@/store/chatStore';
import { MessageItem } from './MessageItem';
import { MessageDateDivider } from './MessageDateDivider';
import { UnreadDivider } from './UnreadDivider';

interface MessageListProps {
  channelId: string;
}

export function MessageList({ channelId }: MessageListProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const { messages, hasMore, loadMore, lastReadAt, isLoading } = useChatStore();

  // Load more on scroll to top
  const handleScroll = useCallback((e: React.UIEvent<HTMLDivElement>) => {
    const { scrollTop } = e.currentTarget;
    if (scrollTop < 100 && hasMore && !isLoading) {
      loadMore(channelId);
    }
  }, [channelId, hasMore, isLoading, loadMore]);

  // Scroll to bottom on new messages (if already at bottom)
  useEffect(() => {
    if (containerRef.current) {
      const { scrollHeight, scrollTop, clientHeight } = containerRef.current;
      const isAtBottom = scrollHeight - scrollTop - clientHeight < 100;
      if (isAtBottom) {
        containerRef.current.scrollTop = scrollHeight;
      }
    }
  }, [messages]);

  // Group messages by date
  const groupedMessages = groupMessagesByDate(messages);

  return (
    <div
      ref={containerRef}
      onScroll={handleScroll}
      className="flex-1 overflow-y-auto px-4"
    >
      {isLoading && (
        <div className="text-center py-4 text-zinc-500">Loading...</div>
      )}

      {groupedMessages.map((group, groupIndex) => (
        <div key={group.date}>
          <MessageDateDivider date={group.date} />

          {group.messages.map((message, msgIndex) => {
            const showUnreadDivider =
              lastReadAt &&
              msgIndex > 0 &&
              new Date(message.created_at) > lastReadAt &&
              new Date(group.messages[msgIndex - 1].created_at) <= lastReadAt;

            return (
              <div key={message.id}>
                {showUnreadDivider && <UnreadDivider />}
                <MessageItem
                  message={message}
                  showAuthor={shouldShowAuthor(group.messages, msgIndex)}
                />
              </div>
            );
          })}
        </div>
      ))}
    </div>
  );
}

function shouldShowAuthor(messages: Message[], index: number): boolean {
  if (index === 0) return true;
  const prev = messages[index - 1];
  const curr = messages[index];

  // Show author if different user or more than 5 minutes apart
  if (prev.author.id !== curr.author.id) return true;

  const timeDiff = new Date(curr.created_at).getTime() - new Date(prev.created_at).getTime();
  return timeDiff > 5 * 60 * 1000;
}
```

### MessageInput Component

```tsx
// src/components/chat/MessageInput.tsx

import { useState, useRef, useCallback, KeyboardEvent } from 'react';
import { Send, Bold, Italic, Code } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { useChatStore } from '@/store/chatStore';
import { EmojiPicker } from './EmojiPicker';

interface MessageInputProps {
  channelId: string;
  threadId?: string;
  placeholder?: string;
}

export function MessageInput({ channelId, threadId, placeholder }: MessageInputProps) {
  const [content, setContent] = useState('');
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const { sendMessage, sendTyping } = useChatStore();

  const handleSend = useCallback(async () => {
    const trimmed = content.trim();
    if (!trimmed) return;

    await sendMessage(channelId, trimmed, threadId);
    setContent('');
  }, [channelId, content, threadId, sendMessage]);

  const handleKeyDown = useCallback((e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  }, [handleSend]);

  const handleChange = useCallback((value: string) => {
    setContent(value);
    sendTyping(channelId);
  }, [channelId, sendTyping]);

  const insertMarkdown = useCallback((prefix: string, suffix: string = prefix) => {
    const textarea = textareaRef.current;
    if (!textarea) return;

    const start = textarea.selectionStart;
    const end = textarea.selectionEnd;
    const text = content;
    const selected = text.substring(start, end);

    const newText = text.substring(0, start) + prefix + selected + suffix + text.substring(end);
    setContent(newText);

    // Restore cursor position
    setTimeout(() => {
      textarea.focus();
      textarea.setSelectionRange(start + prefix.length, end + prefix.length);
    }, 0);
  }, [content]);

  const charCount = content.length;
  const isOverLimit = charCount > 2000;

  return (
    <div className="px-4 pb-4">
      <div className="bg-zinc-800 rounded-lg border border-zinc-700">
        {/* Toolbar */}
        <div className="flex items-center gap-1 px-2 py-1 border-b border-zinc-700">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => insertMarkdown('**')}
            title="Bold"
          >
            <Bold className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => insertMarkdown('*')}
            title="Italic"
          >
            <Italic className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => insertMarkdown('`')}
            title="Inline Code"
          >
            <Code className="h-4 w-4" />
          </Button>
          <div className="flex-1" />
          <EmojiPicker onSelect={(emoji) => setContent(content + emoji)} />
        </div>

        {/* Input */}
        <div className="flex items-end gap-2 p-2">
          <Textarea
            ref={textareaRef}
            value={content}
            onChange={(e) => handleChange(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={placeholder || 'Send a message...'}
            className="min-h-[40px] max-h-[200px] resize-none bg-transparent border-0 focus-visible:ring-0"
            rows={1}
          />
          <Button
            onClick={handleSend}
            disabled={!content.trim() || isOverLimit}
            size="icon"
          >
            <Send className="h-4 w-4" />
          </Button>
        </div>

        {/* Character count */}
        <div className="px-3 pb-1 text-right">
          <span className={cn('text-xs', isOverLimit ? 'text-red-400' : 'text-zinc-500')}>
            {charCount}/2000
          </span>
        </div>
      </div>
    </div>
  );
}
```

---

## 12. Client State Management

### Chat Store

```typescript
// src/store/chatStore.ts

import { create } from 'zustand';
import { api } from '@/lib/api';
import type { Message, Reaction } from '@/types/api';

interface OptimisticMessage extends Message {
  nonce: string;
  pending: boolean;
  failed: boolean;
}

interface ChatState {
  // Messages by channel
  messagesByChannel: Map<string, (Message | OptimisticMessage)[]>;

  // Pagination state
  cursors: Map<string, string | null>;
  hasMore: Map<string, boolean>;
  isLoading: boolean;

  // Read state
  lastReadByChannel: Map<string, Date>;
  unreadCounts: Map<string, number>;

  // Actions
  loadMessages: (channelId: string) => Promise<void>;
  loadMore: (channelId: string) => Promise<void>;
  sendMessage: (channelId: string, content: string, threadId?: string) => Promise<void>;
  editMessage: (messageId: string, content: string) => Promise<void>;
  deleteMessage: (messageId: string) => Promise<void>;
  addReaction: (messageId: string, emoji: string) => Promise<void>;
  removeReaction: (messageId: string, emoji: string) => Promise<void>;
  replyToThread: (parentId: string, content: string) => Promise<void>;
  markAsRead: (channelId: string, messageId: string) => Promise<void>;

  // WebSocket event handlers
  handleMessageCreate: (payload: MessageCreatePayload) => void;
  handleMessageUpdate: (payload: MessageUpdatePayload) => void;
  handleMessageDelete: (payload: MessageDeletePayload) => void;
  handleReactionAdd: (payload: ReactionPayload) => void;
  handleReactionRemove: (payload: ReactionPayload) => void;

  // Typing
  sendTyping: (channelId: string) => void;
}

export const useChatStore = create<ChatState>((set, get) => ({
  messagesByChannel: new Map(),
  cursors: new Map(),
  hasMore: new Map(),
  isLoading: false,
  lastReadByChannel: new Map(),
  unreadCounts: new Map(),

  loadMessages: async (channelId) => {
    set({ isLoading: true });

    try {
      const response = await api.get(`/channels/${channelId}/messages?limit=50`);
      const { data, pagination } = response.data;

      set((state) => {
        const messages = new Map(state.messagesByChannel);
        messages.set(channelId, data);

        const cursors = new Map(state.cursors);
        cursors.set(channelId, pagination.next_cursor);

        const hasMore = new Map(state.hasMore);
        hasMore.set(channelId, pagination.has_more);

        return { messagesByChannel: messages, cursors, hasMore, isLoading: false };
      });
    } catch (error) {
      set({ isLoading: false });
      throw error;
    }
  },

  sendMessage: async (channelId, content, threadId) => {
    const nonce = crypto.randomUUID();
    const user = useAuthStore.getState().user;

    const optimisticMessage: OptimisticMessage = {
      id: nonce,
      channel_id: channelId,
      author: user!,
      content,
      thread_id: threadId || null,
      is_thread_root: false,
      reply_count: 0,
      reactions: [],
      edited_at: null,
      created_at: new Date().toISOString(),
      nonce,
      pending: true,
      failed: false,
    };

    // Add optimistic message
    set((state) => {
      const messages = new Map(state.messagesByChannel);
      const channelMessages = messages.get(channelId) || [];
      messages.set(channelId, [...channelMessages, optimisticMessage]);
      return { messagesByChannel: messages };
    });

    try {
      const response = await api.post(`/channels/${channelId}/messages`, {
        content,
        thread_id: threadId,
        nonce,
      });

      // Replace optimistic with real message
      set((state) => {
        const messages = new Map(state.messagesByChannel);
        const channelMessages = messages.get(channelId) || [];
        const updated = channelMessages.map((m) =>
          (m as OptimisticMessage).nonce === nonce ? response.data : m
        );
        messages.set(channelId, updated);
        return { messagesByChannel: messages };
      });
    } catch (error) {
      // Mark as failed
      set((state) => {
        const messages = new Map(state.messagesByChannel);
        const channelMessages = messages.get(channelId) || [];
        const updated = channelMessages.map((m) =>
          (m as OptimisticMessage).nonce === nonce
            ? { ...m, pending: false, failed: true }
            : m
        );
        messages.set(channelId, updated);
        return { messagesByChannel: messages };
      });
      throw error;
    }
  },

  // WebSocket handlers
  handleMessageCreate: (payload) => {
    set((state) => {
      const messages = new Map(state.messagesByChannel);
      const channelMessages = messages.get(payload.channel_id) || [];

      // Check if we already have this message (by nonce or id)
      const exists = channelMessages.some(
        (m) => m.id === payload.id || (m as OptimisticMessage).nonce === payload.nonce
      );

      if (!exists) {
        messages.set(payload.channel_id, [...channelMessages, payload]);
      }

      return { messagesByChannel: messages };
    });
  },

  handleMessageUpdate: (payload) => {
    set((state) => {
      const messages = new Map(state.messagesByChannel);
      messages.forEach((channelMessages, channelId) => {
        const updated = channelMessages.map((m) =>
          m.id === payload.id
            ? { ...m, content: payload.content, edited_at: payload.edited_at }
            : m
        );
        messages.set(channelId, updated);
      });
      return { messagesByChannel: messages };
    });
  },

  handleMessageDelete: (payload) => {
    set((state) => {
      const messages = new Map(state.messagesByChannel);
      const channelMessages = messages.get(payload.channel_id) || [];
      const updated = channelMessages.map((m) =>
        m.id === payload.id ? { ...m, deleted_at: new Date().toISOString() } : m
      );
      messages.set(payload.channel_id, updated);
      return { messagesByChannel: messages };
    });
  },

  // ... additional methods
}));
```

---

## 13. Optimistic Updates

### Strategy

1. **Send:** Add message to local state immediately with `pending: true`
2. **Success:** Replace optimistic message with server response (matched by `nonce`)
3. **Failure:** Mark message as `failed: true`, show retry UI

### Failed Message UI

```tsx
// src/components/chat/FailedMessage.tsx

interface FailedMessageProps {
  message: OptimisticMessage;
  onRetry: () => void;
  onDiscard: () => void;
}

export function FailedMessage({ message, onRetry, onDiscard }: FailedMessageProps) {
  return (
    <div className="py-2 px-4 bg-red-900/20 border-l-2 border-red-500">
      <div className="flex items-center gap-2 text-sm text-red-400">
        <AlertCircle className="h-4 w-4" />
        <span>Failed to send</span>
      </div>
      <div className="mt-1 text-zinc-300">{message.content}</div>
      <div className="mt-2 flex gap-2">
        <Button size="sm" variant="outline" onClick={onRetry}>
          Retry
        </Button>
        <Button size="sm" variant="ghost" onClick={onDiscard}>
          Discard
        </Button>
      </div>
    </div>
  );
}
```

---

## 14. Configuration

### Config File Additions

```yaml
# config/config.yaml additions

messages:
  max_length: 2000
  edit_window_minutes: 15
  page_size: 50
  max_code_block_length: 1500

reactions:
  max_per_message: 20
  max_users_displayed: 10

rate_limits:
  messages:
    limit: 5
    window_seconds: 5
  edits:
    limit: 3
    window_seconds: 60
  reactions:
    limit: 20
    window_seconds: 60

markdown:
  allowed_elements:
    - strong
    - em
    - del
    - code
    - pre
    - a
  max_link_length: 2000
  auto_link_urls: true
```

---

## 15. Testing Strategy

### Unit Tests

```go
// internal/messages/service_test.go

func TestMessageService_Send(t *testing.T) {
    ctx := context.Background()

    tests := []struct {
        name    string
        content string
        wantErr error
    }{
        {"valid message", "Hello world", nil},
        {"empty message", "", ErrMessageEmpty},
        {"too long", strings.Repeat("a", 2001), ErrMessageTooLong},
        {"with markdown", "**bold** and `code`", nil},
        {"with code block", "```go\nfmt.Println()\n```", nil},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateContent(tt.content)
            assert.Equal(t, tt.wantErr, err)
        })
    }
}
```

### Integration Tests

```go
// internal/api/handlers/messages_integration_test.go

func TestMessageFlow(t *testing.T) {
    // 1. Send message
    // 2. Verify WebSocket broadcast
    // 3. Edit message
    // 4. Verify edit history
    // 5. Add reactions
    // 6. Delete message
    // 7. Verify soft delete placeholder
}

func TestThreadFlow(t *testing.T) {
    // 1. Send parent message
    // 2. Reply to thread
    // 3. Verify reply_count increment
    // 4. Load thread replies
}

func TestPagination(t *testing.T) {
    // 1. Create 100 messages
    // 2. Load first page (50)
    // 3. Load second page with cursor
    // 4. Verify ordering and completeness
}
```

---

## 16. Implementation Tasks

### Milestone 1: Database & Core Service

- [ ] Write migration `0003_messages.up.sql`
- [ ] Write migration `0003_messages.down.sql`
- [ ] Seed curated emoji set
- [ ] Add sqlc queries for messages
- [ ] Add sqlc queries for reactions
- [ ] Add sqlc queries for read state
- [ ] Implement MessageService
- [ ] Implement ReactionService
- [ ] Implement content validation
- [ ] Add message rate limiter

### Milestone 2: API Handlers

- [ ] Implement POST `/channels/:id/messages`
- [ ] Implement GET `/channels/:id/messages` (paginated)
- [ ] Implement GET `/messages/:id`
- [ ] Implement PATCH `/messages/:id`
- [ ] Implement DELETE `/messages/:id`
- [ ] Implement GET `/messages/:id/edits`
- [ ] Implement POST `/messages/:id/reactions`
- [ ] Implement DELETE `/messages/:id/reactions/:emoji`
- [ ] Implement GET `/messages/:id/thread`
- [ ] Implement POST `/messages/:id/thread`
- [ ] Implement PUT `/channels/:id/read`
- [ ] Implement GET `/channels/:id/unread`
- [ ] Implement GET `/emoji`
- [ ] Wire up routes in router

### Milestone 3: WebSocket Events

- [ ] Add message event types to presence/events.go
- [ ] Implement PublishMessage in hub
- [ ] Implement PublishMessageUpdate in hub
- [ ] Implement PublishMessageDelete in hub
- [ ] Implement PublishReaction in hub
- [ ] Integrate hub calls in message service
- [ ] Test WebSocket fan-out

### Milestone 4: Client Components

- [ ] Create MessageList component
- [ ] Create MessageItem component
- [ ] Create MessageContent (markdown renderer)
- [ ] Create MessageInput component
- [ ] Create MessageDateDivider component
- [ ] Create UnreadDivider component
- [ ] Create ReactionBar component
- [ ] Create ReactionPicker component
- [ ] Create Thread component
- [ ] Create EditMessageForm component
- [ ] Create EditHistoryModal component
- [ ] Create FailedMessage component

### Milestone 5: Client State

- [ ] Create chatStore
- [ ] Implement message loading/pagination
- [ ] Implement optimistic send
- [ ] Implement optimistic edit
- [ ] Implement optimistic delete
- [ ] Implement optimistic reactions
- [ ] Implement WebSocket event handlers
- [ ] Implement typing indicator sending
- [ ] Implement read state tracking
- [ ] Integrate with useWebSocket hook

### Milestone 6: Emoji & Reactions

- [ ] Create emojiStore
- [ ] Load curated emoji set from API
- [ ] Implement emoji picker UI
- [ ] Implement reaction toggle logic
- [ ] Implement emoji shortcode autocomplete component
- [ ] Integrate autocomplete with MessageInput
- [ ] Add keyboard navigation for autocomplete
- [ ] Test reaction validation

### Milestone 7: Testing

- [ ] Write message service unit tests
- [ ] Write reaction service unit tests
- [ ] Write content validation tests
- [ ] Write API handler tests
- [ ] Write pagination tests
- [ ] Write client store tests
- [ ] Integration test: full message flow
- [ ] Integration test: thread flow
- [ ] Manual testing checklist

### Milestone 8: Polish & Documentation

- [ ] Update OpenAPI spec with message endpoints
- [ ] Document WebSocket event protocol
- [ ] Add loading states to UI
- [ ] Add error states to UI
- [ ] Performance optimization (virtualized list if needed)
- [ ] Update README with Phase 3 features

---

## 17. Acceptance Criteria

### Message CRUD

- [ ] User can send a message to a text channel
- [ ] Messages appear instantly (optimistic update)
- [ ] Messages are persisted and survive page refresh
- [ ] User can edit their own messages within 15 minutes
- [ ] Edit shows "(edited)" indicator
- [ ] Edit history is viewable (author and admins only)
- [ ] User can delete their own messages anytime
- [ ] Deleted messages show "message was deleted" placeholder
- [ ] Admins can delete any message in their space

### Real-time Delivery

- [ ] New messages appear for all channel members via WebSocket
- [ ] Edits are reflected in real-time
- [ ] Deletes are reflected in real-time
- [ ] No duplicate messages from optimistic + WebSocket

### Pagination

- [ ] Initial load fetches 50 messages
- [ ] Scroll to top loads more messages
- [ ] Cursor-based pagination is stable
- [ ] Messages are ordered by created_at DESC (newest at bottom)

### Threading

- [ ] User can reply to any message
- [ ] First reply converts parent to thread root
- [ ] First 3 thread replies appear inline, indented
- [ ] Reply count is displayed on thread root
- [ ] "Load N more replies" button appears when >3 replies
- [ ] Loading more replies fetches full thread from API

### Reactions

- [ ] User can add reaction from curated emoji set
- [ ] User can remove their own reaction
- [ ] Reactions show count and list of users
- [ ] Own reaction is visually highlighted
- [ ] Invalid emoji is rejected
- [ ] Reactions update in real-time

### Emoji Shortcodes

- [ ] Typing `:` followed by 2+ chars shows autocomplete popup
- [ ] Autocomplete filters emoji by name
- [ ] Can navigate suggestions with arrow keys
- [ ] Enter or click selects emoji and replaces shortcode
- [ ] Escape closes autocomplete

### Markdown

- [ ] Bold, italic, strikethrough render correctly
- [ ] Inline code renders with background
- [ ] Code blocks render with syntax highlighting
- [ ] Code blocks have copy-to-clipboard button
- [ ] Links are clickable and open in new tab
- [ ] URLs auto-link
- [ ] XSS is prevented

### Read State

- [ ] Unread count shows on channel list
- [ ] Unread count updates when messages arrive
- [ ] Mark as read when scrolling to bottom
- [ ] Mark as read when switching channels
- [ ] "New messages" divider shows on channel open

### Rate Limiting

- [ ] Cannot send more than 5 messages in 5 seconds
- [ ] Cannot edit more than 3 times per minute
- [ ] Cannot add more than 20 reactions per minute
- [ ] Rate limit error message is user-friendly

### Error Handling

- [ ] Failed message shows retry/discard options
- [ ] Network errors show toast notification
- [ ] Validation errors show inline feedback
- [ ] Rate limit shows remaining time

### Performance

- [ ] Message list scrolls smoothly with 1000+ messages
- [ ] Initial load completes in < 1 second
- [ ] WebSocket reconnects automatically

---

## Summary

Phase 3 adds comprehensive text messaging to Redoubt with:

- **Full message lifecycle:** Send, edit (with history), delete (soft), with proper permission checks
- **Flat threading:** One level deep, inline expanded display
- **Curated reactions:** ~100 emoji with native system rendering
- **Basic markdown:** Bold, italic, code, links with syntax highlighting
- **Channel-level read tracking:** Unread counts and "new messages" divider
- **Real-time delivery:** WebSocket fan-out for all message events
- **Optimistic UI:** Instant feedback with server confirmation rollback
- **Rate limiting:** Spam protection on messages, edits, and reactions

The implementation builds on Phase 2's WebSocket infrastructure and maintains the single-VPS architecture while delivering a responsive chat experience.
