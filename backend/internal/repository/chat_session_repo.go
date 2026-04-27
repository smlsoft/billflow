package repository

import (
	"database/sql"
	"encoding/json"
	"time"
)

// ChatSessionRecord is the raw DB row for a LINE chat session.
type ChatSessionRecord struct {
	LineUserID   string
	History      json.RawMessage
	PendingOrder json.RawMessage // may be nil
	LastActive   time.Time
}

type ChatSessionRepo struct {
	db *sql.DB
}

func NewChatSessionRepo(db *sql.DB) *ChatSessionRepo {
	return &ChatSessionRepo{db: db}
}

// Get retrieves the session for a LINE user. Returns nil if not found.
func (r *ChatSessionRepo) Get(lineUserID string) (*ChatSessionRecord, error) {
	rec := &ChatSessionRecord{}
	var pendingRaw []byte
	err := r.db.QueryRow(
		`SELECT line_user_id, history, pending_order, last_active
		 FROM chat_sessions WHERE line_user_id = $1`,
		lineUserID,
	).Scan(&rec.LineUserID, &rec.History, &pendingRaw, &rec.LastActive)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rec.PendingOrder = pendingRaw
	return rec, nil
}

// Upsert inserts or updates the session record.
func (r *ChatSessionRepo) Upsert(rec *ChatSessionRecord) error {
	_, err := r.db.Exec(
		`INSERT INTO chat_sessions (line_user_id, history, pending_order, last_active)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (line_user_id) DO UPDATE SET
		   history       = EXCLUDED.history,
		   pending_order = EXCLUDED.pending_order,
		   last_active   = EXCLUDED.last_active`,
		rec.LineUserID, rec.History, rec.PendingOrder, rec.LastActive,
	)
	return err
}

// Delete removes the session for a LINE user.
func (r *ChatSessionRepo) Delete(lineUserID string) error {
	_, err := r.db.Exec(`DELETE FROM chat_sessions WHERE line_user_id = $1`, lineUserID)
	return err
}

// PruneIdle deletes sessions that have been inactive for more than maxAge.
func (r *ChatSessionRepo) PruneIdle(maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)
	_, err := r.db.Exec(`DELETE FROM chat_sessions WHERE last_active < $1`, cutoff)
	return err
}
