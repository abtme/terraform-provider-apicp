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

// fakeRelayAPI is apicpd's relay and mail domain API for one relay (relay_1 on node-1) and one
// mail domain (mdom_1). old = an apicpd before v0.8.0: no relay routes, relay_id ignored.
type fakeRelayAPI struct {
	old      bool
	relayID  string // the domain's
	requests []string
	bodies   []map[string]any
}

func (f *fakeRelayAPI) client(t *testing.T) *client.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		f.bodies = append(f.bodies, body)
		w.Header().Set("Content-Type", "application/json")
		relay := map[string]any{"id": "relay_1", "node_id": "node-1", "name": "shop", "host": "smtp.shop.example", "port": 587,
			"username": "u", "default": false, "status": "active", "mail_domains": []string{}}
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/nodes/node-1/smtp-relays") && f.old:
			http.NotFound(w, r)
			return
		case r.Method == http.MethodPost && r.URL.Path == "/v1/nodes/node-1/smtp-relays":
			for k, v := range body {
				relay[k] = v
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(relay)
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/nodes/node-1/smtp-relays/relay_1":
			for k, v := range body {
				relay[k] = v
			}
			delete(relay, "password")
			_ = json.NewEncoder(w).Encode(relay)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/nodes/node-1/smtp-relays/relay_1":
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"relay shop is used by mail domain(s) a.example: give them another relay (or the default) first"}`))
		case r.URL.Path == "/v1/mail/domains" || r.URL.Path == "/v1/mail/domains/mdom_1":
			if v, ok := body["relay_id"].(string); ok && !f.old {
				f.relayID = v
			}
			out := map[string]any{"id": "mdom_1", "name": "a.example", "unix_user": "ml_a", "status": "active"}
			if f.relayID != "" {
				out["relay_id"] = f.relayID
			}
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusCreated)
			}
			_ = json.NewEncoder(w).Encode(out)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	return client.New(srv.URL, "token")
}

func mailDomainCreate(t *testing.T, f *fakeRelayAPI, relay any) (resource.CreateResponse, MailDomainResourceModel) {
	t.Helper()
	r := &MailDomainResource{client: f.client(t)}
	sch := resourceSchema(t, r)
	vals := map[string]any{"id": unknownValue{}, "name": "a.example", "unix_user": unknownValue{}, "disable_imap": false, "disable_pop3": false,
		"max_emails_per_hour": 0, "status": unknownValue{}, "relay_id": relay}
	resp := resource.CreateResponse{State: nullState(tfsdk.State{Schema: sch})}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: sch, Raw: objectValue(t, sch, vals)}}, &resp)
	var got MailDomainResourceModel
	resp.State.Get(context.Background(), &got)
	return resp, got
}

func TestMailDomainCreateWithARelay(t *testing.T) {
	f := &fakeRelayAPI{}
	resp, got := mailDomainCreate(t, f, "relay_1")
	if resp.Diagnostics.HasError() || got.RelayID.ValueString() != "relay_1" || f.bodies[0]["relay_id"] != "relay_1" {
		t.Fatalf("%v %+v %v", resp.Diagnostics, got, f.bodies)
	}
	// left out: not sent, and the state shows the server's ("" = the default relay)
	f = &fakeRelayAPI{}
	resp, got = mailDomainCreate(t, f, unknownValue{})
	if _, sent := f.bodies[0]["relay_id"]; sent || resp.Diagnostics.HasError() || got.RelayID.IsUnknown() || got.RelayID.ValueString() != "" {
		t.Fatalf("%v %+v %v", resp.Diagnostics, got, f.bodies)
	}
}

// An apicpd before v0.8.0 creates the domain and ignores relay_id: an error naming the version, and
// the domain in state (so Terraform replaces it instead of losing track of it).
func TestMailDomainRelayOnAnOldApicpd(t *testing.T) {
	resp, got := mailDomainCreate(t, &fakeRelayAPI{old: true}, "relay_1")
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "v0.8.0") {
		t.Fatalf("%v", resp.Diagnostics)
	}
	if got.ID.ValueString() != "mdom_1" {
		t.Fatalf("the created domain must be in state: %+v", got)
	}
}

func mailDomainUpdate(t *testing.T, f *fakeRelayAPI, from string, to any) (resource.UpdateResponse, MailDomainResourceModel) {
	t.Helper()
	r := &MailDomainResource{client: f.client(t)}
	sch := resourceSchema(t, r)
	base := map[string]any{"id": "mdom_1", "name": "a.example", "unix_user": "ml_a", "disable_imap": false, "disable_pop3": false,
		"max_emails_per_hour": 10, "status": "active"}
	state, plan := map[string]any{"relay_id": from}, map[string]any{"relay_id": to, "status": unknownValue{}}
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
	var got MailDomainResourceModel
	resp.State.Get(context.Background(), &got)
	return resp, got
}

func TestMailDomainUpdateRelay(t *testing.T) {
	for _, tc := range []struct{ from, to string }{{"", "relay_1"}, {"relay_1", ""}} {
		f := &fakeRelayAPI{relayID: tc.from}
		resp, got := mailDomainUpdate(t, f, tc.from, tc.to)
		if resp.Diagnostics.HasError() || got.RelayID.ValueString() != tc.to || f.bodies[0]["relay_id"] != tc.to {
			t.Errorf("%q -> %q: %v %+v %v", tc.from, tc.to, resp.Diagnostics, got, f.bodies)
		}
	}
	// Unchanged relay: not sent (other changes keep working on an apicpd before v0.8.0).
	f := &fakeRelayAPI{old: true}
	resp, _ := mailDomainUpdate(t, f, "", "")
	if _, sent := f.bodies[0]["relay_id"]; sent || resp.Diagnostics.HasError() {
		t.Errorf("%v %v", resp.Diagnostics, f.bodies)
	}
	f = &fakeRelayAPI{old: true}
	if resp, _ := mailDomainUpdate(t, f, "", "relay_1"); !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "v0.8.0") {
		t.Errorf("an old apicpd ignoring relay_id must be an error: %v", resp.Diagnostics)
	}
}

func mailRelayPlan(t *testing.T, r *MailRelayResource, vals map[string]any) tfsdk.Plan {
	t.Helper()
	sch := resourceSchema(t, r)
	base := map[string]any{"node_id": "node-1", "name": "shop", "host": "smtp.shop.example", "port": 587, "username": "u", "password": "p", "default": false}
	for k, v := range vals {
		base[k] = v
	}
	return tfsdk.Plan{Schema: sch, Raw: objectValue(t, sch, base)}
}

func TestMailRelayCreateUpdateDelete(t *testing.T) {
	f := &fakeRelayAPI{}
	r := &MailRelayResource{client: f.client(t)}
	sch := resourceSchema(t, r)
	resp := resource.CreateResponse{State: nullState(tfsdk.State{Schema: sch})}
	r.Create(context.Background(), resource.CreateRequest{Plan: mailRelayPlan(t, r, map[string]any{"id": unknownValue{}, "status": unknownValue{}})}, &resp)
	var got MailRelayResourceModel
	resp.State.Get(context.Background(), &got)
	if resp.Diagnostics.HasError() || got.ID.ValueString() != "relay_1" || got.Password.ValueString() != "p" || got.Default.ValueBool() {
		t.Fatalf("%v %+v", resp.Diagnostics, got)
	}
	if b := f.bodies[0]; b["name"] != "shop" || b["password"] != "p" || b["default"] != nil {
		t.Errorf("create body %v", b)
	}

	// A new host: the password travels with it (apicp only sends a stored password to its own host).
	state := tfsdk.State{Schema: sch, Raw: mailRelayPlan(t, r, map[string]any{"id": "relay_1", "status": "active"}).Raw}
	plan := mailRelayPlan(t, r, map[string]any{"id": "relay_1", "status": unknownValue{}, "host": "smtp2.shop.example", "default": true})
	uresp := resource.UpdateResponse{State: state}
	r.Update(context.Background(), resource.UpdateRequest{Plan: plan, State: state}, &uresp)
	if uresp.Diagnostics.HasError() {
		t.Fatal(uresp.Diagnostics)
	}
	if b := f.bodies[1]; b["host"] != "smtp2.shop.example" || b["password"] != "p" || b["default"] != true {
		t.Errorf("update body %v", b)
	}
	uresp.State.Get(context.Background(), &got)
	if !got.Default.ValueBool() || got.Host.ValueString() != "smtp2.shop.example" {
		t.Errorf("%+v", got)
	}

	// In use: apicp's reason reaches the operator.
	dresp := resource.DeleteResponse{State: state}
	r.Delete(context.Background(), resource.DeleteRequest{State: state}, &dresp)
	if !dresp.Diagnostics.HasError() || !strings.Contains(dresp.Diagnostics.Errors()[0].Detail(), "a.example") {
		t.Errorf("%v", dresp.Diagnostics)
	}
}

func TestMailRelayOnAnOldApicpd(t *testing.T) {
	r := &MailRelayResource{client: (&fakeRelayAPI{old: true}).client(t)}
	resp := resource.CreateResponse{State: nullState(tfsdk.State{Schema: resourceSchema(t, r)})}
	r.Create(context.Background(), resource.CreateRequest{Plan: mailRelayPlan(t, r, map[string]any{"id": unknownValue{}, "status": unknownValue{}})}, &resp)
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "v0.8.0") {
		t.Fatalf("%v", resp.Diagnostics)
	}
}

func TestMailRelayValidateConfigAndImport(t *testing.T) {
	r := &MailRelayResource{}
	sch := resourceSchema(t, r)
	for name, c := range map[string]struct {
		vals    map[string]any
		wantErr bool
	}{
		"login":         {map[string]any{"username": "u", "password": "p"}, false},
		"no login":      {map[string]any{}, false},
		"username only": {map[string]any{"username": "u"}, true},
		"empty strings": {map[string]any{"username": "", "password": ""}, true},
	} {
		vals := map[string]any{"node_id": "node-1", "name": "x", "host": "h", "port": 25}
		for k, v := range c.vals {
			vals[k] = v
		}
		var resp resource.ValidateConfigResponse
		r.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: sch, Raw: objectValue(t, sch, vals)}}, &resp)
		if resp.Diagnostics.HasError() != c.wantErr {
			t.Errorf("%s: %v", name, resp.Diagnostics)
		}
	}
	for id, ok := range map[string]bool{"node-1/relay_1": true, "relay_1": false, "/relay_1": false} {
		resp := resource.ImportStateResponse{State: nullState(tfsdk.State{Schema: sch})}
		r.ImportState(context.Background(), resource.ImportStateRequest{ID: id}, &resp)
		if resp.Diagnostics.HasError() == ok {
			t.Errorf("import %q: %v", id, resp.Diagnostics)
		}
	}
}
