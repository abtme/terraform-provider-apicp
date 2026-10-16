package client

import "fmt"

// Vhost mirrors apicp's internal/vhost.Vhost JSON shape.
type Vhost struct {
	ID     string `json:"id"`
	Domain string `json:"domain"`
	// Aliases are extra host names the vhost serves; one certificate covers the domain and all of them.
	Aliases      []string `json:"aliases,omitempty"`
	NodeID       string   `json:"node_id"`
	DocumentRoot string   `json:"document_root"`
	UnixUser     string   `json:"unix_user"`
	Status       string   `json:"status"`
	Error        string   `json:"error,omitempty"`
	TLSEnabled   bool     `json:"tls_enabled"`
	TLSCertPath  string   `json:"tls_cert_path,omitempty"`
	TLSKeyPath   string   `json:"tls_key_path,omitempty"`
	// PHPVersion is "" for a static (no PHP-FPM) vhost, or one of
	// apicp's PLAN.md §8 phase 2 supported versions (see the provider's
	// own php_version schema description for the current list).
	PHPVersion string `json:"php_version,omitempty"`
	// NodeVersion is set when the vhost runs a Node.js app (then PHPVersion is ""), AppEntry is its start file
	// relative to ~/app ("" = apicp's default) and AppPort the loopback port apicp gave it.
	NodeVersion string `json:"node_version,omitempty"`
	AppEntry    string `json:"app_entry,omitempty"`
	AppPort     int    `json:"app_port,omitempty"`
}

// VhostRuntime is what runs a vhost: nothing (static), a PHP version, or a Node.js app. apicp refuses PHP and Node
// together.
type VhostRuntime struct {
	PHPVersion  string
	NodeVersion string
	AppEntry    string
}

type vhostCreateRequest struct {
	Domain      string   `json:"domain"`
	Aliases     []string `json:"aliases,omitempty"`
	PHPVersion  string   `json:"php_version,omitempty"`
	NodeVersion string   `json:"node_version,omitempty"`
	AppEntry    string   `json:"app_entry,omitempty"`
}

type vhostPatchRequest struct {
	Domain      *string   `json:"domain,omitempty"`
	PHPVersion  *string   `json:"php_version,omitempty"`
	NodeVersion *string   `json:"node_version,omitempty"`
	AppEntry    *string   `json:"app_entry,omitempty"`
	Aliases     *[]string `json:"aliases,omitempty"`
}

func (c *Client) CreateVhost(domain string, aliases []string, rt VhostRuntime) (*Vhost, error) {
	var v Vhost
	req := vhostCreateRequest{Domain: domain, Aliases: aliases, PHPVersion: rt.PHPVersion, NodeVersion: rt.NodeVersion, AppEntry: rt.AppEntry}
	if err := c.Post("/v1/vhosts", req, &v); err != nil {
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

// PatchVhost updates domain and/or the runtime - apicpd handles a domain
// change as remove-then-reapply under the new domain, but the vhost's own
// ID and every other attribute stay the same. Every field is always sent
// (never nil) since the resource always has a resolved plan value for
// each by the time Update runs; PATCH is idempotent, so re-sending an
// unchanged value is a no-op. Sending all three runtime fields together is
// also what lets a vhost move from PHP to Node (or back) in one request.
//
// aliases is always sent, and as a list even when empty, because the API replaces the whole list: an
// empty list is how an alias is removed.
func (c *Client) PatchVhost(id, domain string, aliases []string, rt VhostRuntime) (*Vhost, error) {
	var v Vhost
	if aliases == nil {
		aliases = []string{}
	}

	req := vhostPatchRequest{Domain: &domain, PHPVersion: &rt.PHPVersion, NodeVersion: &rt.NodeVersion, AppEntry: &rt.AppEntry, Aliases: &aliases}
	if err := c.Patch("/v1/vhosts/"+id, req, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (c *Client) DeleteVhost(id string) error {
	return c.Delete("/v1/vhosts/" + id)
}

// Certificate mirrors apicp's internal/tlscert.Certificate JSON shape.
type Certificate struct {
	ID      string `json:"id"`
	VhostID string `json:"vhost_id"`
	Domain  string `json:"domain"`
	// Names are every host name the certificate covers: the vhost's domain, then its aliases.
	Names     []string `json:"names,omitempty"`
	Status    string   `json:"status"`
	Error     string   `json:"error,omitempty"`
	NotBefore string   `json:"not_before,omitempty"`
	NotAfter  string   `json:"not_after,omitempty"`
	IssuedAt  string   `json:"issued_at"`
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
// PublicKeys is the *complete* set of keys currently authorized on the
// vhost — several apicp_ssh_access resources against the same vhost_id
// each grant their own key and all show up here together, not just the
// one a particular resource added.
type SSHAccess struct {
	ID         string   `json:"id"`
	VhostID    string   `json:"vhost_id"`
	UnixUser   string   `json:"unix_user"`
	PublicKeys []string `json:"public_keys"`
	Status     string   `json:"status"`
	Error      string   `json:"error,omitempty"`
}

type sshAccessRequest struct {
	PublicKey string `json:"public_key,omitempty"`
}

// EnableSSHAccess implements POST /v1/vhosts/{id}/ssh-access — additive:
// adds publicKey to whatever set is already authorized (a no-op if it's
// already present), returning the complete resulting set.
func (c *Client) EnableSSHAccess(vhostID, publicKey string) (*SSHAccess, error) {
	var a SSHAccess
	if err := c.Post(fmt.Sprintf("/v1/vhosts/%s/ssh-access", vhostID), sshAccessRequest{PublicKey: publicKey}, &a); err != nil {
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

// DisableSSHAccessKey revokes just publicKey, leaving any other keys
// granted on the vhost active (if publicKey was the last one, apicp
// falls through to a full revoke on its side).
func (c *Client) DisableSSHAccessKey(vhostID, publicKey string) error {
	return c.DeleteWithBody(fmt.Sprintf("/v1/vhosts/%s/ssh-access", vhostID), sshAccessRequest{PublicKey: publicKey})
}

// DisableSSHAccess revokes every key on the vhost.
func (c *Client) DisableSSHAccess(vhostID string) error {
	return c.Delete(fmt.Sprintf("/v1/vhosts/%s/ssh-access", vhostID))
}
