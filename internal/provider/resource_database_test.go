package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

// fakeDatabases is apicpd's database API for one database, db_1. old = an
// apicpd from before database links (no vhost_id, no PATCH).
type fakeDatabases struct {
	old     bool
	vhost   string
	creates []map[string]any
	patches []map[string]any
	refuse  string // a field this apicpd does not know: a request sending it gets apicp v0.7.0's 422
}

func (f *fakeDatabases) client(t *testing.T) *client.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		w.Header().Set("Content-Type", "application/json")
		if f.refuse != "" && r.Method != http.MethodGet {
			_ = json.NewDecoder(r.Body).Decode(&body)
			if _, has := body[f.refuse]; has {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"error":"unknown field \"` + f.refuse + `\": apicpd 0.7.0 does not know it (is the client newer than this server?); nothing was changed"}`))
				return
			}
			t.Errorf("refuse is only for requests that send %s", f.refuse)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/databases":
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.creates = append(f.creates, body)
			if v, ok := body["vhost_id"].(string); ok && !f.old {
				if v == "vh_other" {
					w.WriteHeader(http.StatusUnprocessableEntity)
					_, _ = w.Write([]byte(`{"error":"vhost_id is not a website of this account"}`))
					return
				}
				f.vhost = v
			}
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/databases/db_1":
			if f.old {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.patches = append(f.patches, body)
			f.vhost = body["vhost_id"].(string)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/databases/db_1":
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		out := map[string]any{"id": "db_1", "engine": "mysql", "name": "shop", "username": "shop", "node_id": "n1", "status": "active"}
		if f.vhost != "" {
			out["vhost_id"] = f.vhost
		}
		if r.Method == http.MethodPost {
			out["password"] = "secret"
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return client.New(srv.URL, "token")
}

func databaseCreate(t *testing.T, f *fakeDatabases, vhost any) (resource.CreateResponse, DatabaseResourceModel) {
	t.Helper()
	r := &DatabaseResource{client: f.client(t)}
	sch := resourceSchema(t, r)
	vals := map[string]any{"id": unknownValue{}, "engine": "mysql", "name": "shop", "username": unknownValue{}, "node_id": unknownValue{},
		"status": unknownValue{}, "password": unknownValue{}, "vhost_id": vhost}
	resp := resource.CreateResponse{State: nullState(tfsdk.State{Schema: sch})}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: sch, Raw: objectValue(t, sch, vals)}}, &resp)
	var got DatabaseResourceModel
	if !resp.Diagnostics.HasError() {
		resp.State.Get(context.Background(), &got)
	}
	return resp, got
}

func TestDatabaseCreateLinksTheWebsite(t *testing.T) {
	f := &fakeDatabases{}
	resp, got := databaseCreate(t, f, "vh_1")
	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() != 0 {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if f.creates[0]["vhost_id"] != "vh_1" || got.VhostID.ValueString() != "vh_1" || got.Password.ValueString() != "secret" {
		t.Errorf("sent %v, state %+v", f.creates[0], got)
	}

	// left out: not sent, and the state shows the server's (no link)
	f = &fakeDatabases{}
	resp, got = databaseCreate(t, f, unknownValue{})
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if _, has := f.creates[0]["vhost_id"]; has || got.VhostID.IsUnknown() || got.VhostID.ValueString() != "" {
		t.Errorf("sent %v, state vhost_id %v", f.creates[0], got.VhostID)
	}
}

func TestDatabaseCreateWebsiteOfAnotherAccount(t *testing.T) {
	resp, _ := databaseCreate(t, &fakeDatabases{}, "vh_other")
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "not a website of this account") {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
}

// An apicpd without links creates the database but not the link: a warning,
// the database kept in state (not tainted), the link shown as not there.
func TestDatabaseCreateOnAnOldApicpd(t *testing.T) {
	resp, got := databaseCreate(t, &fakeDatabases{old: true}, "vh_1")
	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() != 1 || !strings.Contains(resp.Diagnostics.Warnings()[0].Detail(), "v0.7.0") {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if got.ID.ValueString() != "db_1" || got.VhostID.ValueString() != "" {
		t.Errorf("state = %+v", got)
	}
}

func databaseUpdate(t *testing.T, f *fakeDatabases, from string, to any) resource.UpdateResponse {
	t.Helper()
	r := &DatabaseResource{client: f.client(t)}
	sch := resourceSchema(t, r)
	base := map[string]any{"id": "db_1", "engine": "mysql", "name": "shop", "username": "shop", "node_id": "n1", "status": "active", "password": "secret"}
	state, plan := map[string]any{"vhost_id": from}, map[string]any{"vhost_id": to, "status": unknownValue{}}
	for k, v := range base {
		state[k] = v
		if _, set := plan[k]; !set {
			plan[k] = v
		}
	}
	resp := resource.UpdateResponse{State: tfsdk.State{Schema: sch, Raw: objectValue(t, sch, state)}}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: sch, Raw: objectValue(t, sch, plan)},
		State: tfsdk.State{Schema: sch, Raw: objectValue(t, sch, state)},
	}, &resp)
	return resp
}

func TestDatabaseUpdateChangesTheLink(t *testing.T) {
	for _, tc := range []struct{ from, to string }{{"", "vh_1"}, {"vh_1", "vh_2"}, {"vh_1", ""}} {
		f := &fakeDatabases{vhost: tc.from}
		resp := databaseUpdate(t, f, tc.from, tc.to)
		if resp.Diagnostics.HasError() {
			t.Fatalf("%q -> %q: %v", tc.from, tc.to, resp.Diagnostics)
		}
		var got DatabaseResourceModel
		resp.State.Get(context.Background(), &got)
		if len(f.patches) != 1 || f.patches[0]["vhost_id"] != tc.to || got.VhostID.ValueString() != tc.to || got.Status.ValueString() != "active" || got.Password.ValueString() != "secret" {
			t.Errorf("%q -> %q: patches %v, state %+v", tc.from, tc.to, f.patches, got)
		}
	}
}

func TestDatabaseUpdateOnAnOldApicpd(t *testing.T) {
	resp := databaseUpdate(t, &fakeDatabases{old: true}, "", "vh_1")
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "v0.7.0") {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
}

// The create body is exactly engine and name (and vhost_id when set).
func TestDatabaseCreateBodyIsExact(t *testing.T) {
	for vhost, want := range map[any]string{"vh_1": `{"engine":"mysql","name":"shop","vhost_id":"vh_1"}`, unknownValue{}: `{"engine":"mysql","name":"shop"}`} {
		f := &fakeDatabases{}
		if resp, _ := databaseCreate(t, f, vhost); resp.Diagnostics.HasError() {
			t.Fatal(resp.Diagnostics)
		}
		got, _ := json.Marshal(f.creates[0])
		if string(got) != want {
			t.Errorf("vhost %v: body %s, want %s", vhost, got, want)
		}
	}
	f := &fakeDatabases{}
	databaseUpdate(t, f, "vh_1", "")
	if got, _ := json.Marshal(f.patches); string(got) != `[{"vhost_id":""}]` {
		t.Errorf("PATCH bodies %s", got)
	}
}

// apicp v0.7.0's 422 for a field it does not know reads as such, on create
// and on update (not as "a website of another account").
func TestDatabaseUnknownFieldIsAClearError(t *testing.T) {
	resp, _ := databaseCreate(t, &fakeDatabases{refuse: "vhost_id"}, "vh_1")
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), `does not support "vhost_id"`) {
		t.Errorf("create: %v", resp.Diagnostics)
	}
	uresp := databaseUpdate(t, &fakeDatabases{refuse: "vhost_id"}, "", "vh_1")
	if !uresp.Diagnostics.HasError() || !strings.Contains(uresp.Diagnostics.Errors()[0].Detail(), "remove the vhost_id argument") ||
		strings.Contains(uresp.Diagnostics.Errors()[0].Detail(), "own account") {
		t.Errorf("update: %v", uresp.Diagnostics)
	}
}
