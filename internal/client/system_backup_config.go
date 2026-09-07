package client

// SystemBackupConfig mirrors apicp's internal/sysbackup.Config JSON shape
// (PLAN.md §8 phase 5) - only enabled/destination are settable through
// this API; the backup mechanics themselves (what's archived, on what
// schedule) are never exposed here by design.
type SystemBackupConfig struct {
	Enabled      bool   `json:"enabled"`
	Destination  string `json:"destination"`
	UpdatedAt    string `json:"updated_at"`
	LastRunAt    string `json:"last_run_at,omitempty"`
	LastRunError string `json:"last_run_error,omitempty"`
}

type systemBackupConfigRequest struct {
	Enabled     bool   `json:"enabled"`
	Destination string `json:"destination"`
}

func (c *Client) GetSystemBackupConfig() (*SystemBackupConfig, error) {
	var cfg SystemBackupConfig
	if err := c.Get("/v1/system/backup-config", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Client) SetSystemBackupConfig(enabled bool, destination string) (*SystemBackupConfig, error) {
	var cfg SystemBackupConfig
	req := systemBackupConfigRequest{Enabled: enabled, Destination: destination}
	if err := c.Put("/v1/system/backup-config", req, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
