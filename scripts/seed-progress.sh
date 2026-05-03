#!/usr/bin/env bash
# Mostra los counts de las tablas seedeadas en vivo.
# Usalo en otra terminal mientras corre seed.sql.
#
# Uso: ./scripts/seed-progress.sh

set -euo pipefail

cd "$(dirname "$0")/.."
set -a; source .env; set +a

watch -n 2 -t "psql \"$DB_URL\" -At -c \"
SELECT format('%-22s %s', tbl, to_char(c, 'FM999,999,999')) FROM (
    SELECT 'users'               AS tbl, count(*) AS c, 1 AS o FROM users
    UNION ALL SELECT 'rooms',                count(*), 2 FROM rooms
    UNION ALL SELECT 'room_members',         count(*), 3 FROM room_members
    UNION ALL SELECT 'conversations',        count(*), 4 FROM conversations
    UNION ALL SELECT 'messages',             count(*), 5 FROM messages
    UNION ALL SELECT 'inbox_conversations',  count(*), 6 FROM inbox_conversations
    UNION ALL SELECT 'inbox_events',         count(*), 7 FROM inbox_events
) t ORDER BY o;\""
