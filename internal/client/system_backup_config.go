package client

import (
	"errors"
	"net/http"
)

// SystemBackupConfig mirrors GET /v1/system/backup-config. Format, Compress,
// Encrypt, BackupUnit and Verify are the EFFECTIVE values (what the next run
// does), not what is saved: a saved "use the default" shows as the default's
// value. So never send these back as they were read - that saves the default
// of the moment as an explicit choice (how a server that later got an
// encryption key kept writing plain backups).
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

	// From apicpd with per-domain backups on (empty / nil before).
	BackupUnit          string   `json:"backup_unit,omitempty"`
	Verify              *bool    `json:"verify,omitempty"`
	EncryptionAvailable bool     `json:"encryption_available"`
	EncryptionWarning   string   `json:"encryption_warning,omitempty"`
	LastRunWarnings     []string `json:"last_run_warnings,omitempty"`
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

// ErrPatchUnsupported is PatchSystemBackupConfig's answer from an apicpd
// without PATCH /v1/system/backup-config (before per-domain backups): the
// caller falls back to PUT, which replaces the whole config.
var ErrPatchUnsupported = errors.New("apicp: this apicpd has no PATCH /v1/system/backup-config")

func (c *Client) GetSystemBackupConfig() (*SystemBackupConfig, error) {
	var cfg SystemBackupConfig
	if err := c.Get("/v1/system/backup-config", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SetSystemBackupConfig replaces the whole config (PUT).
func (c *Client) SetSystemBackupConfig(in SystemBackupConfigInput) (*SystemBackupConfig, error) {
	var cfg SystemBackupConfig
	if err := c.Put("/v1/system/backup-config", in, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// PatchSystemBackupConfig changes only the settings in patch (PATCH): a key
// left out keeps its saved value, and a nil value puts encrypt, compress or
// verify back to the server's default. ErrPatchUnsupported from an apicpd
// that has no PATCH (405, or 404).
func (c *Client) PatchSystemBackupConfig(patch map[string]any) (*SystemBackupConfig, error) {
	var cfg SystemBackupConfig
	err := c.Patch("/v1/system/backup-config", patch, &cfg)
	var ae *APIError
	if errors.Is(err, ErrNotFound) || (errors.As(err, &ae) && ae.StatusCode == http.StatusMethodNotAllowed) {
		return nil, ErrPatchUnsupported
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}
