-- +goose Up
ALTER TABLE user_team_roles ADD COLUMN alias TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE user_team_roles DROP COLUMN IF EXISTS alias;
