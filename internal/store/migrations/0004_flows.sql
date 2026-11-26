-- +goose Up
CREATE TABLE flows (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL DEFAULT '',
  params_json TEXT NOT NULL DEFAULT '[]',
  failure_policy TEXT NOT NULL DEFAULT 'abort',
  max_duration_sec INTEGER NOT NULL DEFAULT 3600,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_by TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE flow_stages (
  id TEXT PRIMARY KEY,
  flow_id TEXT NOT NULL REFERENCES flows(id) ON DELETE CASCADE,
  idx INTEGER NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  always_run INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_flow_stages_flow ON flow_stages(flow_id);

CREATE TABLE flow_steps (
  id TEXT PRIMARY KEY,
  stage_id TEXT NOT NULL REFERENCES flow_stages(id) ON DELETE CASCADE,
  idx INTEGER NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  tool_id TEXT NOT NULL,
  param_mapping_json TEXT NOT NULL DEFAULT '{}',
  timeout_sec INTEGER NOT NULL DEFAULT 0,
  retry_count INTEGER NOT NULL DEFAULT 0,
  retry_delay_sec INTEGER NOT NULL DEFAULT 0,
  enabled INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX idx_flow_steps_stage ON flow_steps(stage_id);
CREATE INDEX idx_flow_steps_tool ON flow_steps(tool_id);

-- +goose Down
DROP TABLE flow_steps;
DROP TABLE flow_stages;
DROP TABLE flows;
