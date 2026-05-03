-- Seed de datos masivos para chattui.
--
-- Uso:
--   psql "$DB_URL" -f scripts/seed.sql
--
-- Requiere DB con migraciones aplicadas y tablas vacías (no upsert: usa INSERT directo).
-- Ajustá las constantes de abajo para subir/bajar volumen.
--
-- Password de todos los users seedeados: "test1234" (bcrypt cost 10).
-- Cualquiera de los users user_1 .. user_<N_USERS> puede loguearse con esa pass.

\timing on
\set ON_ERROR_STOP on

-- =========================================================================
-- Constantes
-- =========================================================================
\set n_users         100000
\set n_rooms         1000
\set n_room_members  60000
\set n_conversations 200000
\set n_room_msgs     3000000
\set n_conv_msgs     2000000
\set n_inbox_events  50000

-- Hash bcrypt fijo de "test1234" (cost 10). Generado fuera de banda con
-- golang.org/x/crypto/bcrypt para evitar dependencias en SQL.
\set bcrypt_hash '''$2a$10$ZoaS5SSn2YjgzRxKFJtjtet4Qvzpzt4cAw6kBwiUaGEriaAUSBKJy'''

-- =========================================================================
-- 1. users
-- =========================================================================
\echo '>>> users'
INSERT INTO users (username, password, created_at)
SELECT
    'user_' || g,
    :bcrypt_hash,
    now() - (random() * interval '365 days')
FROM generate_series(1, :n_users) g;

-- =========================================================================
-- 2. rooms
-- =========================================================================
\echo '>>> rooms'
INSERT INTO rooms (name, slug, created_at)
SELECT
    'Room ' || g,
    'room-' || g,
    now() - (random() * interval '365 days')
FROM generate_series(1, :n_rooms) g;

-- =========================================================================
-- 3. room_members
-- Power-law: rooms con id bajo concentran más miembros (random()^2).
-- Sobre-muestreamos y ON CONFLICT descarta duplicados (PK = room_id, user_id).
-- =========================================================================
\echo '>>> room_members'
INSERT INTO room_members (user_id, room_id, joined_at)
SELECT
    1 + floor(:n_users * random())::bigint,
    1 + floor(:n_rooms * (random()^2))::bigint,
    now() - (random() * interval '365 days')
FROM generate_series(1, :n_room_members)
ON CONFLICT DO NOTHING;

-- =========================================================================
-- 4. conversations (1-1, user_a < user_b)
-- Sobre-muestreamos para compensar colisiones (raras con 100k users).
-- =========================================================================
\echo '>>> conversations'
INSERT INTO conversations (user_a, user_b)
SELECT user_a, user_b FROM (
    SELECT DISTINCT
        LEAST(a, b)    AS user_a,
        GREATEST(a, b) AS user_b
    FROM (
        SELECT
            1 + floor(:n_users * random())::bigint AS a,
            1 + floor(:n_users * random())::bigint AS b
        FROM generate_series(1, (:n_conversations * 1.1)::int)
    ) t
    WHERE a <> b
) p
LIMIT :n_conversations;

-- =========================================================================
-- 5a. messages en rooms
-- Power-law sobre rooms (random()^3). Sólo se eligen rooms con miembros.
-- sender_id se samplea de los miembros del room (array indexado).
-- =========================================================================
\echo '>>> room_member_arr (cache)'
CREATE TEMP TABLE rooms_with_members AS
SELECT
    row_number() OVER (ORDER BY room_id)        AS rn,
    room_id,
    array_agg(user_id)                          AS members,
    count(*)::int                               AS n_members
FROM room_members
GROUP BY room_id;

CREATE INDEX ON rooms_with_members (rn);
ANALYZE rooms_with_members;

\echo '>>> messages (room)'
WITH n AS (SELECT count(*)::int AS c FROM rooms_with_members),
picks AS (
    SELECT
        1 + floor((SELECT c FROM n) * (random()^3))::int AS rn,
        now() - (random() * interval '365 days')        AS ts,
        random()                                         AS sender_pick
    FROM generate_series(1, :n_room_msgs)
)
INSERT INTO messages (room_id, sender_id, body, created_at)
SELECT
    rwm.room_id,
    rwm.members[1 + floor(rwm.n_members * p.sender_pick)::int],
    'msg-room-' || md5(random()::text),
    p.ts
FROM picks p
JOIN rooms_with_members rwm ON rwm.rn = p.rn;

-- =========================================================================
-- 5b. messages en conversations
-- Power-law sobre conversations. sender = user_a o user_b al azar.
-- =========================================================================
\echo '>>> convs_indexed (cache)'
CREATE TEMP TABLE convs_indexed AS
SELECT
    row_number() OVER (ORDER BY id) AS rn,
    id, user_a, user_b
FROM conversations;

CREATE INDEX ON convs_indexed (rn);
ANALYZE convs_indexed;

\echo '>>> messages (conv)'
WITH n AS (SELECT count(*)::int AS c FROM convs_indexed),
picks AS (
    SELECT
        1 + floor((SELECT c FROM n) * (random()^3))::int AS rn,
        now() - (random() * interval '365 days')        AS ts,
        random()                                         AS sender_pick
    FROM generate_series(1, :n_conv_msgs)
)
INSERT INTO messages (conversation_id, sender_id, body, created_at)
SELECT
    ci.id,
    CASE WHEN p.sender_pick < 0.5 THEN ci.user_a ELSE ci.user_b END,
    'msg-conv-' || md5(random()::text),
    p.ts
FROM picks p
JOIN convs_indexed ci ON ci.rn = p.rn;

-- =========================================================================
-- 6a. inbox_conversations (room-based)
-- Una row por (user, room) donde el user es miembro y el room tiene mensajes.
-- last_message_* viene del último msg del room (compartido).
-- ~70% leídos (unread=0, last_read = último msg), ~30% con unread aleatorio.
-- =========================================================================
\echo '>>> inbox_conversations (room)'
INSERT INTO inbox_conversations (
    user_id, ref_room_id, last_read_message_id, created_at,
    last_message_body, last_message_at, last_message_sender_id, unread_count
)
SELECT
    rm.user_id,
    rm.room_id,
    CASE WHEN rnd.r < 0.7 THEN lm.id ELSE NULL END,
    rm.joined_at,
    lm.body,
    lm.created_at,
    lm.sender_id,
    CASE WHEN rnd.r < 0.7 THEN 0 ELSE 1 + floor(random() * 50)::int END
FROM room_members rm
JOIN LATERAL (
    SELECT id, body, created_at, sender_id
    FROM messages
    WHERE room_id = rm.room_id
    ORDER BY created_at DESC
    LIMIT 1
) lm ON true
CROSS JOIN LATERAL (SELECT random() AS r) rnd;

-- =========================================================================
-- 6b. inbox_conversations (conv-based) — dos rows por conv (una por participante).
-- =========================================================================
\echo '>>> inbox_conversations (conv)'
INSERT INTO inbox_conversations (
    user_id, ref_conversation_id, last_read_message_id, created_at,
    last_message_body, last_message_at, last_message_sender_id, unread_count
)
SELECT
    u.user_id,
    c.id,
    CASE WHEN rnd.r < 0.7 THEN lm.id ELSE NULL END,
    lm.created_at - interval '1 day',
    lm.body,
    lm.created_at,
    lm.sender_id,
    CASE WHEN rnd.r < 0.7 THEN 0 ELSE 1 + floor(random() * 30)::int END
FROM conversations c
JOIN LATERAL (
    SELECT id, body, created_at, sender_id
    FROM messages
    WHERE conversation_id = c.id
    ORDER BY created_at DESC
    LIMIT 1
) lm ON true
CROSS JOIN LATERAL (
    SELECT c.user_a AS user_id UNION ALL SELECT c.user_b
) u
CROSS JOIN LATERAL (SELECT random() AS r) rnd;

-- =========================================================================
-- 7. inbox_events
-- =========================================================================
\echo '>>> inbox_events'
INSERT INTO inbox_events (user_id, kind, room_id, source_user_id, created_at, read_at)
SELECT
    1 + floor(:n_users * random())::bigint,
    (ARRAY['room_join','room_leave','friend_request','room_invite'])[1 + floor(4 * random())::int],
    CASE WHEN random() < 0.6 THEN 1 + floor(:n_rooms * random())::bigint ELSE NULL END,
    1 + floor(:n_users * random())::bigint,
    now() - (random() * interval '365 days'),
    CASE WHEN random() < 0.5 THEN now() - (random() * interval '180 days') ELSE NULL END
FROM generate_series(1, :n_inbox_events);

-- =========================================================================
-- ANALYZE final para que el planner tenga stats actualizadas.
-- =========================================================================
\echo '>>> ANALYZE'
ANALYZE users;
ANALYZE rooms;
ANALYZE room_members;
ANALYZE conversations;
ANALYZE messages;
ANALYZE inbox_conversations;
ANALYZE inbox_events;

-- =========================================================================
-- Resumen
-- =========================================================================
\echo ''
\echo '=== counts ==='
SELECT 'users'                AS tbl, count(*) FROM users
UNION ALL SELECT 'rooms',                count(*) FROM rooms
UNION ALL SELECT 'room_members',         count(*) FROM room_members
UNION ALL SELECT 'conversations',        count(*) FROM conversations
UNION ALL SELECT 'messages',             count(*) FROM messages
UNION ALL SELECT 'inbox_conversations',  count(*) FROM inbox_conversations
UNION ALL SELECT 'inbox_events',         count(*) FROM inbox_events;
