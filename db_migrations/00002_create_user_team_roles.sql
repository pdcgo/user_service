-- +goose Up
CREATE TABLE user_team_roles (
    id         BIGSERIAL   PRIMARY KEY,
    team_id    BIGINT      NOT NULL,
    user_id    BIGINT      NOT NULL,
    role       BIGINT      NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX team_user_unique ON user_team_roles (team_id, user_id);

-- +goose Down
DROP TABLE IF EXISTS user_team_roles;
