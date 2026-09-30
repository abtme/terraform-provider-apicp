package client

// Node mirrors apicp's node.Node JSON shape.
type Node struct {
	ID           string   `json:"id"`
	Hostname     string   `json:"hostname"`
	PublicHost   string   `json:"public_host,omitempty"`
	Roles        []string `json:"roles"`
	Status       string   `json:"status"`
	Connected    bool     `json:"connected"`
	AgentVersion string   `json:"agent_version,omitempty"`
	LastSeenAt   string   `json:"last_seen_at,omitempty"`
	EnrolledAt   string   `json:"enrolled_at"`
}

func (c *Client) ListNodes() ([]Node, error) {
	var out struct {
		Nodes []Node `json:"nodes"`
	}
	if err := c.Get("/v1/nodes", &out); err != nil {
		return nil, err
	}
	return out.Nodes, nil
}
