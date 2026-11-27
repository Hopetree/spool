-- +goose Up
ALTER TABLE tool_versions ADD COLUMN host_label TEXT NOT NULL DEFAULT '';
ALTER TABLE tool_versions ADD COLUMN host_default_ids_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE schedules DROP COLUMN node_host_ids_json;
ALTER TABLE flow_nodes DROP COLUMN host_source;
ALTER TABLE flow_nodes DROP COLUMN host_ref_node_id;
ALTER TABLE flow_nodes DROP COLUMN host_ids_json;

-- +goose Down
ALTER TABLE flow_nodes ADD COLUMN host_ids_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE flow_nodes ADD COLUMN host_ref_node_id TEXT NOT NULL DEFAULT '';
ALTER TABLE flow_nodes ADD COLUMN host_source TEXT NOT NULL DEFAULT '';
ALTER TABLE schedules ADD COLUMN node_host_ids_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE tool_versions DROP COLUMN host_default_ids_json;
ALTER TABLE tool_versions DROP COLUMN host_label;
