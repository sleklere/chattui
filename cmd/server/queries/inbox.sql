-- name: SaveInboxRoomEvent :exec
INSERT INTO inbox_events (user_id, kind, room_id, source_user_id)
    SELECT rm.user_id, @kind, @room_id, @source_user_id
    FROM room_members rm
    WHERE rm.room_id = @room_id
    AND rm.user_id != @source_user_id;

-- name: UpsertInboxConversationCursor :exec
INSERT INTO inbox_conversations (user_id, ref_room_id, ref_conversation_id)
    VALUES (@user_id, @ref_room_id, @ref_conversation_id)
    ON CONFLICT DO NOTHING;

-- name: UpdateInboxCursorOnRoomMessage :many
WITH updated AS (
    UPDATE inbox_conversations
    SET last_message_body      = @body,
        last_message_at        = NOW(),
        last_message_sender_id = @sender_id,
        unread_count           = unread_count + 1
    WHERE ref_room_id = @room_id
      AND user_id != @sender_id
    RETURNING *
)
SELECT
    'conversation'::text             AS entry_type,
    ''::text                         AS kind,
    0::bigint                        AS source_user_id,
    ''::text                         AS source_username,
    updated.ref_room_id,
    updated.ref_conversation_id,
    updated.unread_count::bigint    AS unread_count,
    updated.last_message_body,
    updated.last_message_sender_id,
    r.name                           AS room_name,
    COALESCE(peer.id, 0)             AS peer_id,
    COALESCE(peer.username, '')      AS peer_username,
    updated.last_message_at          AS created_at,
    updated.user_id
FROM updated
LEFT JOIN rooms r ON r.id = updated.ref_room_id
LEFT JOIN conversations c ON c.id = updated.ref_conversation_id
LEFT JOIN users peer ON peer.id = CASE
    WHEN c.user_a = updated.user_id THEN c.user_b
    ELSE c.user_a
END;

-- name: UpdateInboxCursorOnDMMessage :many
-- Filters by user_id with equality (instead of != sender_id) so the partial index
-- inbox_conv_user_conv_uniq (user_id, ref_conversation_id) is used directly.
-- Filtering with != on the leading column forces a full index scan.
WITH updated AS (
    UPDATE inbox_conversations
    SET last_message_body      = @body,
        last_message_at        = NOW(),
        last_message_sender_id = @sender_id,
        unread_count           = unread_count + 1
    WHERE user_id = @recipient_id
      AND ref_conversation_id = @conversation_id
    RETURNING *
)
SELECT
    'conversation'::text   AS entry_type,
    ''::text               AS kind,
    0::bigint              AS source_user_id,
    ''::text               AS source_username,
    ref_room_id,
    ref_conversation_id,
    unread_count::bigint,
    last_message_body,
    last_message_sender_id,
    r.name                 AS room_name,
    COALESCE(peer.id, 0)   AS peer_id,
    COALESCE(peer.username, '') AS peer_username,
    last_message_at     AS created_at,
    updated.user_id
FROM updated
LEFT JOIN rooms r ON r.id = ref_room_id
LEFT JOIN conversations c ON c.id = ref_conversation_id
LEFT JOIN users peer ON peer.id = CASE WHEN c.user_a = updated.user_id::bigint THEN c.user_b ELSE c.user_a END;

-- name: ListInboxFeed :many
SELECT
    'event'::text          AS entry_type,
    ie.kind,
    ie.source_user_id,
    u.username             AS source_username,
    ie.room_id             AS ref_room_id,
    NULL::bigint           AS ref_conversation_id,
    0::bigint              AS unread_count,
    NULL::text             AS last_message_body,
    NULL::bigint           AS last_message_sender_id,
    r.name                 AS room_name,
    0::bigint              AS peer_id,
    ''::text               AS peer_username,
    ie.created_at
FROM inbox_events ie
JOIN users u ON u.id = ie.source_user_id
LEFT JOIN rooms r ON r.id = ie.room_id
WHERE ie.user_id = @user_id
UNION ALL
SELECT
    'conversation'::text   AS entry_type,
    ''::text               AS kind,
    0::bigint              AS source_user_id,
    ''::text               AS source_username,
    ic.ref_room_id,
    ic.ref_conversation_id,
    ic.unread_count::bigint,
    ic.last_message_body,
    ic.last_message_sender_id,
    r.name                 AS room_name,
    COALESCE(peer.id, 0)   AS peer_id,
    COALESCE(peer.username, '') AS peer_username,
    ic.last_message_at     AS created_at
FROM inbox_conversations ic
LEFT JOIN rooms r ON r.id = ic.ref_room_id
LEFT JOIN conversations c ON c.id = ic.ref_conversation_id
LEFT JOIN users peer ON peer.id = CASE WHEN c.user_a = @user_id::bigint THEN c.user_b ELSE c.user_a END
WHERE ic.user_id = @user_id
  AND ic.last_message_at IS NOT NULL
ORDER BY created_at DESC
LIMIT @lim;
