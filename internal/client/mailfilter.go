package client

// MailFilterStatus mirrors apicp's GET /v1/mail/antispam|antivirus JSON
// shape (PLAN.md §8 phase 6).
type MailFilterStatus struct {
	Enabled   bool   `json:"enabled"`
	UpdatedAt string `json:"updated_at"`
}

func (c *Client) EnableAntispam() (*MailFilterStatus, error) {
	var s MailFilterStatus
	if err := c.Post("/v1/mail/antispam", nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) GetAntispam() (*MailFilterStatus, error) {
	var s MailFilterStatus
	if err := c.Get("/v1/mail/antispam", &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) DisableAntispam() error {
	return c.Delete("/v1/mail/antispam")
}

func (c *Client) EnableAntivirus() (*MailFilterStatus, error) {
	var s MailFilterStatus
	if err := c.Post("/v1/mail/antivirus", nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) GetAntivirus() (*MailFilterStatus, error) {
	var s MailFilterStatus
	if err := c.Get("/v1/mail/antivirus", &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) DisableAntivirus() error {
	return c.Delete("/v1/mail/antivirus")
}
