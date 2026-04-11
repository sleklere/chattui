-- name: GetOrCreateConversation :one
INSERT INTO conversations (user_a, user_b)
VALUES (LEAST($1, $2), GREATEST($1, $2))
ON CONFLICT (user_a, user_b) DO UPDATE SET user_a = EXCLUDED.user_a
RETURNING id, user_a, user_b;

-- name: ListConversationsByUser :many
SELECT conversations.id, peer.id as peer_id, peer.username AS peer_username
FROM conversations
JOIN users peer ON (CASE WHEN user_a = @user_id THEN user_b ELSE user_a END) = peer.id
WHERE user_a = @user_id OR user_b = @user_id
ORDER BY conversations.id DESC LIMIT @lim;
