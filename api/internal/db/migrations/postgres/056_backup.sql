-- Scheduled database backup tables (go-backup library)

CREATE TABLE IF NOT EXISTS backup_settings (
    id              VARCHAR(36)   PRIMARY KEY DEFAULT 'singleton',
    enabled         BOOLEAN       NOT NULL DEFAULT false,
    cron_expression VARCHAR(100)  NOT NULL DEFAULT '0 3 * * *',
    folder_id       VARCHAR(1000) NOT NULL DEFAULT '',
    provider_name   VARCHAR(50)   NOT NULL DEFAULT '',
    provider_config BYTEA,
    retention_full_days      INT NOT NULL DEFAULT 30,
    retention_alternate_days INT NOT NULL DEFAULT 60,
    retention_weekly_days    INT NOT NULL DEFAULT 365,
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS backup_history (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    status        VARCHAR(20) NOT NULL DEFAULT 'running'
                              CHECK (status IN ('running','success','failed')),
    triggered_by  VARCHAR(20) NOT NULL DEFAULT 'manual',
    filename      VARCHAR(500)  NOT NULL DEFAULT '',
    size_bytes    BIGINT        NOT NULL DEFAULT 0,
    provider_name VARCHAR(50)   NOT NULL DEFAULT '',
    file_id       VARCHAR(1000) NOT NULL DEFAULT '',
    file_url      VARCHAR(2000) NOT NULL DEFAULT '',
    error_message TEXT          NOT NULL DEFAULT '',
    started_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    finished_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_backup_history_started_at ON backup_history(started_at DESC);
