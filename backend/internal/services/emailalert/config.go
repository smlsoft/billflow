// Package emailalert delivers opt-in operational alerts, independently of bill/SML processing.
package emailalert

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

const configKey = "email.alerts.config"
const secretKey = "email.alerts.smtp_password"

type RequestError string

func (e RequestError) Error() string { return string(e) }

var ErrConflict = errors.New("การตั้งค่าถูกแก้ไขแล้ว กรุณาโหลดค่าล่าสุด")
var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var KindLabels = map[string]string{"shipping_mismatch": "ข้อมูลจัดส่งไม่ตรง", "shipping_unverifiable": "ตรวจจัดส่งไม่ได้", "email_incomplete": "อ่านอีเมลไม่ครบ", "sml_failed": "ส่ง SML ล้มเหลว"}

type Config struct {
	Enabled     bool     `json:"enabled"`
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	TLSMode     string   `json:"tls_mode"`
	Username    string   `json:"username"`
	From        string   `json:"from"`
	Password    string   `json:"password,omitempty"`
	HasPassword bool     `json:"has_password"`
	Recipients  []string `json:"recipients"`
	Kinds       []string `json:"kinds"`
	Sources     []string `json:"sources"`
	AccountIDs  []string `json:"account_ids"`
	Mode        string   `json:"mode"`
	DailyHour   int      `json:"daily_hour"`
	BaseURL     string   `json:"base_url"`
	Revision    string   `json:"revision"`
}

func DefaultConfig() Config {
	return Config{Host: "smtp.gmail.com", Port: 587, TLSMode: "starttls", Mode: "immediate", DailyHour: 9, Recipients: []string{}, Kinds: []string{"shipping_mismatch", "shipping_unverifiable", "email_incomplete", "sml_failed"}, Sources: []string{}, AccountIDs: []string{}}
}
func addressOK(v string) bool {
	if strings.ContainsAny(v, "\r\n") || len(v) > 254 {
		return false
	}
	a, e := mail.ParseAddress(v)
	return e == nil && a.Address == v && a.Name == "" && strings.Contains(v, "@")
}
func (c *Config) Normalize() { c.normalize() }

func (c *Config) normalize() {
	c.Host = strings.ToLower(strings.TrimSpace(c.Host))
	if strings.EqualFold(c.Host, "smtp.gmail.com") {
		c.Password = strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, c.Password)
	}
	c.From = strings.TrimSpace(c.From)
	c.Username = strings.TrimSpace(c.Username)
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	unique := func(v []string) []string {
		out := []string{}
		for _, s := range v {
			s = strings.TrimSpace(s)
			if !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
		return out
	}
	for i := range c.Recipients {
		c.Recipients[i] = strings.ToLower(strings.TrimSpace(c.Recipients[i]))
	}
	c.Recipients = unique(c.Recipients)
	c.Kinds = unique(c.Kinds)
	c.Sources = unique(c.Sources)
	c.AccountIDs = unique(c.AccountIDs)
}
func (c Config) Validate(requireSMTP bool) error {
	if len(c.Recipients) > 20 || len(c.AccountIDs) > 200 {
		return errors.New("รองรับผู้รับไม่เกิน 20 คน และกล่องอีเมลไม่เกิน 200 กล่อง")
	}
	for _, v := range c.Recipients {
		if !addressOK(v) {
			return errors.New("กรอกอีเมลผู้รับให้ถูกต้อง ใช้เฉพาะที่อยู่อีเมล")
		}
	}
	if c.Mode != "immediate" && c.Mode != "daily" {
		return errors.New("เลือกรูปแบบแจ้งเตือนให้ถูกต้อง")
	}
	if c.DailyHour < 0 || c.DailyHour > 23 {
		return errors.New("เวลาสรุปรายวันต้องอยู่ระหว่าง 0–23 นาฬิกา")
	}
	for _, k := range c.Kinds {
		if _, ok := KindLabels[k]; !ok {
			return errors.New("ประเภทแจ้งเตือนไม่รองรับ")
		}
	}
	for _, s := range c.Sources {
		if s != "shopee_shipped" && s != "lazada_email" {
			return errors.New("ตัวกรองช่องทางไม่รองรับ")
		}
	}
	for _, id := range c.AccountIDs {
		if !uuidRE.MatchString(id) {
			return errors.New("รหัสกล่องอีเมลไม่ถูกต้อง")
		}
	}
	if c.Host == "smtp.gmail.com" && c.Password != "" && len(c.Password) != 16 {
		return errors.New("App Password ของ Gmail ต้องมี 16 ตัวหลังตัดช่องว่าง ไม่ใช่รหัสผ่านปกติ")
	}
	if len(c.Password) > 4096 || len(c.Username) > 254 || len(c.Host) > 253 || len(c.BaseURL) > 2048 {
		return errors.New("ข้อมูลยาวเกินกำหนด")
	}
	if c.TLSMode != "starttls" && c.TLSMode != "tls" {
		return errors.New("SMTP ต้องใช้ STARTTLS หรือ TLS เท่านั้น")
	}
	if c.Port != 587 && c.Port != 465 {
		return errors.New("รองรับ SMTP พอร์ต 587 หรือ 465 เท่านั้น")
	}
	if c.BaseURL != "" {
		u, e := url.Parse(c.BaseURL)
		if e != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(c.BaseURL, "\r\n") {
			return errors.New("URL หน้า BillFlow ไม่ถูกต้อง")
		}
	}
	if !requireSMTP && !c.Enabled {
		return nil
	}
	if c.Host == "" || strings.ContainsAny(c.Host, "/: \r\n\t") || strings.EqualFold(c.Host, "localhost") || net.ParseIP(c.Host) != nil {
		return errors.New("กรอกชื่อโฮสต์ SMTP เช่น smtp.gmail.com")
	}
	if !((c.Port == 587 && c.TLSMode == "starttls") || (c.Port == 465 && c.TLSMode == "tls")) {
		return errors.New("พอร์ตและรูปแบบ TLS ไม่ตรงกัน")
	}
	if !addressOK(c.From) || c.Username == "" || strings.ContainsAny(c.Username, "\r\n") {
		return errors.New("กรอกผู้ส่งและบัญชี SMTP ให้ครบ")
	}
	if !c.HasPassword && c.Password == "" {
		return errors.New("กรอกรหัสผ่าน SMTP/App Password ก่อน")
	}
	if len(c.Recipients) == 0 || len(c.Kinds) == 0 || c.BaseURL == "" {
		return errors.New("เพิ่มผู้รับ เลือกประเภทปัญหา และกรอก URL หน้า BillFlow ก่อน")
	}
	return nil
}

type Issue struct {
	Kind      string `json:"kind"`
	EntityKey string `json:"entity_key"`
	Source    string `json:"source"`
	AccountID string `json:"account_id"`
	OrderID   string `json:"order_id"`
	Path      string `json:"path"`
	Summary   string `json:"summary"`
}

func (c Config) matches(i Issue) bool {
	return slices.Contains(c.Kinds, i.Kind) && (len(c.Sources) == 0 || slices.Contains(c.Sources, i.Source)) && (len(c.AccountIDs) == 0 || slices.Contains(c.AccountIDs, i.AccountID))
}
func dedupeKey(id string, occ int, recipient, period string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s\x00%s", id, occ, recipient, period)))
	return hex.EncodeToString(sum[:])
}

type Store struct {
	db  *sql.DB
	key [32]byte
}

func NewStore(db *sql.DB, key string) *Store {
	return &Store{db: db, key: sha256.Sum256([]byte("billflow-email-alert-password-v1\x00" + key))}
}
func (s *Store) encrypt(v string) (string, error) {
	block, e := aes.NewCipher(s.key[:])
	if e != nil {
		return "", e
	}
	g, e := cipher.NewGCM(block)
	if e != nil {
		return "", e
	}
	nonce := make([]byte, g.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return "", e
	}
	return base64.StdEncoding.EncodeToString(g.Seal(nonce, nonce, []byte(v), nil)), nil
}
func (s *Store) decrypt(v string) (string, error) {
	raw, e := base64.StdEncoding.DecodeString(v)
	if e != nil {
		return "", errors.New("อ่านรหัสผ่าน SMTP ไม่สำเร็จ กรุณาบันทึกรหัสผ่านใหม่")
	}
	b, _ := aes.NewCipher(s.key[:])
	g, _ := cipher.NewGCM(b)
	if len(raw) < g.NonceSize() {
		return "", errors.New("รหัสผ่าน SMTP ไม่ถูกต้อง")
	}
	plain, e := g.Open(nil, raw[:g.NonceSize()], raw[g.NonceSize():], nil)
	if e != nil {
		return "", errors.New("ถอดรหัสผ่าน SMTP ไม่สำเร็จ กรุณาบันทึกรหัสผ่านใหม่")
	}
	return string(plain), nil
}
