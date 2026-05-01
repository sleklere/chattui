-- name: SaveRoomEvent :exec
INSERT INTO inbox_events (user_id, kind, room_id, source_user_id)
    SELECT rm.user_id, @kind, @room_id, @source_user_id
    FROM room_members rm
    WHERE rm.room_id = @room_id
    AND rm.user_id != @source_user_id;

-- name: FindEventsByUserID :many
SELECT e.id, e.kind, e.room_id, e.created_at, source_user_id, u.username AS source_username, e.read_at
FROM inbox_events e
JOIN users u ON e.source_user_id = u.id
WHERE e.user_id = $1
ORDER BY e.created_at DESC LIMIT $2;
