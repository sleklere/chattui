-- +goose Up

-- +goose StatementBegin
CREATE TABLE inbox_conversations (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    ref_room_id BIGINT,
    ref_conversation_id BIGINT,
    last_read_message_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT room_conv_refs CHECK (
        (ref_room_id IS NOT NULL AND ref_conversation_id IS NULL)
        OR
        (ref_room_id IS NULL AND ref_conversation_id IS NOT NULL)
    )
);

CREATE TABLE inbox_events (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    kind TEXT NOT NULL,
    room_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    source_user_id BIGINT NOT NULL,
    read_at TIMESTAMPTZ,

    CONSTRAINT kind_enum CHECK (
        kind IN ('room_join', 'room_leave', 'friend_request', 'room_invite')
    )
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE UNIQUE INDEX inbox_conv_user_room_uniq
    ON inbox_conversations (user_id, ref_room_id)
    WHERE ref_room_id IS NOT NULL;

CREATE UNIQUE INDEX inbox_conv_user_conv_uniq
    ON inbox_conversations (user_id, ref_conversation_id)
    WHERE ref_conversation_id IS NOT NULL;

CREATE INDEX inbox_conv_user_created_idx
    ON inbox_conversations (user_id, created_at DESC);

CREATE INDEX inbox_events_user_created_idx
    ON inbox_events (user_id, created_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS inbox_events_user_created_idx;
DROP INDEX IF EXISTS inbox_conv_user_created_idx;
DROP INDEX IF EXISTS inbox_conv_user_conv_uniq;
DROP INDEX IF EXISTS inbox_conv_user_room_uniq;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS inbox_events;
DROP TABLE IF EXISTS inbox_conversations;
-- +goose StatementEnd
