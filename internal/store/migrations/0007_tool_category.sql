-- +goose Up
ALTER TABLE tools ADD COLUMN category TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_tools_category ON tools(category);

-- +goose Down
DROP INDEX idx_tools_category;
ALTER TABLE tools DROP COLUMN category;
