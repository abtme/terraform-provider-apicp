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
	ID       string `json:"id"`
	NodeID   string `json:"node_id"`
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
