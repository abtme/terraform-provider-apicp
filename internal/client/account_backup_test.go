package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fastPoll makes the backup polling quick for a test.
func fastPoll(t *testing.T) {
	t.Helper()
	old := AccountBackupPollInterval
	AccountBackupPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { AccountBackupPollInterval = old })
}

// fakeBackups is a stand-in apicpd: each POST and GET answer comes from the
// handler's queue for that request, in order; the last one repeats.
type fakeBackups struct {
	mu       sync.Mutex
	posts    []reply
	gets     map[string][]reply // by run ID
	nPost    int
	getPaths []string
}

type reply struct {
	code int
	body string
}

func (f *fakeBackups) next(q []reply, i int) reply {
	if i >= len(q) {
		return q[len(q)-1]
	}
	return q[i]
}

func (f *fakeBackups) serve(t *testing.T) *Client {
	t.Helper()
	gotGets := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var rp reply
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/accounts/acct_1/backups":
			rp = f.next(f.posts, f.nPost)
			f.nPost++
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/accounts/acct_1/backups/runs/"):
			run := strings.TrimPrefix(r.URL.Path, "/v1/accounts/acct_1/backups/runs/")
			f.getPaths = append(f.getPaths, run)
			q, ok := f.gets[run]
			if !ok {
				rp = reply{404, `{"error":"backup run not found"}`}
				break
			}
			rp = f.next(q, gotGets[run])
			gotGets[run]++
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			rp = reply{500, `{"error":"unexpected"}`}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rp.code)
		_, _ = w.Write([]byte(rp.body))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "token")
}

const meta1 = `{"id":"bkp_1","account_id":"acct_1","created_at":"2026-10-22T10:00:00Z","size_bytes":1234,"server":"srv"}`

func running(run, stage string) string {
	return `{"run_id":"` + run + `","account_id":"acct_1","status":"running","stage":"` + stage + `","requested_at":"2026-10-22T10:00:00Z"}`
}

func finished(run, meta, warning string) string {
	w := ""
	if warning != "" {
		w = `,"warning":"` + warning + `"`
	}
	return `{"run_id":"` + run + `","account_id":"acct_1","status":"finished","requested_at":"2026-10-22T10:00:00Z","finished_at":"2026-10-22T10:01:00Z","backup":` + meta + w + `}`
}

func TestCreateAccountBackupWaitsForTheRun(t *testing.T) {
	fastPoll(t)
	f := &fakeBackups{
		posts: []reply{{202, running("arun_a1", "starting")}},
		gets: map[string][]reply{"arun_a1": {
			{200, running("arun_a1", "collecting")},
			{200, running("arun_a1", "storing")},
			{200, finished("arun_a1", meta1, "")},
		}},
	}
	c := f.serve(t)

	res, err := c.CreateAccountBackup(context.Background(), "acct_1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Backup.ID != "bkp_1" || res.Backup.SizeBytes != 1234 || res.Backup.AccountID != "acct_1" || res.RunID != "arun_a1" || res.Warning != "" {
		t.Errorf("result = %+v / %+v", res, res.Backup)
	}
	if f.nPost != 1 || len(f.getPaths) != 3 {
		t.Errorf("posts %d, gets %v", f.nPost, f.getPaths)
	}
}

func TestCreateAccountBackupPassesOnTheWarning(t *testing.T) {
	fastPoll(t)
	f := &fakeBackups{
		posts: []reply{{202, running("arun_a1", "starting")}},
		gets:  map[string][]reply{"arun_a1": {{200, finished("arun_a1", meta1, "old backups could not be removed")}}},
	}
	res, err := f.serve(t).CreateAccountBackup(context.Background(), "acct_1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Backup.ID != "bkp_1" || res.Warning != "old backups could not be removed" {
		t.Errorf("result = %+v", res)
	}
}

func TestCreateAccountBackupFailedRunIsAnError(t *testing.T) {
	fastPoll(t)
	f := &fakeBackups{
		posts: []reply{{202, running("arun_a1", "starting")}},
		gets: map[string][]reply{"arun_a1": {
			{200, running("arun_a1", "collecting")},
			{200, `{"run_id":"arun_a1","account_id":"acct_1","status":"failed","error":"dump database acme_wp: disk full"}`},
		}},
	}
	_, err := f.serve(t).CreateAccountBackup(context.Background(), "acct_1")
	if err == nil || !strings.Contains(err.Error(), "disk full") || !strings.Contains(err.Error(), "arun_a1") {
		t.Fatalf("err = %v", err)
	}
}

// A run already going (200) was started by someone else: wait for it to end,
// then take a backup of our own, so two resources never share one backup.
func TestCreateAccountBackupWaitsForAnotherRunThenStartsItsOwn(t *testing.T) {
	fastPoll(t)
	meta2 := strings.Replace(meta1, "bkp_1", "bkp_2", 1)
	f := &fakeBackups{
		posts: []reply{
			{200, running("arun_other", "collecting")},
			{202, running("arun_ours", "starting")},
		},
		gets: map[string][]reply{
			// the other run fails: not our concern, ours still goes ahead
			"arun_other": {{200, running("arun_other", "storing")}, {200, `{"run_id":"arun_other","account_id":"acct_1","status":"failed","error":"someone else's problem"}`}},
			"arun_ours":  {{200, finished("arun_ours", meta2, "")}},
		},
	}
	res, err := f.serve(t).CreateAccountBackup(context.Background(), "acct_1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Backup.ID != "bkp_2" || res.RunID != "arun_ours" {
		t.Errorf("result = %+v", res)
	}
	if f.nPost != 2 {
		t.Errorf("posts = %d, want 2 (one met the other run, one started ours)", f.nPost)
	}
}

// apicpd before the account backup process made the backup within the
// request and answered 201 with it.
func TestCreateAccountBackupOlderServer(t *testing.T) {
	fastPoll(t)
	f := &fakeBackups{posts: []reply{{201, meta1}}}
	res, err := f.serve(t).CreateAccountBackup(context.Background(), "acct_1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Backup.ID != "bkp_1" || res.Backup.CreatedAt != "2026-10-22T10:00:00Z" || res.RunID != "" {
		t.Errorf("result = %+v", res)
	}
	if len(f.getPaths) != 0 {
		t.Errorf("polled %v on a server with no runs", f.getPaths)
	}
}

func TestCreateAccountBackupGivesUpWhenTheContextEnds(t *testing.T) {
	fastPoll(t)
	f := &fakeBackups{
		posts: []reply{{202, running("arun_a1", "starting")}},
		gets:  map[string][]reply{"arun_a1": {{200, running("arun_a1", "collecting")}}},
	}
	c := f.serve(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := c.CreateAccountBackup(ctx, "acct_1")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the deadline", err)
	}
	if !strings.Contains(err.Error(), "arun_a1") || !strings.Contains(err.Error(), "carries on on the server") {
		t.Errorf("err = %v: should name the run and say it carries on", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("took %v to give up", time.Since(start))
	}
}

// apicpd restarting mid-run (the run is its own process and survives that):
// a 5xx or no answer is asked again, not a failure.
func TestCreateAccountBackupRidesOutPassingErrors(t *testing.T) {
	fastPoll(t)
	f := &fakeBackups{
		posts: []reply{{202, running("arun_a1", "starting")}},
		gets: map[string][]reply{"arun_a1": {
			{502, `bad gateway`},
			{503, `{"error":"starting up"}`},
			{200, finished("arun_a1", meta1, "")},
		}},
	}
	res, err := f.serve(t).CreateAccountBackup(context.Background(), "acct_1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Backup.ID != "bkp_1" {
		t.Errorf("result = %+v", res)
	}
}

func TestCreateAccountBackupRunGoneIsAnError(t *testing.T) {
	fastPoll(t)
	f := &fakeBackups{
		posts: []reply{{202, running("arun_a1", "starting")}},
		gets:  map[string][]reply{}, // every GET is 404
	}
	_, err := f.serve(t).CreateAccountBackup(context.Background(), "acct_1")
	if err == nil || !strings.Contains(err.Error(), "gone") {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateAccountBackupStartRefused(t *testing.T) {
	fastPoll(t)
	f := &fakeBackups{posts: []reply{{503, `{"error":"this server does not make account backups"}`}}}
	_, err := f.serve(t).CreateAccountBackup(context.Background(), "acct_1")
	var ae *APIError
	if !errors.As(err, &ae) || ae.StatusCode != 503 || !strings.Contains(err.Error(), "does not make account backups") {
		t.Fatalf("err = %v", err)
	}
}

const perDomainFinished = `{"run_id":"arun_a1","account_id":"acct_1","status":"finished","warning":"",
	"backup":{"id":"bkp_s","account_id":"acct_1","created_at":"2026-10-25T10:00:00Z","size_bytes":10,"scope":"account","file":"manual/root._.alice.acct_1.t.bkp_s.tar.gz"},
	"backups":[
		{"id":"bkp_s","account_id":"acct_1","created_at":"2026-10-25T10:00:00Z","size_bytes":10,"scope":"account","file":"manual/root._.alice.acct_1.t.bkp_s.tar.gz"},
		{"id":"bkp_w","account_id":"acct_1","created_at":"2026-10-25T10:00:05Z","size_bytes":200,"scope":"domain-web","domain":"alice.example","file":"manual/root._.alice.alice.example.web.t.bkp_w.tar.gz"},
		{"id":"bkp_m","account_id":"acct_1","created_at":"2026-10-25T10:00:09Z","size_bytes":30,"scope":"domain-mail","domain":"alice.example","path":"manual/root._.alice.alice.example.mail.t.bkp_m","warnings":["mailbox a: password not readable"]}],
	"warnings":["root._.alice.alice.example.mail.tar.gz: mailbox a: password not readable"]}`

// Per-domain backups: one run makes several archives, all of them returned.
func TestCreateAccountBackupPerDomainArchives(t *testing.T) {
	fastPoll(t)
	f := &fakeBackups{
		posts: []reply{{202, running("arun_a1", "starting")}},
		gets:  map[string][]reply{"arun_a1": {{200, perDomainFinished}}},
	}
	res, err := f.serve(t).CreateAccountBackup(context.Background(), "acct_1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Backup.ID != "bkp_s" || len(res.Backups) != 3 || res.Backups[1].Scope != "domain-web" || res.Backups[1].Domain != "alice.example" {
		t.Fatalf("result = %+v", res)
	}
	if res.Backups[0].Name() != "manual/root._.alice.acct_1.t.bkp_s.tar.gz" || res.Backups[2].Name() != "manual/root._.alice.alice.example.mail.t.bkp_m" {
		t.Errorf("names = %q, %q", res.Backups[0].Name(), res.Backups[2].Name())
	}
	if len(res.Warnings) != 1 || res.Backups[0].Verified != nil {
		t.Errorf("warnings %v, verified %v", res.Warnings, res.Backups[0].Verified)
	}
}

// A run that fails part way on an apicpd from before v0.7.0 (which keeps
// what it made): the error names the archives it did make.
func TestCreateAccountBackupFailedRunNamesItsArchives(t *testing.T) {
	fastPoll(t)
	f := &fakeBackups{
		posts: []reply{{202, running("arun_a1", "starting")}},
		gets: map[string][]reply{"arun_a1": {{200, `{"run_id":"arun_a1","account_id":"acct_1","status":"failed",
			"error":"1 of 3 archives failed: mail domain alice.example: disk full",
			"backups":[{"id":"bkp_s","file":"manual/a.tar.gz"},{"id":"bkp_w"}]}`}}},
	}
	_, err := f.serve(t).CreateAccountBackup(context.Background(), "acct_1")
	if err == nil || !strings.Contains(err.Error(), "disk full") || !strings.Contains(err.Error(), "bkp_s (manual/a.tar.gz), bkp_w") {
		t.Fatalf("err = %v", err)
	}
}

func failedRun(t *testing.T, body string) *AccountBackupFailedError {
	t.Helper()
	fastPoll(t)
	f := &fakeBackups{
		posts: []reply{{202, running("arun_a1", "starting")}},
		gets:  map[string][]reply{"arun_a1": {{200, body}}},
	}
	res, err := f.serve(t).CreateAccountBackup(context.Background(), "acct_1")
	var failed *AccountBackupFailedError
	if res != nil || !errors.As(err, &failed) {
		t.Fatalf("result %+v, err %v: want an *AccountBackupFailedError", res, err)
	}
	return failed
}

// apicp v0.7.0: a run that fails part way deletes what it made; nothing is left.
func TestCreateAccountBackupFailedRunRemovedItsArchives(t *testing.T) {
	failed := failedRun(t, `{"run_id":"arun_a1","account_id":"acct_1","status":"failed",
		"error":"1 of 3 archives failed: mail domain alice.example: disk full; the 2 archives it had made were removed",
		"removed":["bkp_s","bkp_w"]}`)
	if len(failed.Run.Archives()) != 0 || failed.RestoreCheckFailed() || len(failed.Run.Removed) != 2 {
		t.Errorf("run = %+v", failed.Run)
	}
	if msg := failed.Error(); !strings.Contains(msg, "disk full") || !strings.Contains(msg, "were removed") || strings.Contains(msg, "stay on the server") {
		t.Errorf("err = %s", msg)
	}
}

// A part apicp could not delete stays, marked incomplete, and is named.
func TestCreateAccountBackupFailedRunLeftAnIncompletePart(t *testing.T) {
	failed := failedRun(t, `{"run_id":"arun_a1","account_id":"acct_1","status":"failed",
		"error":"1 of 3 archives failed: mail domain alice.example: disk full; the 1 archives it had made were removed, except bkp_w: permission denied",
		"removed":["bkp_s"],"backup":{"id":"bkp_w","batch_id":"arun_a1","incomplete":true},
		"backups":[{"id":"bkp_w","batch_id":"arun_a1","incomplete":true,"file":"manual/w.tar.gz"}]}`)
	left := failed.Run.Archives()
	if len(left) != 1 || !left[0].Incomplete || left[0].BatchID != "arun_a1" {
		t.Fatalf("left = %+v", left)
	}
	if msg := failed.Error(); !strings.Contains(msg, "marked incomplete") || !strings.Contains(msg, "bkp_w (manual/w.tar.gz)") {
		t.Errorf("err = %s", msg)
	}
}

// Every archive made, one fails the restore check: all stay, the failing one named with why.
func TestCreateAccountBackupRestoreCheckFailed(t *testing.T) {
	failed := failedRun(t, `{"run_id":"arun_a1","account_id":"acct_1","status":"failed",
		"error":"restore check: 1 of 2 archives could not be read back: w.tar.gz: unexpected EOF",
		"backups":[{"id":"bkp_s","scope":"account","batch_id":"arun_a1","verified":true},
			{"id":"bkp_w","scope":"domain-web","batch_id":"arun_a1","verified":false,"verify_error":"unexpected EOF","file":"manual/w.tar.gz"}]}`)
	if !failed.RestoreCheckFailed() || len(failed.Run.Archives()) != 2 {
		t.Fatalf("run = %+v", failed.Run)
	}
	msg := failed.Error()
	if !strings.Contains(msg, "2 archive(s) stay on the server") || !strings.Contains(msg, "bkp_w (manual/w.tar.gz): unexpected EOF") || strings.Contains(msg, "bkp_s") {
		t.Errorf("err = %s", msg)
	}
	w := failed.Run.Archives()[1]
	if w.Verified == nil || *w.Verified || w.VerifyError != "unexpected EOF" {
		t.Errorf("archive = %+v", w)
	}
}

func TestBatchArchivesSettingsFirst(t *testing.T) {
	all := []AccountBackup{
		{ID: "bkp_w", Scope: "domain-web", BatchID: "arun_1"},
		{ID: "bkp_old", Scope: "account"},
		{ID: "bkp_s", Scope: "account", BatchID: "arun_1"},
		{ID: "bkp_x", Scope: "domain-mail", BatchID: "arun_2"},
		{ID: "bkp_m", Scope: "domain-mail", BatchID: "arun_1"},
	}
	var ids []string
	for _, b := range BatchArchives(all, "arun_1") {
		ids = append(ids, b.ID)
	}
	if strings.Join(ids, ",") != "bkp_s,bkp_w,bkp_m" {
		t.Errorf("batch = %v", ids)
	}
	if len(BatchArchives(all, "")) != 0 {
		t.Error("an empty batch ID matches nothing")
	}
}

func TestUnknownField(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{&APIError{StatusCode: 422, Message: `unknown field "backup_unit": apicpd 0.7.0 does not know it (is the client newer than this server?); nothing was changed`}, "backup_unit"},
		{&APIError{StatusCode: 422, Message: "retention must be 0 (keep every backup) or a number from 1 to 99"}, ""},
		{&APIError{StatusCode: 400, Message: `unknown field "x"`}, ""},
		{ErrNotFound, ""},
		{nil, ""},
	} {
		got, ok := UnknownField(tc.err)
		if got != tc.want || ok != (tc.want != "") {
			t.Errorf("UnknownField(%v) = %q, %v; want %q", tc.err, got, ok, tc.want)
		}
	}
}
