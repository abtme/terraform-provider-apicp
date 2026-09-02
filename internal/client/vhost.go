package client

import "fmt"

// Vhost mirrors apicp's internal/vhost.Vhost JSON shape.
type Vhost struct {
	ID           string `json:"id"`
	Domain       string `json:"domain"`
	NodeID       string `json:"node_id"`
	DocumentRoot string `json:"document_root"`
	UnixUser     string `json:"unix_user"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
	TLSEnabled   bool   `json:"tls_enabled"`
	TLSCertPath  string `json:"tls_cert_path,omitempty"`
	TLSKeyPath   string `json:"tls_key_path,omitempty"`
}

type vhostCreateRequest struct {
	Domain string `json:"domain"`
}

type vhostPatchRequest struct {
	Domain *string `json:"domain,omitempty"`
}

func (c *Client) CreateVhost(domain string) (*Vhost, error) {
	var v Vhost
	if err := c.Post("/v1/vhosts", vhostCreateRequest{Domain: domain}, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (c *Client) GetVhost(id string) (*Vhost, error) {
	var v Vhost
	if err := c.Get("/v1/vhosts/"+id, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// RenameVhost changes a vhost's domain — apicpd handles this as
// remove-then-reapply under the new domain, but the vhost's own ID and
// every other attribute stay the same.
func (c *Client) RenameVhost(id, newDomain string) (*Vhost, error) {
	var v Vhost
	if err := c.Patch("/v1/vhosts/"+id, vhostPatchRequest{Domain: &newDomain}, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (c *Client) DeleteVhost(id string) error {
	return c.Delete("/v1/vhosts/" + id)
}

// Certificate mirrors apicp's internal/tlscert.Certificate JSON shape.
type Certificate struct {
	ID        string `json:"id"`
	VhostID   string `json:"vhost_id"`
	Domain    string `json:"domain"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	NotBefore string `json:"not_before,omitempty"`
	NotAfter  string `json:"not_after,omitempty"`
	IssuedAt  string `json:"issued_at"`
}

// IssueCertificate implements POST /v1/vhosts/{id}/certificate — takes no
// request body, ACME issuance is driven entirely by the vhost's own state.
func (c *Client) IssueCertificate(vhostID string) (*Certificate, error) {
	var cert Certificate
	if err := c.Post(fmt.Sprintf("/v1/vhosts/%s/certificate", vhostID), nil, &cert); err != nil {
		return nil, err
	}
	return &cert, nil
}

func (c *Client) GetCertificate(vhostID string) (*Certificate, error) {
	var cert Certificate
	if err := c.Get(fmt.Sprintf("/v1/vhosts/%s/certificate", vhostID), &cert); err != nil {
		return nil, err
	}
	return &cert, nil
}

func (c *Client) DeleteCertificate(vhostID string) error {
	return c.Delete(fmt.Sprintf("/v1/vhosts/%s/certificate", vhostID))
}
