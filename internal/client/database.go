package client

// Database mirrors apicp's internal/database.Database JSON shape, plus
// the one-time Password field createDatabaseResponse adds on create only.
type Database struct {
	ID       string `json:"id"`
	Engine   string `json:"engine"`
	Name     string `json:"name"`
	Username string `json:"username"`
	NodeID   string `json:"node_id"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
	Password string `json:"password,omitempty"` // only ever populated by CreateDatabase's response
}

type databaseCreateRequest struct {
	Engine string `json:"engine"`
	Name   string `json:"name"`
}

func (c *Client) CreateDatabase(engine, name string) (*Database, error) {
	var d Database
	if err := c.Post("/v1/databases", databaseCreateRequest{Engine: engine, Name: name}, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (c *Client) GetDatabase(id string) (*Database, error) {
	var d Database
	if err := c.Get("/v1/databases/"+id, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (c *Client) DeleteDatabase(id string) error {
	return c.Delete("/v1/databases/" + id)
}
