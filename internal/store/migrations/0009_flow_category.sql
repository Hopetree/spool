-- +goose Up
ALTER TABLE flows ADD COLUMN category TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_flows_category ON flows(category);

-- +goose Down
DROP INDEX idx_flows_category;
ALTER TABLE flows DROP COLUMN category;
