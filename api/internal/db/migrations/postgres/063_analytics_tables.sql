-- Migration 063: Analytics tables for comprehensive user engagement tracking

-- 1. Page views — frontend sends beacon on each SPA navigation
CREATE TABLE IF NOT EXISTS page_views (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    path        TEXT NOT NULL,
    referrer    TEXT,
    session_id  TEXT NOT NULL,
    duration_ms INTEGER,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_page_views_user_id ON page_views(user_id);
CREATE INDEX IF NOT EXISTS idx_page_views_created_at ON page_views(created_at);
CREATE INDEX IF NOT EXISTS idx_page_views_session ON page_views(session_id);

-- 2. API request log — middleware logs every authenticated API request
CREATE TABLE IF NOT EXISTS api_request_log (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key_id  BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,
    method      TEXT NOT NULL,
    path        TEXT NOT NULL,
    status_code INTEGER NOT NULL,
    duration_ms INTEGER NOT NULL,
    agent_name  TEXT,
    ip_address  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_api_request_log_user_id ON api_request_log(user_id);
CREATE INDEX IF NOT EXISTS idx_api_request_log_api_key ON api_request_log(api_key_id);
CREATE INDEX IF NOT EXISTS idx_api_request_log_created ON api_request_log(created_at);
CREATE INDEX IF NOT EXISTS idx_api_request_log_path ON api_request_log(path);

-- 3. User sessions — aggregated session data for time-on-site tracking
CREATE TABLE IF NOT EXISTS user_sessions (
    id                BIGSERIAL PRIMARY KEY,
    user_id           BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id        TEXT NOT NULL UNIQUE,
    started_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    page_count        INTEGER NOT NULL DEFAULT 1,
    total_duration_ms BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_started ON user_sessions(started_at);
