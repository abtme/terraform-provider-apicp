package client

import (
	"fmt"
	"net/url"
)

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
	ID        string   `json:"id"`
	Source    string   `json:"source,omitempty"` // CIDR; "" = anywhere
	Sources   []string `json:"sources,omitempty"`
	List      string   `json:"list,omitempty"` // an address list or template (accept rules only)
	Port      string   `json:"port,omitempty"` // "22", "1000:2000"; "" = all ports
	Protocol  string   `json:"protocol"`       // tcp, udp, all
	Action    string   `json:"action"`         // accept, drop, reject
	Comment   string   `json:"comment,omitempty"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

type firewallRuleCreateRequest struct {
	Source   string   `json:"source,omitempty"`
	Sources  []string `json:"sources,omitempty"`
	List     string   `json:"list,omitempty"`
	Port     string   `json:"port,omitempty"`
	Protocol string   `json:"protocol"`
	Action   string   `json:"action"`
	Comment  string   `json:"comment,omitempty"`
}

// CreateFirewallRule implements POST /v1/firewall/rules. No update
// endpoint exists server-side - every field change is a replace. list names
// an address list or template ("" for none; accept rules only).
func (c *Client) CreateFirewallRule(source string, sources []string, list, port, protocol, action, comment string) (*FirewallRule, error) {
	var r FirewallRule
	req := firewallRuleCreateRequest{Source: source, Sources: sources, List: list, Port: port, Protocol: protocol, Action: action, Comment: comment}
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

// FirewallRestriction mirrors apicp's internal/firewall.Restriction JSON
// shape: a baseline service port limited to a list of source CIDRs.
type FirewallRestriction struct {
	Port      int      `json:"port"`
	Sources   []string `json:"sources"`
	Lists     []string `json:"lists,omitempty"`
	Comment   string   `json:"comment,omitempty"`
	Force     bool     `json:"force,omitempty"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

type firewallRestrictionRequest struct {
	Sources []string `json:"sources"`
	Lists   []string `json:"lists,omitempty"`
	Comment string   `json:"comment,omitempty"`
	Force   bool     `json:"force,omitempty"`
}

// SetFirewallRestriction implements PUT /v1/firewall/restrictions/{port} -
// creates or replaces (idempotent), so it serves both create and update.
// lists names address lists or templates whose addresses also reach the port.
func (c *Client) SetFirewallRestriction(port int, sources, lists []string, comment string, force bool) (*FirewallRestriction, error) {
	var r FirewallRestriction
	if sources == nil {
		sources = []string{}
	}
	req := firewallRestrictionRequest{Sources: sources, Lists: lists, Comment: comment, Force: force}
	if err := c.Put(fmt.Sprintf("/v1/firewall/restrictions/%d", port), req, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) GetFirewallRestriction(port int) (*FirewallRestriction, error) {
	var r FirewallRestriction
	if err := c.Get(fmt.Sprintf("/v1/firewall/restrictions/%d", port), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) DeleteFirewallRestriction(port int) error {
	return c.Delete(fmt.Sprintf("/v1/firewall/restrictions/%d", port))
}

// FirewallSite mirrors apicp's internal/firewall.Site JSON shape: a connection on
// a dynamic address, kept as a DNS name that follows it.
type FirewallSite struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Host      string   `json:"host"`
	Enabled   bool     `json:"enabled"`
	Ports     []int    `json:"ports,omitempty"`
	Addresses []string `json:"addresses"`
	Changed   string   `json:"changed,omitempty"`
}

type FirewallSiteInput struct {
	Name    string `json:"name"`
	Host    string `json:"host"`
	Enabled *bool  `json:"enabled,omitempty"`
	Ports   []int  `json:"ports,omitempty"`
}

type FirewallSitePatch struct {
	Name    *string `json:"name,omitempty"`
	Host    *string `json:"host,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
	Ports   *[]int  `json:"ports,omitempty"`
}

func (c *Client) CreateFirewallSite(in FirewallSiteInput) (*FirewallSite, error) {
	var s FirewallSite
	if err := c.Post("/v1/firewall/sites", in, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) GetFirewallSite(id string) (*FirewallSite, error) {
	var s FirewallSite
	if err := c.Get("/v1/firewall/sites/"+id, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) UpdateFirewallSite(id string, p FirewallSitePatch) (*FirewallSite, error) {
	var s FirewallSite
	if err := c.Patch("/v1/firewall/sites/"+id, p, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) DeleteFirewallSite(id string) error {
	return c.Delete("/v1/firewall/sites/" + id)
}

// FirewallList mirrors apicp's firewall address list (GET /v1/firewall/lists/{id}):
// a named set of addresses used by accept rules (list) and restrictions (lists).
// Built-in templates come back the same way with Builtin set.
type FirewallList struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Comment   string   `json:"comment,omitempty"`
	Sources   []string `json:"sources"`
	Template  string   `json:"template,omitempty"`
	Builtin   bool     `json:"builtin,omitempty"`
	Addresses []string `json:"addresses"`
	Problem   string   `json:"problem,omitempty"`
	UsedBy    []string `json:"used_by"`
}

type FirewallListInput struct {
	Name    string   `json:"name"`
	Sources []string `json:"sources"`
	Comment string   `json:"comment,omitempty"`
}

type FirewallListPatch struct {
	Name    *string   `json:"name,omitempty"`
	Sources *[]string `json:"sources,omitempty"`
	Comment *string   `json:"comment,omitempty"`
}

func (c *Client) CreateFirewallList(in FirewallListInput) (*FirewallList, error) {
	var l FirewallList
	if err := c.Post("/v1/firewall/lists", in, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// GetFirewallList takes the list's id or name (or a template's id).
func (c *Client) GetFirewallList(id string) (*FirewallList, error) {
	var l FirewallList
	if err := c.Get("/v1/firewall/lists/"+url.PathEscape(id), &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// UpdateFirewallList changes a list; apicp re-applies the firewall when a rule
// or restriction uses it, and refuses a rename while it is in use.
func (c *Client) UpdateFirewallList(id string, p FirewallListPatch) (*FirewallList, error) {
	var l FirewallList
	if err := c.Patch("/v1/firewall/lists/"+url.PathEscape(id), p, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// DeleteFirewallList removes a list; apicp refuses (409) while a rule or
// restriction uses it.
func (c *Client) DeleteFirewallList(id string) error {
	return c.Delete("/v1/firewall/lists/" + url.PathEscape(id))
}
