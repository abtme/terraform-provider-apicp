package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

func fastBackupPoll(t *testing.T) {
	t.Helper()
	old := client.AccountBackupPollInterval
	client.AccountBackupPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { client.AccountBackupPollInterval = old })
}

// fakeAccountBackups is apicpd's account backup API for acct_1: POST starts
// arun_1, which finishes with run; the list is stored; DELETE removes from it.
type fakeAccountBackups struct {
	mu      sync.Mutex
	run     string           // the finished run's JSON
	runGone bool             // the run's status file was pruned (404)
	stored  []map[string]any // GET /v1/accounts/acct_1/backups
	deleted []string
}

func (f *fakeAccountBackups) client(t *testing.T) *client.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		const base = "/v1/accounts/acct_1/backups"
		switch {
		case r.Method == http.MethodPost && r.URL.Path == base:
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"run_id":"arun_1","account_id":"acct_1","status":"running","stage":"starting"}`))
		case r.Method == http.MethodGet && r.URL.Path == base+"/runs/arun_1":
			if f.runGone {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"backup run not found"}`))
				return
			}
			_, _ = w.Write([]byte(f.run))
		case r.Method == http.MethodGet && r.URL.Path == base:
			_ = json.NewEncoder(w).Encode(map[string]any{"backups": f.stored})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, base+"/"):
			id := strings.TrimPrefix(r.URL.Path, base+"/")
			for i, b := range f.stored {
				if b["id"] == id {
					f.stored = append(f.stored[:i:i], f.stored[i+1:]...)
					f.deleted = append(f.deleted, id)
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"backup not found"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	return client.New(srv.URL, "token")
}

func perDomainArchives() []map[string]any {
	return []map[string]any{
		{"id": "bkp_s", "account_id": "acct_1", "created_at": "2026-10-25T10:00:00Z", "size_bytes": 10, "scope": "account", "file": "manual/root._.alice.acct_1.t.bkp_s.tar.gz"},
		{"id": "bkp_w", "account_id": "acct_1", "created_at": "2026-10-25T10:00:05Z", "size_bytes": 200, "scope": "domain-web", "domain": "alice.example", "file": "manual/root._.alice.alice.example.web.t.bkp_w.tar.gz"},
		{"id": "bkp_m", "account_id": "acct_1", "created_at": "2026-10-25T10:00:09Z", "size_bytes": 30, "scope": "domain-mail", "domain": "alice.example", "file": "manual/root._.alice.alice.example.mail.t.bkp_m.tar.gz"},
	}
}

func perDomainRun(t *testing.T, warnings []string) string {
	t.Helper()
	all := perDomainArchives()
	b, err := json.Marshal(map[string]any{"run_id": "arun_1", "account_id": "acct_1", "status": "finished",
		"backup": all[0], "backups": all, "warnings": warnings})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func nullState(sch tfsdk.State) tfsdk.State {
	sch.Raw = tftypes.NewValue(sch.Schema.Type().TerraformType(context.Background()), nil)
	return sch
}

func accountBackupCreate(t *testing.T, f *fakeAccountBackups) (resource.CreateResponse, AccountBackupResourceModel) {
	t.Helper()
	r := &AccountBackupResource{client: f.client(t)}
	sch := resourceSchema(t, r)
	plan := tfsdk.Plan{Schema: sch, Raw: objectValue(t, sch, map[string]any{"id": unknownValue{}, "account_id": "acct_1",
		"created_at": unknownValue{}, "size_bytes": unknownValue{}, "run_id": unknownValue{}, "backups": unknownValue{}})}
	resp := resource.CreateResponse{State: nullState(tfsdk.State{Schema: sch})}
	r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &resp)
	var got AccountBackupResourceModel
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Get(context.Background(), &got)...)
	}
	return resp, got
}

func archivesOf(t *testing.T, m AccountBackupResourceModel) []accountBackupArchive {
	t.Helper()
	var out []accountBackupArchive
	if d := m.Backups.ElementsAs(context.Background(), &out, false); d.HasError() {
		t.Fatal(d)
	}
	return out
}

func stateOf(t *testing.T, r *AccountBackupResource, m AccountBackupResourceModel) tfsdk.State {
	t.Helper()
	st := tfsdk.State{Schema: resourceSchema(t, r)}
	st = nullState(st)
	if d := st.Set(context.Background(), &m); d.HasError() {
		t.Fatal(d)
	}
	return st
}

// Create waits for apicp's backup run and keeps a run warning as a warning,
// not an error: the backup was made.
func TestAccountBackupCreateWaitsAndWarns(t *testing.T) {
	fastBackupPoll(t)
	f := &fakeAccountBackups{run: `{"run_id":"arun_1","account_id":"acct_1","status":"finished","warning":"old backups could not be removed",
		"backup":{"id":"bkp_1","account_id":"acct_1","created_at":"2026-10-22T10:00:00Z","size_bytes":42}}`}
	resp, got := accountBackupCreate(t, f)
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() != 1 || !strings.Contains(resp.Diagnostics.Warnings()[0].Detail(), "old backups could not be removed") {
		t.Errorf("want one warning about the prune, got %v", resp.Diagnostics)
	}
	// an apicpd with one archive per account: the old attributes as before, backups holds that one
	a := archivesOf(t, got)
	if got.ID.ValueString() != "bkp_1" || got.SizeBytes.ValueInt64() != 42 || got.AccountID.ValueString() != "acct_1" || got.RunID.ValueString() != "arun_1" ||
		len(a) != 1 || a[0].ID.ValueString() != "bkp_1" || a[0].Scope.ValueString() != "" || !a[0].Verified.IsNull() {
		t.Errorf("state = %+v, archives %+v", got, a)
	}
}

func TestAccountBackupCreatePerDomain(t *testing.T) {
	fastBackupPoll(t)
	f := &fakeAccountBackups{run: perDomainRun(t, []string{"root._.alice.alice.example.mail.tar.gz: mailbox a: password not readable"})}
	resp, got := accountBackupCreate(t, f)
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() != 1 || !strings.Contains(resp.Diagnostics.Warnings()[0].Detail(), "password not readable") {
		t.Errorf("want the archive warning, got %v", resp.Diagnostics)
	}
	a := archivesOf(t, got)
	if got.ID.ValueString() != "bkp_s" || got.SizeBytes.ValueInt64() != 240 || got.CreatedAt.ValueString() != "2026-10-25T10:00:00Z" || len(a) != 3 {
		t.Fatalf("state = %+v (%d archives)", got, len(a))
	}
	if a[1].ID.ValueString() != "bkp_w" || a[1].Scope.ValueString() != "domain-web" || a[1].Domain.ValueString() != "alice.example" ||
		a[1].Name.ValueString() != "manual/root._.alice.alice.example.web.t.bkp_w.tar.gz" || a[1].SizeBytes.ValueInt64() != 200 {
		t.Errorf("archive 1 = %+v", a[1])
	}
}

// Read keeps the archives still on the server; the resource is gone once none is.
func TestAccountBackupReadFollowsRetention(t *testing.T) {
	fastBackupPoll(t)
	f := &fakeAccountBackups{run: perDomainRun(t, nil)}
	_, created := accountBackupCreate(t, f)
	r := &AccountBackupResource{client: f.client(t)}
	state := stateOf(t, r, created)

	all := perDomainArchives()
	f.stored = []map[string]any{{"id": "bkp_other", "account_id": "acct_1"}, all[0], all[2]} // bkp_w pruned
	resp := resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	var got AccountBackupResourceModel
	resp.State.Get(context.Background(), &got)
	a := archivesOf(t, got)
	if len(a) != 2 || a[0].ID.ValueString() != "bkp_s" || a[1].ID.ValueString() != "bkp_m" || got.SizeBytes.ValueInt64() != 40 || got.RunID.ValueString() != "arun_1" {
		t.Errorf("state = %+v, archives %+v", got, a)
	}

	f.stored = f.stored[:1] // only someone else's backup left
	resp = resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() || !resp.State.Raw.IsNull() {
		t.Errorf("want the resource removed: %v, %v", resp.Diagnostics, resp.State.Raw)
	}
}

// State from before backups existed (id only) still reads and deletes.
func TestAccountBackupOldState(t *testing.T) {
	f := &fakeAccountBackups{stored: []map[string]any{{"id": "bkp_1", "account_id": "acct_1", "created_at": "2026-10-01T00:00:00Z", "size_bytes": 5}}}
	r := &AccountBackupResource{client: f.client(t)}
	sch := resourceSchema(t, r)
	state := tfsdk.State{Schema: sch, Raw: objectValue(t, sch, map[string]any{"id": "bkp_1", "account_id": "acct_1",
		"created_at": "2026-10-01T00:00:00Z", "size_bytes": 5})}

	resp := resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	var got AccountBackupResourceModel
	resp.State.Get(context.Background(), &got)
	if a := archivesOf(t, got); len(a) != 1 || a[0].ID.ValueString() != "bkp_1" || got.RunID.ValueString() != "" || got.SizeBytes.ValueInt64() != 5 {
		t.Errorf("state = %+v", got)
	}

	var dresp resource.DeleteResponse
	r.Delete(context.Background(), resource.DeleteRequest{State: state}, &dresp)
	if dresp.Diagnostics.HasError() || len(f.deleted) != 1 || f.deleted[0] != "bkp_1" {
		t.Errorf("delete: %v, deleted %v", dresp.Diagnostics, f.deleted)
	}
}

// Destroy deletes every archive; one already gone is fine.
func TestAccountBackupDeleteRemovesEveryArchive(t *testing.T) {
	fastBackupPoll(t)
	f := &fakeAccountBackups{run: perDomainRun(t, nil)}
	_, created := accountBackupCreate(t, f)
	all := perDomainArchives()
	f.stored = []map[string]any{all[0], all[2]} // bkp_w already gone
	r := &AccountBackupResource{client: f.client(t)}
	state := stateOf(t, r, created)

	var resp resource.DeleteResponse
	r.Delete(context.Background(), resource.DeleteRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	sort.Strings(f.deleted)
	if strings.Join(f.deleted, ",") != "bkp_m,bkp_s" || len(f.stored) != 0 {
		t.Errorf("deleted %v, left %v", f.deleted, f.stored)
	}
}

func TestAccountBackupImport(t *testing.T) {
	f := &fakeAccountBackups{run: perDomainRun(t, nil), stored: perDomainArchives()}
	r := &AccountBackupResource{client: f.client(t)}
	sch := resourceSchema(t, r)

	for id, want := range map[string][]string{
		"acct_1/bkp_s":             {"bkp_s"},
		"acct_1/bkp_s,bkp_w,bkp_m": {"bkp_s", "bkp_w", "bkp_m"},
		"acct_1/arun_1":            {"bkp_s", "bkp_w", "bkp_m"},
	} {
		resp := resource.ImportStateResponse{State: nullState(tfsdk.State{Schema: sch})}
		r.ImportState(context.Background(), resource.ImportStateRequest{ID: id}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("%s: %v", id, resp.Diagnostics)
		}
		// the read that follows an import
		rresp := resource.ReadResponse{State: resp.State}
		r.Read(context.Background(), resource.ReadRequest{State: resp.State}, &rresp)
		if rresp.Diagnostics.HasError() {
			t.Fatalf("%s read: %v", id, rresp.Diagnostics)
		}
		var got AccountBackupResourceModel
		rresp.State.Get(context.Background(), &got)
		var ids []string
		for _, a := range archivesOf(t, got) {
			ids = append(ids, a.ID.ValueString())
		}
		if strings.Join(ids, ",") != strings.Join(want, ",") || got.AccountID.ValueString() != "acct_1" || got.ID.ValueString() != "bkp_s" || got.CreatedAt.ValueString() == "" {
			t.Errorf("%s: state %+v, archives %v", id, got, ids)
		}
	}

	resp := resource.ImportStateResponse{State: nullState(tfsdk.State{Schema: sch})}
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: "bkp_s"}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Error("an ID without the account must be refused")
	}
}

// perDomainBatch is perDomainArchives as apicp v0.7.0 lists them: batch_id
// arun_1 and the restore check's result.
func perDomainBatch() []map[string]any {
	all := perDomainArchives()
	for _, a := range all {
		a["batch_id"] = "arun_1"
		a["verified"] = true
	}
	all[2]["verified"] = false
	all[2]["verify_error"] = "unexpected EOF"
	return all
}

func readBackup(t *testing.T, r *AccountBackupResource, state tfsdk.State) (resource.ReadResponse, AccountBackupResourceModel) {
	t.Helper()
	resp := resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
	var got AccountBackupResourceModel
	if !resp.Diagnostics.HasError() && !resp.State.Raw.IsNull() {
		resp.State.Get(context.Background(), &got)
	}
	return resp, got
}

// Read takes verified and verify_error from the backup list; null where the
// server does not report them.
func TestAccountBackupReadFillsVerified(t *testing.T) {
	fastBackupPoll(t)
	f := &fakeAccountBackups{run: perDomainRun(t, nil)}
	_, created := accountBackupCreate(t, f)
	if a := archivesOf(t, created); !a[0].Verified.IsNull() || a[0].VerifyError.ValueString() != "" {
		t.Fatalf("the run reported no check: archive %+v", a[0])
	}
	r := &AccountBackupResource{client: f.client(t)}

	f.stored = perDomainBatch()
	_, got := readBackup(t, r, stateOf(t, r, created))
	a := archivesOf(t, got)
	if len(a) != 3 || !a[0].Verified.ValueBool() || a[2].Verified.IsNull() || a[2].Verified.ValueBool() || a[2].VerifyError.ValueString() != "unexpected EOF" {
		t.Errorf("archives %+v", a)
	}

	f.stored = perDomainArchives() // an apicpd that does not report it
	_, got = readBackup(t, r, stateOf(t, r, got))
	for _, x := range archivesOf(t, got) {
		if !x.Verified.IsNull() || x.VerifyError.ValueString() != "" {
			t.Errorf("archive %s: verified %v, verify_error %q; want null and empty", x.ID.ValueString(), x.Verified, x.VerifyError.ValueString())
		}
	}
}

// With batch IDs the backup is every archive of its batch: one the state
// does not name joins, the settings archive stays first, other batches and
// an archive named in state but of another batch stay out.
func TestAccountBackupReadUsesTheBatch(t *testing.T) {
	f := &fakeAccountBackups{}
	r := &AccountBackupResource{client: f.client(t)}
	all := perDomainBatch()
	f.stored = []map[string]any{
		all[1], {"id": "bkp_other", "account_id": "acct_1", "scope": "account", "batch_id": "arun_2"}, all[2], all[0],
	}
	sch := resourceSchema(t, r)
	state := tfsdk.State{Schema: sch, Raw: objectValue(t, sch, map[string]any{"id": "bkp_s", "account_id": "acct_1",
		"created_at": "2026-10-25T10:00:00Z", "size_bytes": 10, "run_id": "arun_1"})}
	_, got := readBackup(t, r, state)
	var ids []string
	for _, a := range archivesOf(t, got) {
		ids = append(ids, a.ID.ValueString())
	}
	if strings.Join(ids, ",") != "bkp_s,bkp_w,bkp_m" || got.ID.ValueString() != "bkp_s" || got.SizeBytes.ValueInt64() != 240 || got.RunID.ValueString() != "arun_1" {
		t.Errorf("archives %v, state %+v", ids, got)
	}

	// the batch pruned away: gone, even though another archive exists
	f.stored = f.stored[1:2]
	resp, _ := readBackup(t, r, state)
	if resp.Diagnostics.HasError() || !resp.State.Raw.IsNull() {
		t.Errorf("want the resource removed: %v", resp.Diagnostics)
	}
}

// Import by run ID finds the archives by batch_id when apicp has already
// pruned the run's status file; without batch IDs or the run it is an error.
func TestAccountBackupImportByBatchID(t *testing.T) {
	f := &fakeAccountBackups{runGone: true, stored: append([]map[string]any{{"id": "bkp_other", "account_id": "acct_1", "batch_id": "arun_9"}}, perDomainBatch()...)}
	r := &AccountBackupResource{client: f.client(t)}
	sch := resourceSchema(t, r)
	resp := resource.ImportStateResponse{State: nullState(tfsdk.State{Schema: sch})}
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: "acct_1/arun_1"}, &resp)
	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() != 0 {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	var got AccountBackupResourceModel
	resp.State.Get(context.Background(), &got)
	a := archivesOf(t, got)
	if len(a) != 3 || a[0].ID.ValueString() != "bkp_s" || got.RunID.ValueString() != "arun_1" || a[2].Verified.ValueBool() {
		t.Errorf("state %+v, archives %+v", got, a)
	}

	f.stored = perDomainArchives() // no batch IDs, run gone
	resp = resource.ImportStateResponse{State: nullState(tfsdk.State{Schema: sch})}
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: "acct_1/arun_1"}, &resp)
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "import by archive IDs") {
		t.Errorf("diagnostics: %v", resp.Diagnostics)
	}
}

// Incomplete leftovers of a failed run import with a warning; a run still
// going is refused.
func TestAccountBackupImportIncomplete(t *testing.T) {
	f := &fakeAccountBackups{run: `{"run_id":"arun_1","account_id":"acct_1","status":"failed","error":"disk full"}`,
		stored: []map[string]any{{"id": "bkp_w", "account_id": "acct_1", "batch_id": "arun_1", "incomplete": true}}}
	r := &AccountBackupResource{client: f.client(t)}
	sch := resourceSchema(t, r)
	resp := resource.ImportStateResponse{State: nullState(tfsdk.State{Schema: sch})}
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: "acct_1/arun_1"}, &resp)
	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() != 1 || !strings.Contains(resp.Diagnostics.Warnings()[0].Detail(), "incomplete") {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}

	f.run = `{"run_id":"arun_1","account_id":"acct_1","status":"running","stage":"storing"}`
	resp = resource.ImportStateResponse{State: nullState(tfsdk.State{Schema: sch})}
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: "acct_1/arun_1"}, &resp)
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "still running") {
		t.Errorf("diagnostics: %v", resp.Diagnostics)
	}
}

// apicp v0.7.0: a run that failed part way removed its archives - an error,
// nothing in state.
func TestAccountBackupCreateFailedRunLeavesNothing(t *testing.T) {
	fastBackupPoll(t)
	f := &fakeAccountBackups{run: `{"run_id":"arun_1","account_id":"acct_1","status":"failed",
		"error":"1 of 3 archives failed: mail domain alice.example: disk full; the 2 archives it had made were removed","removed":["bkp_s","bkp_w"]}`}
	resp, _ := accountBackupCreate(t, f)
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "disk full") {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Errorf("nothing may go into state: %v", resp.State.Raw)
	}
}

// A failed restore check keeps the archives: they go into state with the
// error (Terraform then marks the resource tainted), verified as reported.
func TestAccountBackupCreateRestoreCheckFailedTaints(t *testing.T) {
	fastBackupPoll(t)
	all := perDomainBatch()
	b, _ := json.Marshal(map[string]any{"run_id": "arun_1", "account_id": "acct_1", "status": "failed",
		"error": "restore check: 1 of 3 archives could not be read back: mail.tar.gz: unexpected EOF", "backup": all[0], "backups": all})
	f := &fakeAccountBackups{run: string(b)}
	resp, _ := accountBackupCreate(t, f)
	if !resp.Diagnostics.HasError() || resp.Diagnostics.Errors()[0].Summary() != "Account backup failed its restore check" ||
		!strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "bkp_m") || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "tainted") {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	var got AccountBackupResourceModel
	if d := resp.State.Get(context.Background(), &got); d.HasError() {
		t.Fatal(d)
	}
	a := archivesOf(t, got)
	if len(a) != 3 || got.ID.ValueString() != "bkp_s" || got.RunID.ValueString() != "arun_1" || a[2].Verified.ValueBool() || a[2].VerifyError.ValueString() != "unexpected EOF" {
		t.Errorf("state %+v, archives %+v", got, a)
	}
	if !resp.State.Raw.IsFullyKnown() {
		t.Error("the state must be fully known")
	}
}
