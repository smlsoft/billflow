package emailalert

import (
	"strings"
	"testing"
)

func validConfig() Config {
	return Config{Enabled: true, Host: "smtp.example.com", Port: 587, TLSMode: "starttls", Username: "sender@example.com", From: "sender@example.com", Recipients: []string{"owner@example.com"}, Kinds: []string{"shipping_mismatch"}, Mode: "immediate", DailyHour: 9, BaseURL: "http://example.com:9985", HasPassword: true}
}
func TestConfigSafety(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*Config)
	}{
		{"header injection", func(c *Config) { c.From = "sender@example.com\r\nBcc: attacker@example.com" }},
		{"recipient injection", func(c *Config) { c.Recipients = []string{"owner@example.com\nextra@example.com"} }},
		{"no TLS", func(c *Config) { c.TLSMode = "none" }},
		{"wrong TLS port", func(c *Config) { c.Port = 25 }},
		{"unknown kind", func(c *Config) { c.Kinds = []string{"everything"} }},
		{"unsafe link", func(c *Config) { c.BaseURL = "javascript:alert(1)" }},
		{"missing password", func(c *Config) { c.HasPassword = false }},
		{"invalid hour", func(c *Config) { c.DailyHour = 24 }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := validConfig()
			tt.mutate(&c)
			if c.Validate(true) == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
	c := validConfig()
	if err := c.Validate(true); err != nil {
		t.Fatal(err)
	}
	c.Enabled = false
	c.Host = ""
	c.From = ""
	c.Username = ""
	c.HasPassword = false
	if err := c.Validate(false); err != nil {
		t.Fatal("disabled drafts should save", err)
	}
}
func TestSecretRoundTripAndIsolation(t *testing.T) {
	a := NewStore(nil, "synthetic-key-a")
	b := NewStore(nil, "synthetic-key-b")
	enc, err := a.encrypt("synthetic-password")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(enc, "synthetic-password") {
		t.Fatal("plaintext stored")
	}
	plain, err := a.decrypt(enc)
	if err != nil || plain != "synthetic-password" {
		t.Fatal(err)
	}
	if _, err = b.decrypt(enc); err == nil {
		t.Fatal("other deployment key decrypted secret")
	}
}
func TestFilterAndDeliveryDedup(t *testing.T) {
	c := validConfig()
	issue := Issue{Kind: "shipping_mismatch", EntityKey: "bill:1", Source: "lazada_email", AccountID: "mail"}
	if !c.matches(issue) {
		t.Fatal("matching issue ignored")
	}
	c.Sources = []string{"shopee_shipped"}
	if c.matches(issue) {
		t.Fatal("wrong channel passed")
	}
	c.Sources = nil
	c.AccountIDs = []string{"another"}
	if c.matches(issue) {
		t.Fatal("wrong mailbox passed")
	}
	if dedupeKey("i", 1, "owner@example.com", "event") != dedupeKey("i", 1, "owner@example.com", "event") {
		t.Fatal("unstable dedup")
	}
	if dedupeKey("i", 1, "owner@example.com", "event") == dedupeKey("i", 2, "owner@example.com", "event") {
		t.Fatal("reopened problem suppressed")
	}
}

func TestGmailAppPasswordNormalization(t *testing.T) {
	c := validConfig()
	c.Host = "SMTP.GMAIL.COM"
	c.Password = "abcd efgh ijkl mnop"
	c.Normalize()
	if c.Password != "abcdefghijklmnop" || c.Host != "smtp.gmail.com" {
		t.Fatal("Gmail copied password not normalized")
	}
	if err := c.Validate(true); err != nil {
		t.Fatal(err)
	}
	c.Password = "normal-password"
	if c.Validate(true) == nil {
		t.Fatal("non-app password length accepted")
	}
}
