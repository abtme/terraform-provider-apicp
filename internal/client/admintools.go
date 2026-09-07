package client

// AdminToolStatus mirrors apicp's internal/phpadmin.Status JSON shape
// (PLAN.md §8 phase 3).
type AdminToolStatus struct {
	Tool      string `json:"tool"`
	Enabled   bool   `json:"enabled"`
	URL       string `json:"url,omitempty"`
	UpdatedAt string `json:"updated_at"`
}

// EnableAdminTool implements POST /v1/admin-tools/{tool} - takes no
// request body, there is nothing to configure beyond on/off.
func (c *Client) EnableAdminTool(tool string) (*AdminToolStatus, error) {
	var s AdminToolStatus
	if err := c.Post("/v1/admin-tools/"+tool, nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) GetAdminTool(tool string) (*AdminToolStatus, error) {
	var s AdminToolStatus
	if err := c.Get("/v1/admin-tools/"+tool, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) DisableAdminTool(tool string) error {
	return c.Delete("/v1/admin-tools/" + tool)
}
