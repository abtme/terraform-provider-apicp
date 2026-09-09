package client

// FirewallConfig mirrors apicp's internal/firewall.Config JSON shape
// (PLAN.md §8 phase 7) - the enable/disable toggle. Enforcement off
// (the default, matching every existing install's current state) means
// apicp manages no firewall rules at all.
type FirewallConfig struct {
	Enabled   bool   `json:"enabled"`
	UpdatedAt string `json:"updated_at"`
}

// EnableFirewall implements POST /v1/firewall - installs the
// default-DROP allowlist chain with every currently-stored rule already
// applied. Takes no request body, nothing to configure beyond on/off.
func (c *Client) EnableFirewall() (*FirewallConfig, error) {
	var cfg FirewallConfig
	if err := c.Post("/v1/firewall", nil, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Client) GetFirewallConfig() (*FirewallConfig, error) {
	var cfg FirewallConfig
	if err := c.Get("/v1/firewall", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// DisableFirewall implements DELETE /v1/firewall - removes the chain
// entirely, the node ends up exactly as open as before this feature was
// ever used. Stored rules are left alone server-side (re-enabling
// reapplies them).
func (c *Client) DisableFirewall() error {
	return c.Delete("/v1/firewall")
}

// FirewallRule mirrors apicp's internal/firewall.Rule JSON shape.
type FirewallRule struct {
	ID        string `json:"id"`
	Source    string `json:"source,omitempty"` // CIDR; "" = anywhere
	Port      string `json:"port,omitempty"`   // "22", "1000:2000"; "" = all ports
	Protocol  string `json:"protocol"`         // tcp, udp, all
	Action    string `json:"action"`           // accept, drop, reject
	Comment   string `json:"comment,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type firewallRuleCreateRequest struct {
	Source   string `json:"source,omitempty"`
	Port     string `json:"port,omitempty"`
	Protocol string `json:"protocol"`
	Action   string `json:"action"`
	Comment  string `json:"comment,omitempty"`
}

// CreateFirewallRule implements POST /v1/firewall/rules. No update
// endpoint exists server-side - every field change is a replace.
func (c *Client) CreateFirewallRule(source, port, protocol, action, comment string) (*FirewallRule, error) {
	var r FirewallRule
	req := firewallRuleCreateRequest{Source: source, Port: port, Protocol: protocol, Action: action, Comment: comment}
	if err := c.Post("/v1/firewall/rules", req, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) GetFirewallRule(id string) (*FirewallRule, error) {
	var r FirewallRule
	if err := c.Get("/v1/firewall/rules/"+id, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) DeleteFirewallRule(id string) error {
	return c.Delete("/v1/firewall/rules/" + id)
}
