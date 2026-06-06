-- +goose Up
CREATE TABLE users (
    id                  BIGSERIAL   PRIMARY KEY,
    name                TEXT        NOT NULL DEFAULT '',
    profile_picture     TEXT        NOT NULL DEFAULT '',
    username            TEXT        NOT NULL,
    password            TEXT        NOT NULL DEFAULT '',
    email               TEXT        NOT NULL,
    phone_number        TEXT        NOT NULL DEFAULT '',
    is_suspended        BOOLEAN     NOT NULL DEFAULT FALSE,
    last_password_reset TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX username_unique ON users (username);
CREATE UNIQUE INDEX email_unique ON users (email);

-- +goose Down
DROP TABLE IF EXISTS users;
