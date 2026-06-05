-- +goose Up
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN public_key bytea;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN IF EXISTS public_key;
-- +goose StatementEnd
