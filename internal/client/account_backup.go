package client

import "fmt"

// AccountBackup mirrors apicp's backup.Meta JSON shape.
type AccountBackup struct {
	ID        string `json:"id"`
	AccountID string `json:"account_id"`
	CreatedAt string `json:"created_at"`
	SizeBytes int64  `json:"size_bytes"`
}

func (c *Client) CreateAccountBackup(accountID string) (*AccountBackup, error) {
	var b AccountBackup
	if err := c.Post(fmt.Sprintf("/v1/accounts/%s/backups", accountID), nil, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// GetAccountBackup finds one backup in the account's list - apicp has no
// single-backup GET. Returns ErrNotFound if the account or backup is gone.
func (c *Client) GetAccountBackup(accountID, id string) (*AccountBackup, error) {
	var out struct {
		Backups []AccountBackup `json:"backups"`
	}
	if err := c.Get(fmt.Sprintf("/v1/accounts/%s/backups", accountID), &out); err != nil {
		return nil, err
	}
	for i := range out.Backups {
		if out.Backups[i].ID == id {
			return &out.Backups[i], nil
		}
	}
	return nil, ErrNotFound
}

func (c *Client) DeleteAccountBackup(accountID, id string) error {
	return c.Delete(fmt.Sprintf("/v1/accounts/%s/backups/%s", accountID, id))
}
