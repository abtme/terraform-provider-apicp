package client

import "fmt"

// SMTPRelay mirrors apicp's internal/smtprelay.SMTPRelay JSON shape
// (PLAN.md §2 "SMTP smarthost / relay") - one per node. Password is
// operator-supplied (not apicp-generated, unlike an apicp_database
// password), so unlike that resource's Redacted() treatment, apicp
// never even echoes it back on GET - only the create/enable response
// carries it, same "returned once" convention as everywhere else a
// secret crosses this API.
type SMTPRelay struct {
	ID     string `json:"id"`
	NodeID string `json:"node_id"`
	// Name and Default exist from apicp v0.8.0 (several relays per node,
	// one of them the default; the single-relay routes are the default).
	Name     string `json:"name,omitempty"`
	Default  bool   `json:"default"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

type smtpRelayCreateRequest struct {
	Host     string `json:"host"`
	Port     int64  `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// EnableSMTPRelay implements POST /v1/nodes/{id}/smtp-relay - also used
// to replace an existing relay config for the node (same "re-running
// this call is how you update it" shape as apicp_ssh_access).
func (c *Client) EnableSMTPRelay(nodeID, host string, port int64, username, password string) (*SMTPRelay, error) {
	var r SMTPRelay
	if err := c.Post(fmt.Sprintf("/v1/nodes/%s/smtp-relay", nodeID), smtpRelayCreateRequest{
		Host: host, Port: port, Username: username, Password: password,
	}, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) GetSMTPRelay(nodeID string) (*SMTPRelay, error) {
	var r SMTPRelay
	if err := c.Get(fmt.Sprintf("/v1/nodes/%s/smtp-relay", nodeID), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) DisableSMTPRelay(nodeID string) error {
	return c.Delete(fmt.Sprintf("/v1/nodes/%s/smtp-relay", nodeID))
}

// Named relays (apicp v0.8.0): /v1/nodes/{id}/smtp-relays. A node has any
// number; at most one is its default (the one /v1/nodes/{id}/smtp-relay
// manages), and a mail domain can name another with relay_id.

// SMTPRelayCreate is the body of POST /v1/nodes/{id}/smtp-relays.
type SMTPRelayCreate struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int64  `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Default  bool   `json:"default,omitempty"`
}

// SMTPRelayPatch is the body of PATCH /v1/nodes/{id}/smtp-relays/{relayID};
// nil fields are left as they are. Changing host or port of a relay with a
// login needs the password in the same request.
type SMTPRelayPatch struct {
	Name     *string `json:"name,omitempty"`
	Host     *string `json:"host,omitempty"`
	Port     *int64  `json:"port,omitempty"`
	Username *string `json:"username,omitempty"`
	Password *string `json:"password,omitempty"`
	Default  *bool   `json:"default,omitempty"`
}

func relaysPath(nodeID string) string { return fmt.Sprintf("/v1/nodes/%s/smtp-relays", nodeID) }

// CreateRelay adds a relay to the node; the response is the only one with the password.
func (c *Client) CreateRelay(nodeID string, req SMTPRelayCreate) (*SMTPRelay, error) {
	var r SMTPRelay
	if err := c.Post(relaysPath(nodeID), req, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) GetRelay(nodeID, id string) (*SMTPRelay, error) {
	var r SMTPRelay
	if err := c.Get(relaysPath(nodeID)+"/"+id, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) UpdateRelay(nodeID, id string, req SMTPRelayPatch) (*SMTPRelay, error) {
	var r SMTPRelay
	if err := c.Patch(relaysPath(nodeID)+"/"+id, req, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// DeleteRelay removes a relay; apicpd answers 409 while mail domains use it.
func (c *Client) DeleteRelay(nodeID, id string) error {
	return c.Delete(relaysPath(nodeID) + "/" + id)
}
