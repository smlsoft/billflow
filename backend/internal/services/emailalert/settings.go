package emailalert

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/lib/pq"
)

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadConfig(ctx context.Context, q queryer) (Config, string, error) {
	c := DefaultConfig()
	var raw string
	err := q.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key=$1`, configKey).Scan(&raw)
	if err != nil && err != sql.ErrNoRows {
		return c, "", err
	}
	if err == nil {
		if err = json.Unmarshal([]byte(raw), &c); err != nil {
			return c, "", err
		}
	}
	var enc string
	err = q.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key=$1`, secretKey).Scan(&enc)
	if err != nil && err != sql.ErrNoRows {
		return c, "", err
	}
	c.Password = ""
	c.HasPassword = enc != ""
	return c, enc, nil
}
func (s *Store) Load(ctx context.Context) (Config, error) {
	c, _, err := loadConfig(ctx, s.db)
	return c, err
}
func (s *Store) Save(ctx context.Context, c Config, user string) (Config, error) {
	c.normalize()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Config{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO app_settings(key,value,is_secret) VALUES($1,'{}',false) ON CONFLICT DO NOTHING`, configKey); err != nil {
		return Config{}, err
	}
	var locked string
	if err = tx.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key=$1 FOR UPDATE`, configKey).Scan(&locked); err != nil {
		return Config{}, err
	}
	old, encrypted, err := loadConfig(ctx, tx)
	if err != nil {
		return Config{}, err
	}
	if old.Revision != c.Revision {
		return Config{}, ErrConflict
	}
	c.HasPassword = encrypted != "" || c.Password != ""
	if err = c.Validate(false); err != nil {
		return Config{}, RequestError(err.Error())
	}
	if len(c.AccountIDs) > 0 {
		var found int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM imap_accounts WHERE id=ANY($1::uuid[]) AND channel IN ('shopee','lazada')`, pq.Array(c.AccountIDs)).Scan(&found); err != nil {
			return Config{}, err
		}
		if found != len(c.AccountIDs) {
			return Config{}, RequestError("ไม่พบกล่องอีเมลที่เลือก")
		}
	}
	if encrypted != "" && (old.Host != c.Host || old.Username != c.Username) && c.Password == "" {
		return Config{}, RequestError("เมื่อเปลี่ยนโฮสต์หรือบัญชี SMTP ต้องกรอกรหัสผ่านใหม่")
	}

	changed := c.Password != ""
	if changed {
		encrypted, err = s.encrypt(c.Password)
		if err != nil {
			return Config{}, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO app_settings(key,value,is_secret,updated_by) VALUES($1,$2,true,$3) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,is_secret=true,updated_by=EXCLUDED.updated_by,updated_at=now()`, secretKey, encrypted, user); err != nil {
			return Config{}, err
		}
	}
	c.Password = ""
	c.Revision = time.Now().UTC().Format(time.RFC3339Nano)
	raw, err := json.Marshal(c)
	if err != nil {
		return Config{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app_settings SET value=$2,updated_by=$3,updated_at=now() WHERE key=$1`, configKey, string(raw), user); err != nil {
		return Config{}, err
	}
	// Never put passwords or encrypted credentials in the audit trail.
	audit, _ := json.Marshal(map[string]any{"before": old, "after": c, "password_changed": changed})
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(action,user_id,source,detail) VALUES('email_alert_settings_updated',$1,'settings',$2)`, user, string(audit)); err != nil {
		return Config{}, err
	}
	return c, tx.Commit()
}
