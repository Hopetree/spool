-- +goose Up
ALTER TABLE flow_nodes ADD COLUMN host_ids_json TEXT NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE flow_nodes DROP COLUMN host_ids_json;
