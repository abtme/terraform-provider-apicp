package client

// PackageLimits mirrors apicp's internal/respkg.Limits JSON shape (PLAN.md
// §8 phase 1) - count-based resource caps.
type PackageLimits struct {
	MaxWebDomains  int `json:"max_web_domains"`
	MaxDatabases   int `json:"max_databases"`
	MaxMailDomains int `json:"max_mail_domains"`
	MaxMailboxes   int `json:"max_mailboxes"`
	MaxCronJobs    int `json:"max_cron_jobs"`
	MaxSubAccounts int `json:"max_sub_accounts,omitempty"`
}

// Package mirrors apicp's internal/respkg.Package JSON shape.
type Package struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	OwnerAccountID string        `json:"owner_account_id"`
	ForTier        string        `json:"for_tier"`
	Limits         PackageLimits `json:"limits"`
}

type packageCreateRequest struct {
	Name    string        `json:"name"`
	ForTier string        `json:"for_tier"`
	Limits  PackageLimits `json:"limits"`
}

func (c *Client) CreatePackage(name, forTier string, limits PackageLimits) (*Package, error) {
	var p Package
	if err := c.Post("/v1/packages", packageCreateRequest{Name: name, ForTier: forTier, Limits: limits}, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *Client) GetPackage(id string) (*Package, error) {
	var p Package
	if err := c.Get("/v1/packages/"+id, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *Client) DeletePackage(id string) error {
	return c.Delete("/v1/packages/" + id)
}
