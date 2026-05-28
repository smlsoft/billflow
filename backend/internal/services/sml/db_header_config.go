package sml

// SMLDBHeaderConfig holds per-tenant PostgreSQL credentials that BillFlow
// sends as X-DB-* HTTP headers to sml-api-byboss on every request so that
// sml-api-byboss connects to the correct tenant database.
//
// IMPORTANT: Send to sml-api-byboss ONLY.
// NEVER include these headers in requests to Stock Request URL.
type SMLDBHeaderConfig struct {
	Host       string
	Port       string
	User       string
	Password   string // never log this field
	Name       string
	ImagesName string
	LogName    string
}

// IsSet returns true only when all 5 required fields are non-empty.
// A config with any required field missing must not send headers because
// sml-api-byboss cannot identify the correct database from a partial config.
func (db *SMLDBHeaderConfig) IsSet() bool {
	return db != nil &&
		db.Host != "" && db.Port != "" && db.User != "" &&
		db.Password != "" && db.Name != ""
}

// ApplyToHeaders adds X-DB-* headers to h.
// All header names are defined here — change only this function if
// sml-api-byboss renames its expected headers.
func (db *SMLDBHeaderConfig) ApplyToHeaders(h map[string]string) {
	if !db.IsSet() {
		return
	}
	h["X-DB-Host"]        = db.Host
	h["X-DB-Port"]        = db.Port
	h["X-DB-User"]        = db.User
	h["X-DB-Password"]    = db.Password
	h["X-DB-Name"]        = db.Name
	h["X-DB-Images-Name"] = db.ImagesName
	h["X-DB-Log-Name"]    = db.LogName
}

// DBHeaderFn is the function type injected into all SML clients and handlers
// that make requests to sml-api-byboss. It returns the current DB config from
// cache, or nil when sml_db.* settings are not yet configured.
type DBHeaderFn func() *SMLDBHeaderConfig
