-- +goose Up
CREATE TABLE transfers (
  id TEXT PRIMARY KEY,
  direction TEXT NOT NULL CHECK (direction IN ('upload','download')),
  file_name TEXT NOT NULL DEFAULT '',
  size_bytes INTEGER NOT NULL DEFAULT 0,
  target_total INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  created_by TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  finished_at INTEGER
);
CREATE INDEX idx_transfers_created ON transfers(created_at DESC);

CREATE TABLE transfer_targets (
  id TEXT PRIMARY KEY,
  transfer_id TEXT NOT NULL REFERENCES transfers(id) ON DELETE CASCADE,
  host_id TEXT NOT NULL DEFAULT '',
  host_name_snapshot TEXT NOT NULL DEFAULT '',
  remote_path TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  bytes_done INTEGER NOT NULL DEFAULT 0,
  error TEXT NOT NULL DEFAULT '',
  started_at INTEGER,
  finished_at INTEGER
);
CREATE INDEX idx_transfer_targets_transfer ON transfer_targets(transfer_id);

-- +goose Down
DROP TABLE transfer_targets;
DROP TABLE transfers;
