package client

// TLSConfig mirrors apicp's GET/PUT /v1/system/tls-config JSON shape.
// ListenTLSEnabled/ListenTLSAddr are read-only through the API (set at
// apicpd startup); only the ACME directory URL can be changed.
type TLSConfig struct {
	ACMEDirectoryURL string `json:"acme_directory_url"`
	ListenTLSEnabled bool   `json:"listen_tls_enabled"`
	ListenTLSAddr    string `json:"listen_tls_addr"`
}

type tlsConfigRequest struct {
	ACMEDirectoryURL string `json:"acme_directory_url"`
}

func (c *Client) GetTLSConfig() (*TLSConfig, error) {
	var cfg TLSConfig
	if err := c.Get("/v1/system/tls-config", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Client) SetTLSConfig(acmeDirectoryURL string) (*TLSConfig, error) {
	var cfg TLSConfig
	if err := c.Put("/v1/system/tls-config", tlsConfigRequest{ACMEDirectoryURL: acmeDirectoryURL}, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
