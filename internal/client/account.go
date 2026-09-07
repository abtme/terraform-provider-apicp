package client

// Account mirrors apicp's internal/account.Account JSON shape (PLAN.md §8
// phase 1).
type Account struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Tier            string `json:"tier"`
	ParentAccountID string `json:"parent_account_id,omitempty"`
	PackageID       string `json:"package_id,omitempty"`
	CreatedAt       string `json:"created_at"`
}

type accountCreateRequest struct {
	Name            string `json:"name"`
	Tier            string `json:"tier"`
	ParentAccountID string `json:"parent_account_id,omitempty"`
	PackageID       string `json:"package_id,omitempty"`
}

type accountPatchRequest struct {
	Name      *string `json:"name,omitempty"`
	PackageID *string `json:"package_id,omitempty"`
}

func (c *Client) CreateAccount(name, tier, parentAccountID, packageID string) (*Account, error) {
	var a Account
	if err := c.Post("/v1/accounts", accountCreateRequest{Name: name, Tier: tier, ParentAccountID: parentAccountID, PackageID: packageID}, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func (c *Client) GetAccount(id string) (*Account, error) {
	var a Account
	if err := c.Get("/v1/accounts/"+id, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// PatchAccount updates name and/or package_id - nil pointers leave that
// field unchanged (Tier/ParentAccountID are never patchable, see
// account.PatchRequest's doc comment).
func (c *Client) PatchAccount(id string, name, packageID *string) (*Account, error) {
	var a Account
	if err := c.Patch("/v1/accounts/"+id, accountPatchRequest{Name: name, PackageID: packageID}, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func (c *Client) DeleteAccount(id string) error {
	return c.Delete("/v1/accounts/" + id)
}

type accountTokenCreateRequest struct {
	Label  string   `json:"label"`
	Scopes []string `json:"scopes"`
}

type AccountToken struct {
	Token string `json:"token"`
}

// CreateAccountToken mints a new bearer token for accountID (PLAN.md §8
// phase 1) - returned exactly once, apicp never stores or re-returns the
// plaintext.
func (c *Client) CreateAccountToken(accountID, label string, scopes []string) (*AccountToken, error) {
	var t AccountToken
	if err := c.Post("/v1/accounts/"+accountID+"/tokens", accountTokenCreateRequest{Label: label, Scopes: scopes}, &t); err != nil {
		return nil, err
	}
	return &t, nil
}
