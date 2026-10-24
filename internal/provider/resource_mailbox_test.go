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
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

var mailboxType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{
	"id": tftypes.String, "mail_domain_id": tftypes.String, "local_part": tftypes.String, "email": tftypes.String,
	"disable_imap": tftypes.Bool, "disable_pop3": tftypes.Bool, "max_emails_per_hour": tftypes.Number,
	"status": tftypes.String, "password": tftypes.String,
}}

func mailboxSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	var resp resource.SchemaResponse
	NewMailboxResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if d := resp.Schema.ValidateImplementation(context.Background()); d.HasError() {
		t.Fatalf("schema implementation: %v", d)
	}
	return resp
}

// mailboxValue: id/email/status/password "unknown" or a value; limit as given.
func mailboxValue(id string, disableIMAP bool, limit int64) tftypes.Value {
	s := func(v string) tftypes.Value {
		if v == "unknown" {
			return tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
		}
		return tftypes.NewValue(tftypes.String, v)
	}
	email, status, pw := "info@example.com", "active", "pw"
	if id == "unknown" {
		email, status, pw = "unknown", "unknown", "unknown"
	}
	return tftypes.NewValue(mailboxType, map[string]tftypes.Value{
		"id": s(id), "mail_domain_id": s("mdom_1"), "local_part": s("info"), "email": s(email),
		"disable_imap": tftypes.NewValue(tftypes.Bool, disableIMAP), "disable_pop3": tftypes.NewValue(tftypes.Bool, false),
		"max_emails_per_hour": tftypes.NewValue(tftypes.Number, limit), "status": s(status), "password": s(pw),
	})
}

// fakeMailboxAPI answers like apicpd; old = an apicpd without per-mailbox limits (never returns the field).
type fakeMailboxAPI struct {
	old     bool
	limit   int64
	patches []map[string]any
	posts   []map[string]any
	deleted bool
}

func (f *fakeMailboxAPI) server(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		resp := map[string]any{"id": "mbox_1", "mail_domain_id": "mdom_1", "local_part": "info", "email": "info@example.com",
			"disable_imap": body["disable_imap"] == true, "disable_pop3": false, "status": "active"}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/mail/domains/mdom_1/mailboxes":
			f.posts = append(f.posts, body)
			resp["password"] = "generated"
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/mail/domains/mdom_1/mailboxes/mbox_1":
			f.patches = append(f.patches, body)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/mail/domains/mdom_1/mailboxes/mbox_1":
			f.deleted = true
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if !f.old {
			if v, ok := body["max_emails_per_hour"].(float64); ok {
				f.limit = int64(v)
			}
			resp["max_emails_per_hour"] = f.limit
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func updateMailbox(t *testing.T, f *fakeMailboxAPI, limit int64) resource.UpdateResponse {
	t.Helper()
	r := &MailboxResource{client: client.New(f.server(t).URL, "token")}
	sch := mailboxSchema(t).Schema
	resp := resource.UpdateResponse{State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(mailboxType, nil)}}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: sch, Raw: mailboxValue("mbox_1", true, limit)},
		State: tfsdk.State{Schema: sch, Raw: mailboxValue("mbox_1", true, 0)},
	}, &resp)
	return resp
}

// A limit change must carry both switches: apicpd treats a PATCH body with no field it knows as a password
// reset, and an apicpd from before per-mailbox limits does not know max_emails_per_hour.
func TestMailboxUpdateSendsTheSwitchesWithTheLimit(t *testing.T) {
	f := &fakeMailboxAPI{}
	resp := updateMailbox(t, f, 300)
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if len(f.patches) != 1 {
		t.Fatalf("patches = %v", f.patches)
	}
	p := f.patches[0]
	if p["max_emails_per_hour"] != float64(300) || p["disable_imap"] != true || p["disable_pop3"] != false {
		t.Fatalf("PATCH body = %v, want the limit with both switches", p)
	}
	if _, ok := p["reset_password"]; ok {
		t.Fatal("must never ask for a password reset")
	}
	var got MailboxResourceModel
	resp.State.Get(context.Background(), &got)
	if got.MaxPerHour.ValueInt64() != 300 || got.Password.ValueString() != "pw" {
		t.Errorf("state = %+v", got)
	}
}

// Setting a limit back to 0 still sends the field (no omitempty on update).
func TestMailboxUpdateSendsZero(t *testing.T) {
	f := &fakeMailboxAPI{limit: 300}
	if resp := updateMailbox(t, f, 0); resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if v, ok := f.patches[0]["max_emails_per_hour"]; !ok || v != float64(0) {
		t.Fatalf("PATCH body = %v, want max_emails_per_hour 0", f.patches[0])
	}
}

func TestMailboxUpdateOnAnOldApicpdIsAnError(t *testing.T) {
	f := &fakeMailboxAPI{old: true}
	resp := updateMailbox(t, f, 300)
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "per-mailbox sending limits") {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	var got MailboxResourceModel
	resp.State.Get(context.Background(), &got)
	if got.MaxPerHour.ValueInt64() != 0 {
		t.Errorf("state must record that the server has no limit, got %v", got.MaxPerHour)
	}
	// Without a limit an old apicpd is fine.
	if resp := updateMailbox(t, &fakeMailboxAPI{old: true}, 0); resp.Diagnostics.HasError() {
		t.Fatalf("limit 0 on an old apicpd: %v", resp.Diagnostics)
	}
}

func createMailbox(t *testing.T, f *fakeMailboxAPI, limit int64) resource.CreateResponse {
	t.Helper()
	r := &MailboxResource{client: client.New(f.server(t).URL, "token")}
	sch := mailboxSchema(t).Schema
	resp := resource.CreateResponse{State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(mailboxType, nil)}}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: sch, Raw: mailboxValue("unknown", false, limit)}}, &resp)
	return resp
}

func TestMailboxCreateWithALimit(t *testing.T) {
	f := &fakeMailboxAPI{}
	resp := createMailbox(t, f, 100)
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if f.posts[0]["max_emails_per_hour"] != float64(100) {
		t.Fatalf("POST body = %v", f.posts[0])
	}
	var got MailboxResourceModel
	resp.State.Get(context.Background(), &got)
	if got.MaxPerHour.ValueInt64() != 100 || got.Password.ValueString() != "generated" {
		t.Errorf("state = %+v", got)
	}
}

// An old apicpd makes the mailbox without the limit: the provider removes it again and saves no state, so
// Terraform never taints (and later replaces) a mailbox that may hold mail.
func TestMailboxCreateOnAnOldApicpdRemovesTheMailbox(t *testing.T) {
	f := &fakeMailboxAPI{old: true}
	resp := createMailbox(t, f, 100)
	if !resp.Diagnostics.HasError() {
		t.Fatal("want an error")
	}
	if !f.deleted {
		t.Fatal("the mailbox apicpd made must be removed")
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("no state may be saved")
	}
	// Without a limit an old apicpd works as before, and the limit reads as 0.
	f2 := &fakeMailboxAPI{old: true}
	resp = createMailbox(t, f2, 0)
	if resp.Diagnostics.HasError() || f2.deleted {
		t.Fatalf("limit 0 on an old apicpd: %v deleted=%v", resp.Diagnostics, f2.deleted)
	}
	if _, ok := f2.posts[0]["max_emails_per_hour"]; ok {
		t.Fatalf("POST body = %v: an unset limit is left out", f2.posts[0])
	}
}

func TestMailboxValidateConfigRefusesANegativeLimit(t *testing.T) {
	sch := mailboxSchema(t).Schema
	for limit, wantErr := range map[int64]bool{0: false, 500: false, -1: true} {
		var resp resource.ValidateConfigResponse
		NewMailboxResource().(resource.ResourceWithValidateConfig).ValidateConfig(context.Background(),
			resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: sch, Raw: mailboxValue("mbox_1", false, limit)}}, &resp)
		if got := resp.Diagnostics.HasError(); got != wantErr {
			t.Errorf("limit %d: error=%v, want %v", limit, got, wantErr)
		}
	}
}
