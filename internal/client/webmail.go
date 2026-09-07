package client

// WebmailStatus mirrors apicp's internal/webmail.Status JSON shape
// (PLAN.md §8 phase 4).
type WebmailStatus struct {
	Enabled   bool   `json:"enabled"`
	URL       string `json:"url,omitempty"`
	UpdatedAt string `json:"updated_at"`
}

// EnableWebmail implements POST /v1/webmail - takes no request body,
// nothing to configure beyond on/off.
func (c *Client) EnableWebmail() (*WebmailStatus, error) {
	var s WebmailStatus
	if err := c.Post("/v1/webmail", nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) GetWebmail() (*WebmailStatus, error) {
	var s WebmailStatus
	if err := c.Get("/v1/webmail", &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) DisableWebmail() error {
	return c.Delete("/v1/webmail")
}
