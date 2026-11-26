-- +goose Up
CREATE TABLE credentials (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  auth_type TEXT NOT NULL CHECK (auth_type IN ('password','private_key')),
  username TEXT NOT NULL,
  secret_enc BLOB NOT NULL,
  passphrase_enc BLOB,
  description TEXT NOT NULL DEFAULT '',
  created_by TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE host_groups (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE hosts (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  address TEXT NOT NULL,
  port INTEGER NOT NULL DEFAULT 22,
  group_id TEXT REFERENCES host_groups(id) ON DELETE SET NULL,
  credential_id TEXT NOT NULL REFERENCES credentials(id) ON DELETE RESTRICT,
  enabled INTEGER NOT NULL DEFAULT 1,
  fingerprint TEXT NOT NULL DEFAULT '',
  fingerprint_type TEXT NOT NULL DEFAULT '',
  capability_json TEXT NOT NULL DEFAULT '{}',
  last_test_at INTEGER,
  last_test_ok INTEGER,
  note TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX idx_hosts_group ON hosts(group_id);
CREATE INDEX idx_hosts_credential ON hosts(credential_id);

CREATE TABLE host_tags (
  host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
  tag TEXT NOT NULL,
  PRIMARY KEY (host_id, tag)
);
CREATE INDEX idx_host_tags_tag ON host_tags(tag);

-- +goose Down
DROP TABLE host_tags;
DROP TABLE hosts;
DROP TABLE host_groups;
DROP TABLE credentials;
