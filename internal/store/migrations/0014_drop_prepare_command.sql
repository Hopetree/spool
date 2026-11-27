-- +goose Up
ALTER TABLE tool_versions DROP COLUMN prepare_command;

-- +goose Down
ALTER TABLE tool_versions ADD COLUMN prepare_command TEXT NOT NULL DEFAULT '';
