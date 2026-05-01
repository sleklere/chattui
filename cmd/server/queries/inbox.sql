-- name: SaveRoomEvent :exec
INSERT INTO inbox_events (user_id, kind, room_id, source_user_id)
    SELECT rm.user_id, @kind, @room_id, @source_user_id
    FROM room_members rm
    WHERE rm.room_id = @room_id
    AND rm.user_id != @source_user_id;
