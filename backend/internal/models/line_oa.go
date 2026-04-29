package models

import "time"

// LineOAAccount is one configured LINE Official Account. Multi-OA support:
// admin can register N OAs, each with its own credentials. Webhook URL per OA
// is /webhook/line/<id>.
//
// Sensitive fields (channel_secret, channel_access_token) ride in the JSON
// response only when admin reads a single account for editing — the list
// endpoint masks them.
type LineOAAccount struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	ChannelSecret       string    `json:"channel_secret,omitempty"`
	ChannelAccessToken  string    `json:"channel_access_token,omitempty"`
	BotUserID           string    `json:"bot_user_id"`
	AdminUserID         string    `json:"admin_user_id"`
	Greeting            string    `json:"greeting"`
	Enabled             bool      `json:"enabled"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// LineOAAccountUpsert is the admin-supplied payload for POST/PUT.
// channel_secret + channel_access_token can be left empty on UPDATE to keep
// the existing value (avoids forcing the admin to re-enter long tokens).
type LineOAAccountUpsert struct {
	Name               string `json:"name" binding:"required"`
	ChannelSecret      string `json:"channel_secret"`
	ChannelAccessToken string `json:"channel_access_token"`
	AdminUserID        string `json:"admin_user_id"`
	Greeting           string `json:"greeting"`
	Enabled            *bool  `json:"enabled"`
}
