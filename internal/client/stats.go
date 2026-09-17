package client

import "fmt"

// AccountStats mirrors apicp's internal/api.AccountStats JSON shape
// (GET /v1/accounts/{id}/stats) - a plain resource-count tally, nothing
// apicp lets you configure, hence a data source rather than a resource.
type AccountStats struct {
	WebDomains  int `json:"web_domains"`
	Databases   int `json:"databases"`
	MailDomains int `json:"mail_domains"`
	Mailboxes   int `json:"mailboxes"`
	CronJobs    int `json:"cron_jobs"`
	SubAccounts int `json:"sub_accounts"`
}

func (c *Client) GetAccountStats(accountID string) (*AccountStats, error) {
	var s AccountStats
	if err := c.Get(fmt.Sprintf("/v1/accounts/%s/stats", accountID), &s); err != nil {
		return nil, err
	}
	return &s, nil
}
