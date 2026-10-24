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
	DefaultMaxMessageSizeMB int64  `json:"default_max_message_size_mb"`
	UpdatedAt               string `json:"updated_at,omitempty"`
}

type mailSettingsRequest struct {
	MaxMessageSizeMB int64 `json:"max_message_size_mb"`
}

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

func (c *Client) SetMailSettings(maxMessageSizeMB int64) (*MailSettings, error) {
	var s MailSettings
	if err := c.Put("/v1/mail/settings", mailSettingsRequest{MaxMessageSizeMB: maxMessageSizeMB}, &s); err != nil {
		return nil, mailSettingsErr(err)
	}
	return &s, nil
}
