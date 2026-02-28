-- Messages table (main message storage with thread support)
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
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_id       UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    previous_content TEXT NOT NULL,
    edited_by        UUID NOT NULL REFERENCES users(id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
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

CREATE INDEX idx_emoji_set_category ON emoji_set(category, sort_order);

-- Extend memberships table for read state tracking
ALTER TABLE memberships
    ADD COLUMN last_read_at TIMESTAMPTZ,
    ADD COLUMN last_read_message_id UUID REFERENCES messages(id) ON DELETE SET NULL;

-- Message rate limiting tracking (for burst protection)
CREATE TABLE message_rate_limits (
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id      UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    message_count   INTEGER NOT NULL DEFAULT 1,
    window_start    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (user_id, channel_id)
);

CREATE INDEX idx_message_rate_limits_window ON message_rate_limits(window_start);

-- Seed curated emoji set
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
