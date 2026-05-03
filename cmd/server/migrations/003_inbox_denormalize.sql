-- +goose Up
-- +goose StatementBegin
ALTER TABLE inbox_conversations
    ADD COLUMN last_message_body       TEXT,
    ADD COLUMN last_message_at         TIMESTAMPTZ,
    ADD COLUMN last_message_sender_id  BIGINT,
    ADD COLUMN unread_count            INT NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX inbox_conv_user_last_msg_idx
    ON inbox_conversations (user_id, last_message_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS inbox_conv_user_last_msg_idx;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE inbox_conversations
    DROP COLUMN IF EXISTS last_message_body,
    DROP COLUMN IF EXISTS last_message_at,
    DROP COLUMN IF EXISTS last_message_sender_id,
    DROP COLUMN IF EXISTS unread_count;
-- +goose StatementEnd
