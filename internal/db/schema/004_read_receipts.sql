-- +goose Up
-- +goose StatementBegin
ALTER TABLE room_members
    ADD COLUMN last_read_message_id uuid REFERENCES messages (id) ON DELETE SET NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE room_members DROP COLUMN IF EXISTS last_read_message_id;
-- +goose StatementEnd
