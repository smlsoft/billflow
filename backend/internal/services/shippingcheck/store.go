package shippingcheck

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const settingsKey = "email.shipping_checks"

var ErrConflict = errors.New("การตั้งค่าถูกแก้ไขแล้ว กรุณาโหลดใหม่")

type Settings struct {
	Config
	Revision string `json:"revision"`
}
type Enrollment struct {
	Profile    Profile  `json:"profile"`
	Recipients []string `json:"recipients"`
	Revision   string   `json:"revision"`
}
type Entry struct {
	ID             string     `json:"id"`
	MessageID      string     `json:"message_id"`
	ArtifactID     string     `json:"artifact_id"`
	ArtifactBillID string     `json:"artifact_bill_id"`
	Policy         Enrollment `json:"policy"`
	Result         Result     `json:"result"`
	CreatedAt      time.Time  `json:"created_at"`
}
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db} }
func (s *Store) Load() (Settings, error) {
	c := Settings{Config: Config{Profiles: []Profile{}, Policies: []Policy{}}}
	var data string
	err := s.db.QueryRow(`SELECT value FROM app_settings WHERE key=$1`, settingsKey).Scan(&data)
	if err == sql.ErrNoRows {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	err = json.Unmarshal([]byte(data), &c)
	return c, err
}
func (s *Store) Save(c Settings, user string) (Settings, error) {
	if err := c.Validate(); err != nil {
		return c, err
	}
	// Validate account references without reading mailbox passwords.
	for _, p := range c.Policies {
		var channel string
		if err := s.db.QueryRow(`SELECT channel FROM imap_accounts WHERE id=$1`, p.AccountID).Scan(&channel); err != nil {
			return c, fmt.Errorf("ไม่พบกล่องอีเมลที่เลือก")
		}
		if channel != "shopee" && channel != "lazada" {
			return c, fmt.Errorf("รองรับเฉพาะกล่อง Shopee/Lazada")
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return c, err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO app_settings(key,value,is_secret) VALUES($1,'{}',false) ON CONFLICT(key) DO NOTHING`, settingsKey)
	if err != nil {
		return c, err
	}
	var oldData string
	if err = tx.QueryRow(`SELECT value FROM app_settings WHERE key=$1 FOR UPDATE`, settingsKey).Scan(&oldData); err != nil {
		return c, err
	}
	var old Settings
	if err = json.Unmarshal([]byte(oldData), &old); err != nil {
		return c, err
	}
	if old.Revision != c.Revision {
		return c, ErrConflict
	}
	c.Revision = time.Now().UTC().Format(time.RFC3339Nano)
	for i := range c.Policies {
		c.Policies[i].EnabledAt = ""
		if c.Policies[i].Enabled {
			c.Policies[i].EnabledAt = c.Revision
			for _, prev := range old.Policies {
				if prev.AccountID == c.Policies[i].AccountID && prev.Enabled {
					c.Policies[i].EnabledAt = prev.EnabledAt
				}
			}
		}
	}
	data, err := json.Marshal(c)
	if err != nil {
		return c, err
	}
	_, err = tx.Exec(`UPDATE app_settings SET value=$1,updated_by=$2,updated_at=now() WHERE key=$3`, string(data), user, settingsKey)
	if err != nil {
		return c, err
	}
	detail, _ := json.Marshal(map[string]any{"before": old, "after": c})
	_, err = tx.Exec(`INSERT INTO audit_logs(action,user_id,source,detail) VALUES('shipping_check_config_updated',$1,'settings',$2)`, user, string(detail))
	if err != nil {
		return c, err
	}
	return c, tx.Commit()
}

// Enroll is called only while constructing a NEW purchase bill. Historical bills
// are never enrolled by an email follow-up. Rules are frozen for that bill.
func (s *Store) Enroll(raw map[string]interface{}, account string) error {
	c, err := s.Load()
	if err != nil {
		return err
	}
	for _, policy := range c.Policies {
		if policy.AccountID == account && policy.Enabled {
			for _, p := range c.Profiles {
				if p.ID == policy.ProfileID {
					raw["shipping_check_policy"] = Enrollment{p, policy.Recipients, c.Revision}
					raw["shipping_check_summary"] = map[string]any{"status": "unverifiable", "attention": true, "reason": "รอตรวจข้อมูลจัดส่ง"}
					return nil
				}
			}
			return fmt.Errorf("shipping profile missing")
		}
	}
	return nil
}
func (s *Store) Record(billID, messageID, body, artifactID string) error {
	var raw []byte
	var source string
	if err := s.db.QueryRow(`SELECT raw_data,source FROM bills WHERE id=$1 AND bill_type='purchase'`, billID).Scan(&raw, &source); err != nil {
		return err
	}
	if source != "shopee_shipped" && source != "lazada_email" {
		return nil
	}
	var data struct {
		Policy *Enrollment `json:"shipping_check_policy"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	if data.Policy == nil {
		return nil
	}
	var hasHistory bool
	if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM bill_shipping_checks WHERE bill_id=$1)`, billID).Scan(&hasHistory); err != nil {
		return err
	}
	if hasHistory && len(body) <= 4*1024*1024 && !sectionRE.MatchString(textLines(body)) {
		return nil
	}
	result := Check(body, data.Policy.Profile, data.Policy.Recipients)
	key := messageID
	if key == "" {
		key = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(body)))
	}
	policyJSON, _ := json.Marshal(data.Policy)
	resultJSON, _ := json.Marshal(result)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Serialize per bill so parallel email arrivals cannot clear attention flags.
	if _, err = tx.Exec(`SELECT id FROM bills WHERE id=$1 FOR UPDATE`, billID); err != nil {
		return err
	}
	var artifact any
	if artifactID != "" {
		artifact = artifactID
	}
	inserted, err := tx.Exec(`INSERT INTO bill_shipping_checks(bill_id,message_key,message_id,artifact_id,policy,result) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(bill_id,message_key) DO NOTHING`, billID, key, messageID, artifact, string(policyJSON), string(resultJSON))
	if err != nil {
		return err
	}
	n, err := inserted.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return tx.Commit()
	}
	// Keep cumulative attention even when the latest event matches.
	_, err = tx.Exec(`UPDATE bills SET raw_data=jsonb_set(COALESCE(raw_data,'{}'::jsonb),'{shipping_check_summary}',jsonb_build_object('status',$2::text,'attention',COALESCE(raw_data->'shipping_check_summary'->>'has_warning','false')='true' OR $2<>'match','has_warning',COALESCE(raw_data->'shipping_check_summary'->>'has_warning','false')='true' OR $2<>'match','checked_at',now(),'reason',$3::text),true) WHERE id=$1`, billID, result.Status, result.Reason)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) History(billID string) ([]Entry, error) {
	rows, err := s.db.Query(`SELECT sc.id,sc.message_id,COALESCE(sc.artifact_id::text,''),COALESCE(a.bill_id::text,''),sc.policy,sc.result,sc.created_at FROM bill_shipping_checks sc LEFT JOIN bill_artifacts a ON a.id=sc.artifact_id WHERE sc.bill_id=$1 ORDER BY sc.created_at DESC,sc.id DESC`, billID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		var p, r []byte
		if err = rows.Scan(&e.ID, &e.MessageID, &e.ArtifactID, &e.ArtifactBillID, &p, &r, &e.CreatedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(p, &e.Policy); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(r, &e.Result); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ArtifactID reuses a shared email only within the same source and mailbox.
func (s *Store) ArtifactID(billID, messageID string) (string, error) {
	if messageID == "" {
		return "", nil
	}
	var id string
	err := s.db.QueryRow(`SELECT a.id FROM bill_artifacts a JOIN bills owner ON owner.id=a.bill_id JOIN bills target ON target.id=$1 WHERE a.kind='email_html' AND a.source_meta->>'message_id'=$2 AND owner.source=target.source AND owner.raw_data->>'imap_account_id'=target.raw_data->>'imap_account_id' ORDER BY (a.bill_id=target.id) DESC,a.created_at,a.id LIMIT 1`, billID, messageID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

// Sample loads only non-secret source metadata. Preview never enters a bill
// enrichment, reconciliation, or SML path.
type Sample struct{ ID, Source, BillType, AccountID, MessageID string }

func (s *Store) Sample(billID string) (*Sample, error) {
	v := &Sample{ID: billID}
	err := s.db.QueryRow(`SELECT source,bill_type,COALESCE(raw_data->>'imap_account_id',''),COALESCE(raw_data->>'email_message_id','') FROM bills WHERE id=$1`, billID).Scan(&v.Source, &v.BillType, &v.AccountID, &v.MessageID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return v, err
}
