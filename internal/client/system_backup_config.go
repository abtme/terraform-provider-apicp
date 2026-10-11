package client

// SystemBackupConfig mirrors apicp's internal/sysbackup.Config JSON shape
// (PLAN.md §8 phase 5) - only enabled/destination are settable through
// this API; the backup mechanics themselves (what's archived, on what
// schedule) are never exposed here by design.
type SystemBackupConfig struct {
	Enabled      bool     `json:"enabled"`
	Destination  string   `json:"destination"`
	Retention    int      `json:"retention"`
	Format       string   `json:"format,omitempty"`
	Compress     bool     `json:"compress"`
	Encrypt      bool     `json:"encrypt"`
	ScheduleTime string   `json:"schedule_time,omitempty"`
	ScheduleDays []string `json:"schedule_days,omitempty"`
	Timezone     string   `json:"timezone,omitempty"`
	NextRunAt    string   `json:"next_run_at,omitempty"`
	UpdatedAt    string   `json:"updated_at"`
	LastRunAt    string   `json:"last_run_at,omitempty"`
	LastRunError string   `json:"last_run_error,omitempty"`
}

// SystemBackupConfigInput is what PUT /v1/system/backup-config takes. The
// server replaces the whole config, so send every setting you want kept.
// Compress and Encrypt are pointers: nil leaves the server's default
// (compression on; encryption on whenever the server has a key).
type SystemBackupConfigInput struct {
	Enabled      bool     `json:"enabled"`
	Destination  string   `json:"destination"`
	Retention    int      `json:"retention"`
	Format       string   `json:"format,omitempty"`
	Compress     *bool    `json:"compress,omitempty"`
	Encrypt      *bool    `json:"encrypt,omitempty"`
	ScheduleTime string   `json:"schedule_time,omitempty"`
	ScheduleDays []string `json:"schedule_days,omitempty"`
	Timezone     string   `json:"timezone,omitempty"`
}

func (c *Client) GetSystemBackupConfig() (*SystemBackupConfig, error) {
	var cfg SystemBackupConfig
	if err := c.Get("/v1/system/backup-config", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Client) SetSystemBackupConfig(in SystemBackupConfigInput) (*SystemBackupConfig, error) {
	var cfg SystemBackupConfig
	if err := c.Put("/v1/system/backup-config", in, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
