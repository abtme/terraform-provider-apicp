package client

import (
	"errors"
	"fmt"
)

// MailSettings mirrors apicp's GET/PUT /v1/mail/settings JSON shape: the
// mail server's server-wide settings. The record is meant to grow (more
// settings later), so only fields this provider knows are sent on PUT.
type MailSettings struct {
	// MaxMessageSizeMB is the largest message the mail server accepts,
	// in or out (MB = 1024*1024 bytes); 0 = unlimited.
	MaxMessageSizeMB int64 `json:"max_message_size_mb"`
	// DefaultMaxMessageSizeMB is apicp's own default - what a server has
	// until someone sets it, and what destroying apicp_mail_settings
	// goes back to.
	DefaultMaxMessageSizeMB int64 `json:"default_max_message_size_mb"`
	// AllowWeakPasswordSchemes lets mailboxes log in with password hashes
	// Dovecot counts as weak (MD5-crypt and the like, kept from an older
	// server). nil = an apicpd from before the setting (see
	// ErrWeakPasswordSchemesUnsupported).
	AllowWeakPasswordSchemes *bool `json:"allow_weak_password_schemes,omitempty"`
	// Compression is what the server's Dovecot can do for stored-mail
	// compression (read-only; nil before apicp v0.8.0).
	Compression *MailCompressionSupport `json:"compression,omitempty"`
	UpdatedAt   string                  `json:"updated_at,omitempty"`
}

// MailCompressionSupport is the compression part of GET /v1/mail/settings.
type MailCompressionSupport struct {
	// Methods are those the plugin can write; Ready is false while a domain
	// cannot have compression on (plugin missing or broken, lookup not in place).
	Methods []string `json:"methods"`
	Ready   bool     `json:"ready"`
}

// WeakPasswordSchemes is AllowWeakPasswordSchemes with an old apicpd's
// missing field read as false (it has no such switch).
func (s *MailSettings) WeakPasswordSchemes() bool {
	return s.AllowWeakPasswordSchemes != nil && *s.AllowWeakPasswordSchemes
}

// mailSettingsRequest always sends both settings this provider knows.
type mailSettingsRequest struct {
	MaxMessageSizeMB         int64 `json:"max_message_size_mb"`
	AllowWeakPasswordSchemes bool  `json:"allow_weak_password_schemes"`
}

// ErrWeakPasswordSchemesUnsupported is allow_weak_password_schemes = true
// against an apicpd that does not know the setting (it ignores it).
var ErrWeakPasswordSchemesUnsupported = errors.New("this apicpd does not support allow_weak_password_schemes (needs a newer apicp) - upgrade apicp, or leave it false")

// ErrMailSettingsUnsupported is what a 404 from /v1/mail/settings means:
// an apicpd from before server-wide mail settings existed.
var ErrMailSettingsUnsupported = errors.New("this apicpd has no /v1/mail/settings (needs the release after v0.6.18) - upgrade apicp first")

func mailSettingsErr(err error) error {
	if errors.Is(err, ErrNotFound) {
		// Both stay matchable: errors.Is(err, ErrNotFound) still holds.
		return fmt.Errorf("%w (%w)", ErrMailSettingsUnsupported, err)
	}
	return err
}

func (c *Client) GetMailSettings() (*MailSettings, error) {
	var s MailSettings
	if err := c.Get("/v1/mail/settings", &s); err != nil {
		return nil, mailSettingsErr(err)
	}
	return &s, nil
}

func (c *Client) SetMailSettings(maxMessageSizeMB int64, allowWeakPasswordSchemes bool) (*MailSettings, error) {
	var s MailSettings
	req := mailSettingsRequest{MaxMessageSizeMB: maxMessageSizeMB, AllowWeakPasswordSchemes: allowWeakPasswordSchemes}
	if err := c.Put("/v1/mail/settings", req, &s); err != nil {
		return nil, mailSettingsErr(err)
	}
	return &s, nil
}
