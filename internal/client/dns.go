package client

import "fmt"

// Zone mirrors apicp's internal/dns.Zone JSON shape.
type Zone struct {
	ID     string `json:"id"`
	Name   string `json:"name"` // canonical, trailing dot
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type zoneCreateRequest struct {
	Name string `json:"name"`
}

func (c *Client) CreateZone(name string) (*Zone, error) {
	var z Zone
	if err := c.Post("/v1/dns/zones", zoneCreateRequest{Name: name}, &z); err != nil {
		return nil, err
	}
	return &z, nil
}

func (c *Client) GetZone(id string) (*Zone, error) {
	var z Zone
	if err := c.Get("/v1/dns/zones/"+id, &z); err != nil {
		return nil, err
	}
	return &z, nil
}

func (c *Client) DeleteZone(id string) error {
	return c.Delete("/v1/dns/zones/" + id)
}

// Record mirrors apicp's internal/dns.Record JSON shape.
type Record struct {
	ID      string `json:"id"`
	ZoneID  string `json:"zone_id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
}

type recordCreateRequest struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	TTL     int    `json:"ttl,omitempty"`
}

type recordPatchRequest struct {
	Content *string `json:"content,omitempty"`
	TTL     *int    `json:"ttl,omitempty"`
}

func (c *Client) CreateRecord(zoneID, name, recordType, content string, ttl int) (*Record, error) {
	var r Record
	req := recordCreateRequest{Name: name, Type: recordType, Content: content, TTL: ttl}
	if err := c.Post(fmt.Sprintf("/v1/dns/zones/%s/records", zoneID), req, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) GetRecord(zoneID, id string) (*Record, error) {
	var r Record
	if err := c.Get(fmt.Sprintf("/v1/dns/zones/%s/records/%s", zoneID, id), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) UpdateRecord(zoneID, id string, content string, ttl int) (*Record, error) {
	var r Record
	req := recordPatchRequest{Content: &content, TTL: &ttl}
	if err := c.Patch(fmt.Sprintf("/v1/dns/zones/%s/records/%s", zoneID, id), req, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) DeleteRecord(zoneID, id string) error {
	return c.Delete(fmt.Sprintf("/v1/dns/zones/%s/records/%s", zoneID, id))
}
