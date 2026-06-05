-- +goose Up
-- +goose StatementBegin
ALTER TABLE messages
    ADD COLUMN search_tsv tsvector
    GENERATED ALWAYS AS (to_tsvector('english', body)) STORED;

CREATE INDEX messages_search_idx ON messages USING GIN (search_tsv);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS messages_search_idx;
ALTER TABLE messages DROP COLUMN IF EXISTS search_tsv;
-- +goose StatementEnd
