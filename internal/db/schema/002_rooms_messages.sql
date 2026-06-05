-- +goose Up
-- +goose StatementBegin
CREATE TABLE rooms (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text        NOT NULL DEFAULT '',
    kind       text        NOT NULL DEFAULT 'room',
    dm_key     text        UNIQUE,
    created_by uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE room_members (
    room_id   uuid        NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    user_id   uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    joined_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (room_id, user_id)
);

CREATE TABLE messages (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id    uuid        NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    body       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX messages_room_created_idx ON messages (room_id, created_at DESC);

INSERT INTO rooms (id, name, kind)
VALUES ('00000000-0000-0000-0000-000000000001', 'general', 'room');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS room_members;
DROP TABLE IF EXISTS rooms;
-- +goose StatementEnd
