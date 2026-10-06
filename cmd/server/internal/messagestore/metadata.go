package messagestore

import (
	"context"
	"fmt"

	"github.com/sleklere/chattui/cmd/server/internal/errs"
	dbstore "github.com/sleklere/chattui/cmd/server/internal/store"
)

// PostgresMetadata keeps users and chat identities authoritative in PostgreSQL.
type PostgresMetadata struct{ db dbstore.DBTX }

// NewPostgresMetadata uses the existing SQL connection for reference validation.
func NewPostgresMetadata(db dbstore.DBTX) *PostgresMetadata {
	return &PostgresMetadata{db: db}
}

// ValidateMessage checks sender and chat existence, not cross-store atomicity.
func (m *PostgresMetadata) ValidateMessage(ctx context.Context, p dbstore.CreateMessageParams) error {
	var valid bool
	err := m.db.QueryRow(ctx, `SELECT
 EXISTS (SELECT 1 FROM users WHERE id = $1) AND
 (EXISTS (SELECT 1 FROM rooms WHERE id = $2) OR
  EXISTS (SELECT 1 FROM conversations WHERE id = $3))`, p.SenderID, p.RoomID, p.ConversationID).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("message sender or chat does not exist: %w", errs.ErrNotFound)
	}
	return nil
}

// Username resolves the current SQL username rather than a message-time snapshot.
func (m *PostgresMetadata) Username(ctx context.Context, userID int64) (string, error) {
	user, err := dbstore.New(m.db).GetUserByID(ctx, userID)
	return user.Username, err
}
