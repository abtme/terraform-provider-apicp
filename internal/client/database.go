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
	// VhostID is the website the database is linked to ("" = none). apicpd
	// before database links never sends it.
	VhostID  string `json:"vhost_id,omitempty"`
	Password string `json:"password,omitempty"` // only ever populated by CreateDatabase's response
}

type databaseCreateRequest struct {
	Engine  string `json:"engine"`
	Name    string `json:"name"`
	VhostID string `json:"vhost_id,omitempty"`
}

// CreateDatabase makes a database, linked to the website vhostID when that
// is not "". An apicpd without database links ignores vhostID: compare the
// answer's VhostID.
func (c *Client) CreateDatabase(engine, name, vhostID string) (*Database, error) {
	var d Database
	if err := c.Post("/v1/databases", databaseCreateRequest{Engine: engine, Name: name, VhostID: vhostID}, &d); err != nil {
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

// SetDatabaseVhost links the database to the website vhostID, or unlinks it
// when vhostID is "" (PATCH /v1/databases/{id}; nothing on the database
// server changes). An apicpd without database links answers 405.
func (c *Client) SetDatabaseVhost(id, vhostID string) (*Database, error) {
	var d Database
	if err := c.Patch("/v1/databases/"+id, map[string]string{"vhost_id": vhostID}, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (c *Client) DeleteDatabase(id string) error {
	return c.Delete("/v1/databases/" + id)
}
