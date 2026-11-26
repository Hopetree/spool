-- +goose Up
CREATE TABLE tools (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  current_version_id TEXT NOT NULL DEFAULT '',
  created_by TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE tool_versions (
  id TEXT PRIMARY KEY,
  tool_id TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
  version_no INTEGER NOT NULL,
  type TEXT NOT NULL CHECK (type IN ('shell','python')),
  interpreter TEXT NOT NULL,
  content TEXT NOT NULL,
  params_json TEXT NOT NULL DEFAULT '[]',
  prepare_command TEXT NOT NULL DEFAULT '',
  default_timeout_sec INTEGER NOT NULL DEFAULT 300,
  sudo INTEGER NOT NULL DEFAULT 0,
  content_hash TEXT NOT NULL,
  change_note TEXT NOT NULL DEFAULT '',
  created_by TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  UNIQUE(tool_id, version_no)
);

CREATE TABLE executions (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT 'manual',
  ref_id TEXT NOT NULL DEFAULT '',
  schedule_id TEXT NOT NULL DEFAULT '',
  name_snapshot TEXT NOT NULL DEFAULT '',
  params_json TEXT NOT NULL DEFAULT '{}',
  sensitive_params_enc BLOB,
  plan_json TEXT NOT NULL DEFAULT '{}',
  host_total INTEGER NOT NULL DEFAULT 0,
  host_success INTEGER NOT NULL DEFAULT 0,
  host_failed INTEGER NOT NULL DEFAULT 0,
  host_cancelled INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  failure_reason TEXT NOT NULL DEFAULT '',
  error_summary TEXT NOT NULL DEFAULT '',
  cancel_requested INTEGER NOT NULL DEFAULT 0,
  created_by TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  started_at INTEGER,
  finished_at INTEGER
);
CREATE INDEX idx_exec_created ON executions(created_at DESC);
CREATE INDEX idx_exec_status ON executions(status);
CREATE INDEX idx_exec_ref ON executions(ref_id);

CREATE TABLE host_runs (
  id TEXT PRIMARY KEY,
  execution_id TEXT NOT NULL REFERENCES executions(id) ON DELETE CASCADE,
  host_id TEXT NOT NULL DEFAULT '',
  host_name_snapshot TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  failure_reason TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  started_at INTEGER,
  finished_at INTEGER
);
CREATE INDEX idx_host_runs_exec ON host_runs(execution_id);

CREATE TABLE step_runs (
  id TEXT PRIMARY KEY,
  host_run_id TEXT NOT NULL REFERENCES host_runs(id) ON DELETE CASCADE,
  stage_idx INTEGER NOT NULL DEFAULT 0,
  step_idx INTEGER NOT NULL DEFAULT 0,
  name_snapshot TEXT NOT NULL DEFAULT '',
  tool_id TEXT NOT NULL DEFAULT '',
  tool_version_id TEXT NOT NULL DEFAULT '',
  tool_version_no INTEGER NOT NULL DEFAULT 0,
  content_hash TEXT NOT NULL DEFAULT '',
  interpreter TEXT NOT NULL DEFAULT '',
  sudo INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  failure_reason TEXT NOT NULL DEFAULT '',
  exit_code INTEGER,
  error_message TEXT NOT NULL DEFAULT '',
  attempt INTEGER NOT NULL DEFAULT 0,
  started_at INTEGER,
  finished_at INTEGER,
  log_events INTEGER NOT NULL DEFAULT 0,
  log_bytes INTEGER NOT NULL DEFAULT 0,
  truncated INTEGER NOT NULL DEFAULT 0,
  dropped_bytes INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_step_runs_host ON step_runs(host_run_id);

-- +goose Down
DROP TABLE step_runs;
DROP TABLE host_runs;
DROP TABLE executions;
DROP TABLE tool_versions;
DROP TABLE tools;
