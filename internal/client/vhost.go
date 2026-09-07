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
	// PHPVersion is "" for a static (no PHP-FPM) vhost, or one of
	// apicp's PLAN.md §8 phase 2 supported versions (see the provider's
	// own php_version schema description for the current list).
	PHPVersion string `json:"php_version,omitempty"`
}

type vhostCreateRequest struct {
	Domain     string `json:"domain"`
	PHPVersion string `json:"php_version,omitempty"`
}

type vhostPatchRequest struct {
	Domain     *string `json:"domain,omitempty"`
	PHPVersion *string `json:"php_version,omitempty"`
}

func (c *Client) CreateVhost(domain, phpVersion string) (*Vhost, error) {
	var v Vhost
	if err := c.Post("/v1/vhosts", vhostCreateRequest{Domain: domain, PHPVersion: phpVersion}, &v); err != nil {
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

// PatchVhost updates domain and/or php_version - apicpd handles a domain
// change as remove-then-reapply under the new domain, but the vhost's own
// ID and every other attribute stay the same. Both fields are always sent
// (never nil) since the resource always has a resolved plan value for
// each by the time Update runs; PATCH is idempotent, so re-sending an
// unchanged value is a no-op.
func (c *Client) PatchVhost(id, domain, phpVersion string) (*Vhost, error) {
	var v Vhost
	if err := c.Patch("/v1/vhosts/"+id, vhostPatchRequest{Domain: &domain, PHPVersion: &phpVersion}, &v); err != nil {
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

// SSHAccess mirrors apicp's internal/sshaccess.SSHAccess JSON shape.
type SSHAccess struct {
	ID        string `json:"id"`
	VhostID   string `json:"vhost_id"`
	UnixUser  string `json:"unix_user"`
	PublicKey string `json:"public_key"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

type sshAccessCreateRequest struct {
	PublicKey string `json:"public_key"`
}

// EnableSSHAccess implements POST /v1/vhosts/{id}/ssh-access.
func (c *Client) EnableSSHAccess(vhostID, publicKey string) (*SSHAccess, error) {
	var a SSHAccess
	if err := c.Post(fmt.Sprintf("/v1/vhosts/%s/ssh-access", vhostID), sshAccessCreateRequest{PublicKey: publicKey}, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func (c *Client) GetSSHAccess(vhostID string) (*SSHAccess, error) {
	var a SSHAccess
	if err := c.Get(fmt.Sprintf("/v1/vhosts/%s/ssh-access", vhostID), &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func (c *Client) DisableSSHAccess(vhostID string) error {
	return c.Delete(fmt.Sprintf("/v1/vhosts/%s/ssh-access", vhostID))
}
