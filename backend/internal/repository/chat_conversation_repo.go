package repository

import (
	"database/sql"
	"fmt"

	"billflow/internal/models"
)

// ChatConversationRepo manages chat_conversations (one row per LINE user).
type ChatConversationRepo struct {
	db *sql.DB
}

func NewChatConversationRepo(db *sql.DB) *ChatConversationRepo {
	return &ChatConversationRepo{db: db}
}

const chatConvCols = `
  line_user_id, line_oa_id, display_name, picture_url,
  last_message_at, last_inbound_at, last_admin_reply_at,
  unread_admin_count, created_at
`

func scanChatConversation(s interface{ Scan(...any) error }) (*models.ChatConversation, error) {
	c := &models.ChatConversation{}
	var lineOAID sql.NullString
	var lastInbound, lastAdminReply sql.NullTime
	err := s.Scan(
		&c.LineUserID, &lineOAID, &c.DisplayName, &c.PictureURL,
		&c.LastMessageAt, &lastInbound, &lastAdminReply,
		&c.UnreadAdminCount, &c.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if lineOAID.Valid {
		v := lineOAID.String
		c.LineOAID = &v
	}
	if lastInbound.Valid {
		t := lastInbound.Time
		c.LastInboundAt = &t
	}
	if lastAdminReply.Valid {
		t := lastAdminReply.Time
		c.LastAdminReplyAt = &t
	}
	return c, nil
}

// Upsert is the legacy single-OA entry point — kept for backward compat.
// New callers should use UpsertWithOA so the row gets a line_oa_id.
func (r *ChatConversationRepo) Upsert(lineUserID, displayName, pictureURL string) (*models.ChatConversation, bool, error) {
	return r.UpsertWithOA(lineUserID, displayName, pictureURL, nil)
}

// UpsertWithOA creates or updates a conversation row, optionally setting the
// line_oa_id. The OA id is set ONLY on insert; subsequent calls don't move a
// conversation between OAs (LINE userIDs are scoped per OA anyway, so a row
// can only belong to one OA for its lifetime).
//
// Returns (conversation, isNew). isNew=true on first insert — caller fetches
// the profile from LINE and sends the greeting in that case.
func (r *ChatConversationRepo) UpsertWithOA(
	lineUserID, displayName, pictureURL string, oaID *string,
) (*models.ChatConversation, bool, error) {
	row := r.db.QueryRow(
		`INSERT INTO chat_conversations (line_user_id, line_oa_id, display_name, picture_url)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (line_user_id) DO UPDATE SET
		   display_name = CASE WHEN EXCLUDED.display_name <> '' THEN EXCLUDED.display_name ELSE chat_conversations.display_name END,
		   picture_url  = CASE WHEN EXCLUDED.picture_url  <> '' THEN EXCLUDED.picture_url  ELSE chat_conversations.picture_url  END,
		   -- backfill line_oa_id only if it was previously NULL (don't move OAs)
		   line_oa_id   = COALESCE(chat_conversations.line_oa_id, EXCLUDED.line_oa_id)
		 RETURNING `+chatConvCols+`, (xmax = 0) AS is_new`,
		lineUserID, oaID, displayName, pictureURL,
	)
	c := &models.ChatConversation{}
	var lineOAID sql.NullString
	var lastInbound, lastAdminReply sql.NullTime
	var isNew bool
	if err := row.Scan(
		&c.LineUserID, &lineOAID, &c.DisplayName, &c.PictureURL,
		&c.LastMessageAt, &lastInbound, &lastAdminReply,
		&c.UnreadAdminCount, &c.CreatedAt, &isNew,
	); err != nil {
		return nil, false, fmt.Errorf("upsert chat_conversation: %w", err)
	}
	if lineOAID.Valid {
		v := lineOAID.String
		c.LineOAID = &v
	}
	if lastInbound.Valid {
		t := lastInbound.Time
		c.LastInboundAt = &t
	}
	if lastAdminReply.Valid {
		t := lastAdminReply.Time
		c.LastAdminReplyAt = &t
	}
	return c, isNew, nil
}

// Get returns a single conversation, or nil (no error) when not found.
func (r *ChatConversationRepo) Get(lineUserID string) (*models.ChatConversation, error) {
	row := r.db.QueryRow(
		`SELECT `+chatConvCols+` FROM chat_conversations WHERE line_user_id = $1`,
		lineUserID,
	)
	c, err := scanChatConversation(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get chat_conversation: %w", err)
	}
	return c, nil
}

// ConversationListRow is a list-view row that joins line_oa_accounts.name so
// the inbox can render an OA badge per conversation without an N+1 query.
type ConversationListRow struct {
	models.ChatConversation
	LineOAName string `json:"line_oa_name,omitempty"`
}

// List returns conversations ordered by last_message_at DESC.
// unreadOnly filters to rows with unread_admin_count > 0.
// Each row includes the OA's display name (line_oa_name) for badge rendering.
func (r *ChatConversationRepo) List(limit, offset int, unreadOnly bool) ([]*ConversationListRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT
	        cc.line_user_id, cc.line_oa_id, cc.display_name, cc.picture_url,
	        cc.last_message_at, cc.last_inbound_at, cc.last_admin_reply_at,
	        cc.unread_admin_count, cc.created_at,
	        COALESCE(oa.name, '') AS line_oa_name
	      FROM chat_conversations cc
	      LEFT JOIN line_oa_accounts oa ON oa.id = cc.line_oa_id`
	if unreadOnly {
		q += ` WHERE cc.unread_admin_count > 0`
	}
	q += ` ORDER BY cc.last_message_at DESC LIMIT $1 OFFSET $2`
	rows, err := r.db.Query(q, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list chat_conversations: %w", err)
	}
	defer rows.Close()
	var out []*ConversationListRow
	for rows.Next() {
		row := &ConversationListRow{}
		var lineOAID sql.NullString
		var lastInbound, lastAdminReply sql.NullTime
		if err := rows.Scan(
			&row.LineUserID, &lineOAID, &row.DisplayName, &row.PictureURL,
			&row.LastMessageAt, &lastInbound, &lastAdminReply,
			&row.UnreadAdminCount, &row.CreatedAt,
			&row.LineOAName,
		); err != nil {
			return nil, err
		}
		if lineOAID.Valid {
			v := lineOAID.String
			row.LineOAID = &v
		}
		if lastInbound.Valid {
			t := lastInbound.Time
			row.LastInboundAt = &t
		}
		if lastAdminReply.Valid {
			t := lastAdminReply.Time
			row.LastAdminReplyAt = &t
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// TouchLastMessage updates last_message_at = NOW(), and (when isInbound) also
// last_inbound_at; (when !isInbound) last_admin_reply_at.
func (r *ChatConversationRepo) TouchLastMessage(lineUserID string, isInbound bool) error {
	var q string
	if isInbound {
		q = `UPDATE chat_conversations
		     SET last_message_at = NOW(), last_inbound_at = NOW()
		     WHERE line_user_id = $1`
	} else {
		q = `UPDATE chat_conversations
		     SET last_message_at = NOW(), last_admin_reply_at = NOW()
		     WHERE line_user_id = $1`
	}
	if _, err := r.db.Exec(q, lineUserID); err != nil {
		return fmt.Errorf("touch chat_conversation: %w", err)
	}
	return nil
}

// IncrementUnread bumps unread_admin_count by 1 (called on every incoming message).
func (r *ChatConversationRepo) IncrementUnread(lineUserID string) error {
	_, err := r.db.Exec(
		`UPDATE chat_conversations SET unread_admin_count = unread_admin_count + 1 WHERE line_user_id = $1`,
		lineUserID,
	)
	if err != nil {
		return fmt.Errorf("increment unread: %w", err)
	}
	return nil
}

// MarkRead zeroes unread_admin_count for one conversation.
func (r *ChatConversationRepo) MarkRead(lineUserID string) error {
	_, err := r.db.Exec(
		`UPDATE chat_conversations SET unread_admin_count = 0 WHERE line_user_id = $1`,
		lineUserID,
	)
	if err != nil {
		return fmt.Errorf("mark read: %w", err)
	}
	return nil
}

// UnreadCount returns the total unread (sum across all conversations) — used
// by the sidebar badge poll.
func (r *ChatConversationRepo) UnreadCount() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COALESCE(SUM(unread_admin_count), 0) FROM chat_conversations`).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// CountAll returns total number of conversations (for pagination).
func (r *ChatConversationRepo) CountAll(unreadOnly bool) (int, error) {
	q := `SELECT COUNT(*) FROM chat_conversations`
	if unreadOnly {
		q += ` WHERE unread_admin_count > 0`
	}
	var n int
	if err := r.db.QueryRow(q).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
