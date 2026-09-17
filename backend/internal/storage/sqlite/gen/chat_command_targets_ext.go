package gen

import (
	"context"
	"time"
)

// UpsertChatCommandTarget persists the policy that became current in the same
// transaction as its sessions.controller_generation fence. Kept here because
// sqlc's SQLite parser truncates the ON CONFLICT update expression.
func (q *Queries) UpsertChatCommandTarget(ctx context.Context, sessionID, generation, revision, capability string, updatedAt time.Time) error {
	_, err := q.db.ExecContext(ctx, `INSERT INTO chat_command_targets(session_id,controller_generation,expected_revision,capability_fingerprint,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(session_id) DO UPDATE SET controller_generation=excluded.controller_generation,expected_revision=excluded.expected_revision,capability_fingerprint=excluded.capability_fingerprint,updated_at=excluded.updated_at`, sessionID, generation, revision, capability, updatedAt)
	return err
}

func (q *Queries) DeleteChatCommandTarget(ctx context.Context, sessionID string) error {
	_, err := q.db.ExecContext(ctx, `DELETE FROM chat_command_targets WHERE session_id=?`, sessionID)
	return err
}
