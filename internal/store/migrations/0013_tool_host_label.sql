-- +goose Up
ALTER TABLE tool_versions DROP COLUMN host_label;

-- +goose Down
ALTER TABLE tool_versions ADD COLUMN host_label TEXT NOT NULL DEFAULT '';
