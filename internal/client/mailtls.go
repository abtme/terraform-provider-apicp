package client

import "fmt"

// MailTLSStatus mirrors apicp's GET /v1/mail/tls JSON shape.
type MailTLSStatus struct {
	Enabled        bool   `json:"enabled"`
	Host           string `json:"host,omitempty"`
	SubmissionPort int    `json:"submission_port,omitempty"`
	UpdatedAt      string `json:"updated_at"`
}

func (c *Client) EnableMailTLS() (*MailTLSStatus, error) {
	var s MailTLSStatus
	if err := c.Post("/v1/mail/tls", nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) GetMailTLS() (*MailTLSStatus, error) {
	var s MailTLSStatus
	if err := c.Get("/v1/mail/tls", &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) DisableMailTLS() error {
	return c.Delete("/v1/mail/tls")
}

// ClientConfigRecord is one mail-client discovery record and what apicp did
// with it: "created", "exists" (already there, left alone) or "manual"
// (no matching apicp zone - add it wherever the domain's DNS lives).
type ClientConfigRecord struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	Why     string `json:"why"`
	Action  string `json:"action"`
	Error   string `json:"error,omitempty"`
}

// ClientConfig mirrors apicp's mail-domain client-config response.
type ClientConfig struct {
	MailDomainID  string               `json:"mail_domain_id"`
	Domain        string               `json:"domain"`
	Enabled       bool                 `json:"enabled"`
	Host          string               `json:"host"`
	AutoconfigURL string               `json:"autoconfig_url"`
	Records       []ClientConfigRecord `json:"records"`
}

// EnableClientConfig implements POST /v1/mail/domains/{id}/client-config -
// idempotent, no request body.
func (c *Client) EnableClientConfig(mailDomainID string) (*ClientConfig, error) {
	var cc ClientConfig
	if err := c.Post(fmt.Sprintf("/v1/mail/domains/%s/client-config", mailDomainID), nil, &cc); err != nil {
		return nil, err
	}
	return &cc, nil
}

func (c *Client) GetClientConfig(mailDomainID string) (*ClientConfig, error) {
	var cc ClientConfig
	if err := c.Get(fmt.Sprintf("/v1/mail/domains/%s/client-config", mailDomainID), &cc); err != nil {
		return nil, err
	}
	return &cc, nil
}

func (c *Client) DisableClientConfig(mailDomainID string) error {
	return c.Delete(fmt.Sprintf("/v1/mail/domains/%s/client-config", mailDomainID))
}
