package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// AccountBackup mirrors apicp's backup.Meta JSON shape: one archive.
type AccountBackup struct {
	ID        string `json:"id"`
	AccountID string `json:"account_id"`
	CreatedAt string `json:"created_at"`
	SizeBytes int64  `json:"size_bytes"`
	// Scope is the part of the account the archive holds: "account" (its
	// settings), "domain-web" (one website), "domain-mail" (one mail domain),
	// or "" for a whole-account archive (apicpd before per-domain backups, or
	// backup_unit "account").
	Scope string `json:"scope,omitempty"`
	// Domain is the website or mail domain of a domain-web/domain-mail archive.
	Domain string `json:"domain,omitempty"`
	// File is the archive's file relative to the backup folder; Path is the
	// same without the extension (older apicpd sends only Path, or neither).
	File string `json:"file,omitempty"`
	Path string `json:"path,omitempty"`
	// Verified is the restore check's result for this archive; nil when it
	// was not checked (verify off, an archive from before apicp v0.7.0, or an
	// apicpd that does not report it). VerifyError is why the check failed.
	Verified    *bool    `json:"verified,omitempty"`
	VerifyError string   `json:"verify_error,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
	// BatchID is what made the archive (apicp v0.7.0 on): "arun_..." for one
	// "back up now" run (the AccountBackupRun's RunID, shared by every part
	// it made), "run_..." for a scheduled run. Empty on archives made before
	// 0.7.0 and from older apicpd.
	BatchID string `json:"batch_id,omitempty"`
	// Incomplete: the batch did not finish (a run still going, killed, or one
	// that failed and could not delete this part). apicp refuses to restore it.
	Incomplete bool `json:"incomplete,omitempty"`
}

// Name is the archive's file name for people: File, else Path, else "".
func (b *AccountBackup) Name() string {
	if b.File != "" {
		return b.File
	}
	return b.Path
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
	// Backups is every archive the run made (per-domain backups: the
	// account's settings archive, then one per website and per mail domain).
	// Empty from an apicpd that makes one archive per account.
	Backups []AccountBackup `json:"backups"`
	// Warnings are what the archives could not capture in full
	// ("<file>: <warning>"); the backup still succeeded.
	Warnings []string `json:"warnings"`
	// Removed are the archives a failed run had made and then deleted
	// (apicp v0.7.0 on), so no part of an unfinished backup is left to
	// restore. Parts it could not delete stay in Backups, marked incomplete.
	Removed []string `json:"removed"`
}

// restoreCheckPrefix starts the error of a run whose archives were all
// made but failed apicp's restore check (they stay on the server).
const restoreCheckPrefix = "restore check:"

// AccountBackupFailedError is CreateAccountBackup's error for a run that
// ended failed. Run is that run as it ended: Run.Archives() is what it left
// on the server (nothing, on apicp v0.7.0, when it deleted what it had made).
type AccountBackupFailedError struct {
	Run *AccountBackupRun
}

// RestoreCheckFailed reports whether every archive was made and only the
// restore check failed: the archives stay on the server, those that failed
// with Verified false.
func (e *AccountBackupFailedError) RestoreCheckFailed() bool {
	return strings.HasPrefix(e.Run.Error, restoreCheckPrefix)
}

func archiveLabel(b AccountBackup) string {
	s := b.ID
	if n := b.Name(); n != "" {
		s += " (" + n + ")"
	}
	return s
}

func (e *AccountBackupFailedError) Error() string {
	msg := fmt.Sprintf("the backup failed (run %s): %s", e.Run.RunID, e.Run.Error)
	left := e.Run.Archives()
	if len(left) == 0 {
		return msg
	}
	var labels []string
	switch {
	case e.RestoreCheckFailed():
		for _, b := range left {
			if b.Verified != nil && !*b.Verified {
				l := archiveLabel(b)
				if b.VerifyError != "" {
					l += ": " + b.VerifyError
				}
				labels = append(labels, l)
			}
		}
		return msg + fmt.Sprintf("; its %d archive(s) stay on the server; failing the check: %s", len(left), strings.Join(labels, "; "))
	case len(e.Run.Removed) > 0 || anyIncomplete(left):
		for _, b := range left {
			labels = append(labels, archiveLabel(b))
		}
		return msg + fmt.Sprintf("; %d archive(s) could not be deleted and stay on the server, marked incomplete (apicp does not restore them): %s", len(left), strings.Join(labels, ", "))
	default:
		// an apicpd from before v0.7.0 keeps what a failed run made
		for _, b := range left {
			labels = append(labels, archiveLabel(b))
		}
		return msg + fmt.Sprintf("; the %d archive(s) it did make stay on the server: %s", len(left), strings.Join(labels, ", "))
	}
}

func anyIncomplete(bs []AccountBackup) bool {
	for _, b := range bs {
		if b.Incomplete {
			return true
		}
	}
	return false
}

// Archives is every archive the run made: Backups, or Backup alone from an
// apicpd that makes one archive per account.
func (r *AccountBackupRun) Archives() []AccountBackup {
	if len(r.Backups) > 0 {
		return r.Backups
	}
	if r.Backup != nil && r.Backup.ID != "" {
		return []AccountBackup{*r.Backup}
	}
	return nil
}

// AccountBackupResult is what CreateAccountBackup made.
type AccountBackupResult struct {
	// Backup is the first archive made (the account's settings archive with
	// per-domain backups, the only one otherwise).
	Backup *AccountBackup
	// Backups is every archive made, Backup first.
	Backups []AccountBackup
	// Warnings are what the archives could not capture in full.
	Warnings []string
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
// backup is the result) or failed (an *AccountBackupFailedError). A 200 run was
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
			return &AccountBackupResult{Backup: &b, Backups: []AccountBackup{b}}, nil
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

		archives := end.Archives()
		if end.Status == AccountBackupRunFailed {
			return nil, &AccountBackupFailedError{Run: end}
		}
		if len(archives) == 0 {
			return nil, fmt.Errorf("backup run %s finished without saying which backup it made", end.RunID)
		}
		first := archives[0]
		if end.Backup != nil && end.Backup.ID != "" {
			first = *end.Backup
		}
		return &AccountBackupResult{Backup: &first, Backups: archives, Warnings: end.Warnings, RunID: end.RunID, Warning: end.Warning}, nil
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

// ListAccountBackups lists every backup of an account (GET
// /v1/accounts/{id}/backups). ErrNotFound if the account is gone.
func (c *Client) ListAccountBackups(accountID string) ([]AccountBackup, error) {
	var out struct {
		Backups []AccountBackup `json:"backups"`
	}
	if err := c.Get(accountBackupsPath(accountID), &out); err != nil {
		return nil, err
	}
	return out.Backups, nil
}

// GetAccountBackup finds one backup in the account's list - apicp has no
// single-backup GET. Returns ErrNotFound if the account or backup is gone.
func (c *Client) GetAccountBackup(accountID, id string) (*AccountBackup, error) {
	all, err := c.ListAccountBackups(accountID)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].ID == id {
			return &all[i], nil
		}
	}
	return nil, ErrNotFound
}

// BatchArchives is the archives of all that carry batchID, the account's
// settings archive first (then in list order). Empty for an apicpd that does
// not report batch_id.
func BatchArchives(all []AccountBackup, batchID string) []AccountBackup {
	var out []AccountBackup
	for _, b := range all {
		if batchID != "" && b.BatchID == batchID {
			out = append(out, b)
		}
	}
	SettingsFirst(out)
	return out
}

// SettingsFirst puts the account's settings archive (scope "account") first,
// keeping the order of the rest.
func SettingsFirst(bs []AccountBackup) {
	sort.SliceStable(bs, func(i, j int) bool { return bs[i].Scope == "account" && bs[j].Scope != "account" })
}

// GetAccountBackupRun reads one run (GET
// /v1/accounts/{id}/backups/runs/{run}). apicpd keeps an account's latest 10
// finished runs; ErrNotFound for an older or unknown one.
func (c *Client) GetAccountBackupRun(accountID, runID string) (*AccountBackupRun, error) {
	var run AccountBackupRun
	if err := c.Get(accountBackupsPath(accountID)+"/runs/"+url.PathEscape(runID), &run); err != nil {
		return nil, err
	}
	return &run, nil
}

func (c *Client) DeleteAccountBackup(accountID, id string) error {
	return c.Delete(accountBackupsPath(accountID) + "/" + url.PathEscape(id))
}
