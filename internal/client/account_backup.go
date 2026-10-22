package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// AccountBackup mirrors apicp's backup.Meta JSON shape.
type AccountBackup struct {
	ID        string `json:"id"`
	AccountID string `json:"account_id"`
	CreatedAt string `json:"created_at"`
	SizeBytes int64  `json:"size_bytes"`
}

// Account backup run states (apicp's backup.AccountRun.Status).
const (
	AccountBackupRunRunning  = "running"
	AccountBackupRunFinished = "finished"
	AccountBackupRunFailed   = "failed"
)

// AccountBackupRun mirrors apicp's backup.AccountRun: one run of
// POST /v1/accounts/{id}/backups, which since the account-backup-process
// change makes the backup in a process of its own and answers at once.
type AccountBackupRun struct {
	RunID       string `json:"run_id"`
	AccountID   string `json:"account_id"`
	Status      string `json:"status"`
	Stage       string `json:"stage"`
	RequestedAt string `json:"requested_at"`
	StartedAt   string `json:"started_at"`
	FinishedAt  string `json:"finished_at"`
	PID         int    `json:"pid"`
	Unit        string `json:"unit"`
	Error       string `json:"error"`
	// Warning: the backup was made, but something after it was not (old
	// one-off backups of the account could not be removed).
	Warning string         `json:"warning"`
	Backup  *AccountBackup `json:"backup"`
}

// AccountBackupResult is what CreateAccountBackup made.
type AccountBackupResult struct {
	Backup *AccountBackup
	// RunID is the run that made it; empty from an apicpd that still makes
	// the backup within the request.
	RunID string
	// Warning is the run's warning, if any (the backup itself is fine).
	Warning string
}

// AccountBackupPollInterval is how often CreateAccountBackup asks how a run
// is going.
var AccountBackupPollInterval = 3 * time.Second

func accountBackupsPath(accountID string) string {
	return "/v1/accounts/" + url.PathEscape(accountID) + "/backups"
}

// CreateAccountBackup takes one backup of an account and returns it once it
// is made, or ctx ends.
//
// apicpd from the account-backup-process change on answers POST at once:
// 202 with a new run, or 200 with the run already going for the account (one
// at a time per account). The run is polled until it is finished (its
// backup is the result) or failed (its error is returned). A 200 run was
// started by someone else - the CLI, the panel, another resource - so its
// backup is not ours to keep in state (destroying this resource would delete
// it); CreateAccountBackup waits for that run to end, whatever its result,
// and then starts its own.
//
// Older apicpd made the backup within the request and answered 201 with the
// backup itself (no run_id); that answer is returned as it is.
//
// If ctx ends while a run is going, the run carries on on the server and the
// error says which run it was. Passing problems while polling (connection
// refused or a 5xx while apicpd restarts - the run is a process of its own
// and outlives that) are retried until ctx ends.
func (c *Client) CreateAccountBackup(ctx context.Context, accountID string) (*AccountBackupResult, error) {
	for {
		var raw json.RawMessage
		code, err := c.requestCtx(ctx, http.MethodPost, accountBackupsPath(accountID), nil, nil, &raw)
		if err != nil {
			return nil, err
		}

		var probe struct {
			RunID string `json:"run_id"`
			ID    string `json:"id"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			return nil, fmt.Errorf("decoding the backup answer: %w (body: %s)", err, raw)
		}

		if probe.RunID == "" {
			// an apicpd that makes the backup within the request (201 + the backup)
			if probe.ID == "" {
				return nil, fmt.Errorf("apicp answered %d with neither a backup run nor a backup: %s", code, raw)
			}
			var b AccountBackup
			if err := json.Unmarshal(raw, &b); err != nil {
				return nil, fmt.Errorf("decoding the backup: %w (body: %s)", err, raw)
			}
			return &AccountBackupResult{Backup: &b}, nil
		}

		var run AccountBackupRun
		if err := json.Unmarshal(raw, &run); err != nil {
			return nil, fmt.Errorf("decoding the backup run: %w (body: %s)", err, raw)
		}

		end, err := c.WaitAccountBackupRun(ctx, accountID, run)
		if err != nil {
			if code == http.StatusOK {
				return nil, fmt.Errorf("waiting for another backup of account %s to end before starting this one: %w", accountID, err)
			}
			return nil, err
		}

		if code == http.StatusOK {
			// someone else's run: it has ended, so this account is free -
			// give it a moment, then start ours
			if err := sleepCtx(ctx, AccountBackupPollInterval); err != nil {
				return nil, fmt.Errorf("gave up before starting the backup (another backup of account %s, run %s, was going first): %w", accountID, run.RunID, err)
			}
			continue
		}

		if end.Status == AccountBackupRunFailed {
			return nil, fmt.Errorf("the backup failed (run %s): %s", end.RunID, end.Error)
		}
		if end.Backup == nil || end.Backup.ID == "" {
			return nil, fmt.Errorf("backup run %s finished without saying which backup it made", end.RunID)
		}
		return &AccountBackupResult{Backup: end.Backup, RunID: end.RunID, Warning: end.Warning}, nil
	}
}

// WaitAccountBackupRun polls a run (GET /v1/accounts/{id}/backups/runs/{run})
// until it is finished or failed and returns it in that state; a failed run
// is not an error here. It returns an error when ctx ends first (the run
// carries on on the server), the run is gone (404) or apicpd refuses the
// request; connection errors and 5xx answers are retried until ctx ends.
func (c *Client) WaitAccountBackupRun(ctx context.Context, accountID string, run AccountBackupRun) (*AccountBackupRun, error) {
	path := accountBackupsPath(accountID) + "/runs/" + url.PathEscape(run.RunID)

	var lastErr error
	for {
		switch run.Status {
		case AccountBackupRunFinished, AccountBackupRunFailed:
			return &run, nil
		case AccountBackupRunRunning:
		default:
			return nil, fmt.Errorf("backup run %s: unexpected status %q", run.RunID, run.Status)
		}

		if err := sleepCtx(ctx, AccountBackupPollInterval); err != nil {
			msg := fmt.Sprintf("gave up waiting for backup run %s of account %s (stage %q); it carries on on the server", run.RunID, accountID, run.Stage)
			if lastErr != nil {
				msg += fmt.Sprintf(" (last check: %v)", lastErr)
			}
			return nil, fmt.Errorf("%s: %w", msg, err)
		}

		var next AccountBackupRun
		if _, err := c.requestCtx(ctx, http.MethodGet, path, nil, nil, &next); err != nil {
			if ctx.Err() == nil && passing(err) {
				lastErr = err
				continue
			}
			if ctx.Err() != nil {
				continue // the next turn reports giving up
			}
			if errors.Is(err, ErrNotFound) {
				return nil, fmt.Errorf("backup run %s of account %s is gone from the server", run.RunID, accountID)
			}
			return nil, fmt.Errorf("checking backup run %s: %w", run.RunID, err)
		}
		run, lastErr = next, nil
	}
}

// passing reports whether an error is worth asking again for: no answer at
// all (apicpd restarting) or a 5xx.
func passing(err error) bool {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.StatusCode >= 500
	}
	var ue *url.Error
	return errors.As(err, &ue)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// GetAccountBackup finds one backup in the account's list - apicp has no
// single-backup GET. Returns ErrNotFound if the account or backup is gone.
func (c *Client) GetAccountBackup(accountID, id string) (*AccountBackup, error) {
	var out struct {
		Backups []AccountBackup `json:"backups"`
	}
	if err := c.Get(accountBackupsPath(accountID), &out); err != nil {
		return nil, err
	}
	for i := range out.Backups {
		if out.Backups[i].ID == id {
			return &out.Backups[i], nil
		}
	}
	return nil, ErrNotFound
}

func (c *Client) DeleteAccountBackup(accountID, id string) error {
	return c.Delete(fmt.Sprintf("/v1/accounts/%s/backups/%s", accountID, id))
}
