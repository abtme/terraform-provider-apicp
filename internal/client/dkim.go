package client

import "fmt"

// DKIM mirrors apicp's internal/dkim status JSON shape (PLAN.md §8
// phase 6) - PrivateKeyPEM/PublicKeyB64 are never returned by the API,
// only the DNS record apicp derived from them.
type DKIM struct {
	MailDomainID   string `json:"mail_domain_id"`
	Domain         string `json:"domain"`
	Enabled        bool   `json:"enabled"`
	Selector       string `json:"selector"`
	DNSRecordName  string `json:"dns_record_name"`
	DNSRecordValue string `json:"dns_record_value"`
	DNSAutoCreated bool   `json:"dns_auto_created"`
}

// EnableDKIM implements POST /v1/mail/domains/{id}/dkim - idempotent,
// takes no request body (apicp generates the keypair server-side).
func (c *Client) EnableDKIM(mailDomainID string) (*DKIM, error) {
	var d DKIM
	if err := c.Post(fmt.Sprintf("/v1/mail/domains/%s/dkim", mailDomainID), nil, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (c *Client) GetDKIM(mailDomainID string) (*DKIM, error) {
	var d DKIM
	if err := c.Get(fmt.Sprintf("/v1/mail/domains/%s/dkim", mailDomainID), &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (c *Client) DisableDKIM(mailDomainID string) error {
	return c.Delete(fmt.Sprintf("/v1/mail/domains/%s/dkim", mailDomainID))
}
