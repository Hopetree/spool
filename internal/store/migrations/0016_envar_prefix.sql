-- +goose Up
DELETE FROM platform_env_vars WHERE name NOT GLOB 'SP_[A-Z_]*';

-- +goose Down
