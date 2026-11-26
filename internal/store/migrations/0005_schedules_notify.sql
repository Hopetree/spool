-- +goose Up
CREATE TABLE notify_channels (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  type TEXT NOT NULL CHECK (type IN ('webhook','feishu','dingtalk','wecom')),
  config_json TEXT NOT NULL DEFAULT '{}',
  secret_enc BLOB,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE notify_deliveries (
  id TEXT PRIMARY KEY,
  execution_id TEXT NOT NULL DEFAULT '',
  channel_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  request_snippet TEXT NOT NULL DEFAULT '',
  response_snippet TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE INDEX idx_deliveries_exec ON notify_deliveries(execution_id);
CREATE INDEX idx_deliveries_created ON notify_deliveries(created_at DESC);

CREATE TABLE schedules (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  target_kind TEXT NOT NULL CHECK (target_kind IN ('tool','flow')),
  target_ref_id TEXT NOT NULL,
  params_json TEXT NOT NULL DEFAULT '{}',
  sensitive_params_enc BLOB,
  host_ids_json TEXT NOT NULL DEFAULT '[]',
  cron_expr TEXT NOT NULL,
  concurrency_policy TEXT NOT NULL DEFAULT 'skip',
  notify_channel_ids_json TEXT NOT NULL DEFAULT '[]',
  notify_events TEXT NOT NULL DEFAULT 'failure',
  enabled INTEGER NOT NULL DEFAULT 1,
  last_run_at INTEGER,
  next_run_at INTEGER,
  created_by TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX idx_schedules_enabled ON schedules(enabled);

-- +goose Down
DROP TABLE schedules;
DROP TABLE notify_deliveries;
DROP TABLE notify_channels;
