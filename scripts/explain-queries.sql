-- EXPLAIN (ANALYZE, BUFFERS) sobre las queries reales que usa la app
-- (cmd/server/queries/*.sql). Corré con DB seedeada:
--   psql "$DB_URL" -f scripts/explain-queries.sql
--
-- Targets para stress real:
--   user_id         = 49795  (sleklere — 1.7k msgs, 2 rooms, 7 convs, 9 inbox)
--   room_id         = 1      (~300k msgs — el más pesado del seed)
--   conversation_id = 1      (~34k msgs — la más pesada del seed)
--
-- Lo que mirar:
--   - "Index Scan" / "Index Only Scan" (bueno) vs "Seq Scan" sobre tablas grandes (malo).
--   - "Buffers: shared hit/read" — read alto = mucho I/O, hit alto = caliente en cache.
--   - "Execution Time" para el costo total.
--
-- Las UPDATE las envuelvo en BEGIN/ROLLBACK para no mutar la DB.

\timing on

-- =========================================================================
-- 1. ListInboxFeed (inbox.sql) — query más compleja, hot path al abrir inbox.
-- =========================================================================
\echo '=== 1. ListInboxFeed (user 49795, limit 20) ==='
EXPLAIN (ANALYZE, BUFFERS)
SELECT 'event'::text AS entry_type, ie.kind, ie.source_user_id, u.username AS source_username,
       ie.room_id AS ref_room_id, NULL::bigint AS ref_conversation_id,
       0::bigint AS unread_count, NULL::text AS last_message_body,
       NULL::bigint AS last_message_sender_id, r.name AS room_name,
       0::bigint AS peer_id, ''::text AS peer_username, ie.created_at
FROM inbox_events ie
JOIN users u ON u.id = ie.source_user_id
LEFT JOIN rooms r ON r.id = ie.room_id
WHERE ie.user_id = 49795
UNION ALL
SELECT 'conversation'::text, ''::text, 0::bigint, ''::text,
       ic.ref_room_id, ic.ref_conversation_id, ic.unread_count::bigint,
       ic.last_message_body, ic.last_message_sender_id, r.name,
       COALESCE(peer.id, 0), COALESCE(peer.username, ''), ic.last_message_at
FROM inbox_conversations ic
LEFT JOIN rooms r ON r.id = ic.ref_room_id
LEFT JOIN conversations c ON c.id = ic.ref_conversation_id
LEFT JOIN users peer ON peer.id = CASE WHEN c.user_a = 49795::bigint THEN c.user_b ELSE c.user_a END
WHERE ic.user_id = 49795
  AND ic.last_message_at IS NOT NULL
ORDER BY created_at DESC
LIMIT 20;

-- =========================================================================
-- 2. UpdateInboxCursorOnRoomMessage (inbox.sql) — fan-out al recibir msg en room.
--    El UPDATE toca todas las rows con ref_room_id = X. ¿Usa índice?
--    Indices candidatos: inbox_conv_user_room_uniq (user_id, ref_room_id) WHERE ref_room_id IS NOT NULL.
--    Filtro es solo por ref_room_id: el planner puede usar índice si arranca con room_id, pero el primary col es user_id.
-- =========================================================================
\echo ''
\echo '=== 2. UpdateInboxCursorOnRoomMessage (room 1) ==='
BEGIN;
EXPLAIN (ANALYZE, BUFFERS)
UPDATE inbox_conversations
SET last_message_body      = 'test',
    last_message_at        = NOW(),
    last_message_sender_id = 49795,
    unread_count           = unread_count + 1
WHERE ref_room_id = 1
  AND user_id != 49795;
ROLLBACK;

-- =========================================================================
-- 3. UpdateInboxCursorOnDMMessage (inbox.sql) — fan-out al recibir DM.
--    Filtro por ref_conversation_id, sólo 2 rows afectadas (uno por participante).
--    Índice: inbox_conv_user_conv_uniq (user_id, ref_conversation_id).
-- =========================================================================
\echo ''
\echo '=== 3. UpdateInboxCursorOnDMMessage (conv 1) ==='
BEGIN;
EXPLAIN (ANALYZE, BUFFERS)
UPDATE inbox_conversations
SET last_message_body      = 'test',
    last_message_at        = NOW(),
    last_message_sender_id = 49795,
    unread_count           = unread_count + 1
WHERE ref_conversation_id = 1
  AND user_id != 49795;
ROLLBACK;

-- =========================================================================
-- 4. ListMessagesByRoom (messages.sql) — historial del room más pesado.
--    Index esperado: idx_messages_room_created_at (room_id, created_at DESC).
-- =========================================================================
\echo ''
\echo '=== 4. ListMessagesByRoom (room 1, limit 50) ==='
EXPLAIN (ANALYZE, BUFFERS)
SELECT m.id, m.room_id, m.conversation_id, m.sender_id, u.username AS sender_username,
       m.body, m.created_at
FROM messages m
JOIN users u ON u.id = m.sender_id
WHERE m.room_id = 1
ORDER BY m.created_at DESC
LIMIT 50;

-- =========================================================================
-- 5. ListMessagesByConversation (messages.sql) — historial DM más pesado.
--    Index esperado: idx_messages_conv_created_at (conversation_id, created_at DESC).
-- =========================================================================
\echo ''
\echo '=== 5. ListMessagesByConversation (conv 1, limit 50) ==='
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, room_id, conversation_id, sender_id, body, created_at
FROM messages
WHERE conversation_id = 1
ORDER BY created_at DESC
LIMIT 50;

-- =========================================================================
-- 6. ListConversationsByUser (conversations.sql) — sidebar DMs.
--    OJO: filtro user_a = X OR user_b = X. No hay índice en user_a/user_b sueltos
--    (sólo UNIQUE compuesto user_a, user_b — sirve sólo para user_a). Posible Seq Scan.
-- =========================================================================
\echo ''
\echo '=== 6. ListConversationsByUser (user 49795, limit 50) ==='
EXPLAIN (ANALYZE, BUFFERS)
SELECT conversations.id, peer.id as peer_id, peer.username AS peer_username
FROM conversations
JOIN users peer ON (CASE WHEN user_a = 49795 THEN user_b ELSE user_a END) = peer.id
WHERE user_a = 49795 OR user_b = 49795
ORDER BY conversations.id DESC
LIMIT 50;

-- =========================================================================
-- 7. GetRoomsForUser (room_members.sql) — sidebar de rooms del user.
-- =========================================================================
\echo ''
\echo '=== 7. GetRoomsForUser (user 49795) ==='
EXPLAIN (ANALYZE, BUFFERS)
SELECT r.id, r.name, r.slug, r.created_at
FROM rooms r
JOIN room_members rm ON rm.room_id = r.id
WHERE rm.user_id = 49795
ORDER BY r.created_at DESC;

-- =========================================================================
-- 8. ListRoomMembers (room_members.sql) — al entrar a un room.
--    PK (room_id, user_id) cubre filtro; ORDER BY joined_at requiere sort.
-- =========================================================================
\echo ''
\echo '=== 8. ListRoomMembers (room 1) ==='
EXPLAIN (ANALYZE, BUFFERS)
SELECT u.id, u.username
FROM room_members rm
JOIN users u ON u.id = rm.user_id
WHERE rm.room_id = 1
ORDER BY rm.joined_at;

-- =========================================================================
-- 9. IsMember (room_members.sql) — chequeo de membresía. Hot path en mensajes.
-- =========================================================================
\echo ''
\echo '=== 9. IsMember (room 1, user 49795) ==='
EXPLAIN (ANALYZE, BUFFERS)
SELECT EXISTS (SELECT 1 FROM room_members WHERE room_id = 1 AND user_id = 49795) AS is_member;

-- =========================================================================
-- 10. GetUserByUsername (users.sql) — login.
-- =========================================================================
\echo ''
\echo '=== 10. GetUserByUsername (sleklere) ==='
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, username, password, created_at FROM users WHERE username = 'sleklere';

-- =========================================================================
-- 11. ListRooms (rooms.sql) — listado plano sin paginación.
-- =========================================================================
\echo ''
\echo '=== 11. ListRooms (sin paginación, 1000 rows) ==='
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, name, slug, created_at FROM rooms ORDER BY created_at DESC;
