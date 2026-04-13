CREATE TABLE IF NOT EXISTS playback_sessions (
    user_id    TEXT        NOT NULL PRIMARY KEY,
    track_id   TEXT        NOT NULL,
    position   INTEGER     NOT NULL DEFAULT 0,
    status     VARCHAR(10) NOT NULL DEFAULT 'paused',
    device_id  TEXT        NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_playback_sessions_updated_at ON playback_sessions (updated_at);
