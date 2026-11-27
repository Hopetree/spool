-- +goose Up
ALTER TABLE flow_nodes ADD COLUMN host_source TEXT NOT NULL DEFAULT '';
ALTER TABLE flow_nodes ADD COLUMN host_ref_node_id TEXT NOT NULL DEFAULT '';
ALTER TABLE schedules ADD COLUMN node_host_ids_json TEXT NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE schedules DROP COLUMN node_host_ids_json;
ALTER TABLE flow_nodes DROP COLUMN host_ref_node_id;
ALTER TABLE flow_nodes DROP COLUMN host_source;
