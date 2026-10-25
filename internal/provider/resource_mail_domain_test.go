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

var mailDomainType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{
	"id": tftypes.String, "name": tftypes.String, "unix_user": tftypes.String,
	"disable_imap": tftypes.Bool, "disable_pop3": tftypes.Bool, "max_emails_per_hour": tftypes.Number,
	"compression": tftypes.String, "compression_level": tftypes.Number, "compressed_mail": tftypes.Bool,
	"status": tftypes.String,
}}

func mailDomainSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	var resp resource.SchemaResponse
	NewMailDomainResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if d := resp.Schema.ValidateImplementation(context.Background()); d.HasError() {
		t.Fatalf("schema implementation: %v", d)
	}
	return resp
}

// mailDomainValue: id "unknown" for a plan of a new domain; comp/level as configured.
func mailDomainValue(id string, limit int64, comp string, level int64) tftypes.Value {
	s := func(v string) tftypes.Value {
		if v == "unknown" {
			return tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
		}
		return tftypes.NewValue(tftypes.String, v)
	}
	user, status := "ml_examplecom", "active"
	compressed := tftypes.NewValue(tftypes.Bool, false)
	if id == "unknown" {
		user, status = "unknown", "unknown"
		compressed = tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue)
	}
	return tftypes.NewValue(mailDomainType, map[string]tftypes.Value{
		"id": s(id), "name": s("example.com"), "unix_user": s(user),
		"disable_imap": tftypes.NewValue(tftypes.Bool, false), "disable_pop3": tftypes.NewValue(tftypes.Bool, false),
		"max_emails_per_hour": tftypes.NewValue(tftypes.Number, limit),
		"compression":         s(comp), "compression_level": tftypes.NewValue(tftypes.Number, level),
		"compressed_mail": compressed, "status": s(status),
	})
}

// fakeMailDomainAPI answers like apicpd; old = one from before compression (never returns it).
type fakeMailDomainAPI struct {
	old          bool
	comp         string
	level        float64
	posts, patch []map[string]any
}

func (f *fakeMailDomainAPI) server(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/mail/domains":
			f.posts = append(f.posts, body)
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/mail/domains/mdom_1":
			f.patch = append(f.patch, body)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if c, ok := body["compression"].(string); ok && !f.old {
			f.comp, f.level = c, body["compression_level"].(float64)
		}
		resp := map[string]any{"id": "mdom_1", "name": "example.com", "unix_user": "ml_examplecom", "disable_imap": false,
			"disable_pop3": false, "max_emails_per_hour": body["max_emails_per_hour"], "status": "active"}
		if !f.old {
			if f.comp == "" {
				f.comp = "off"
			}
			resp["compression"], resp["compression_level"], resp["compressed_mail"] = f.comp, f.level, f.comp != "off"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func createMailDomain(t *testing.T, f *fakeMailDomainAPI, comp string, level int64) (MailDomainResourceModel, resource.CreateResponse) {
	t.Helper()
	r := &MailDomainResource{client: client.New(f.server(t).URL, "token")}
	sch := mailDomainSchema(t).Schema
	resp := resource.CreateResponse{State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(mailDomainType, nil)}}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: sch, Raw: mailDomainValue("unknown", 0, comp, level)}}, &resp)
	var got MailDomainResourceModel
	resp.State.Get(context.Background(), &got)
	return got, resp
}

// Compression on create is sent and read back; off is not sent at all, so an
// apicpd from before compression creates the domain as before.
func TestMailDomainCreateCompression(t *testing.T) {
	f := &fakeMailDomainAPI{}
	got, resp := createMailDomain(t, f, "zstd", 5)
	if resp.Diagnostics.HasError() || f.posts[0]["compression"] != "zstd" || f.posts[0]["compression_level"] != float64(5) ||
		got.Compression.ValueString() != "zstd" || got.CompressionLevel.ValueInt64() != 5 || !got.CompressedMail.ValueBool() {
		t.Fatalf("zstd: %v posts %v state %+v", resp.Diagnostics, f.posts, got)
	}

	for _, old := range []bool{false, true} {
		f = &fakeMailDomainAPI{old: old}
		got, resp = createMailDomain(t, f, "off", 0)
		if _, sent := f.posts[0]["compression"]; resp.Diagnostics.HasError() || sent || got.Compression.ValueString() != "off" {
			t.Fatalf("off (old %v): %v posts %v state %+v", old, resp.Diagnostics, f.posts, got)
		}
	}

	// Asked of an old apicpd: an error, and the domain it made is in state.
	f = &fakeMailDomainAPI{old: true}
	got, resp = createMailDomain(t, f, "zstd", 0)
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics[0].Detail(), "v0.8.0") || got.ID.ValueString() != "mdom_1" {
		t.Fatalf("old apicpd: %v state %+v", resp.Diagnostics, got)
	}
}

// Compression is sent on update only when it changes (apicp refuses a change
// while a compress job runs, and other changes must still go through).
func TestMailDomainUpdateCompression(t *testing.T) {
	f := &fakeMailDomainAPI{comp: "zstd", level: 3}
	r := &MailDomainResource{client: client.New(f.server(t).URL, "token")}
	sch := mailDomainSchema(t).Schema
	update := func(state, plan tftypes.Value) (MailDomainResourceModel, resource.UpdateResponse) {
		resp := resource.UpdateResponse{State: tfsdk.State{Schema: sch, Raw: state}}
		r.Update(context.Background(), resource.UpdateRequest{State: tfsdk.State{Schema: sch, Raw: state}, Plan: tfsdk.Plan{Schema: sch, Raw: plan}}, &resp)
		var got MailDomainResourceModel
		resp.State.Get(context.Background(), &got)
		return got, resp
	}
	got, resp := update(mailDomainValue("mdom_1", 0, "zstd", 3), mailDomainValue("mdom_1", 10, "zstd", 3))
	if _, sent := f.patch[0]["compression"]; resp.Diagnostics.HasError() || sent || got.MaxEmailsPerHour.ValueInt64() != 10 {
		t.Fatalf("limit only: %v patch %v", resp.Diagnostics, f.patch)
	}
	got, resp = update(mailDomainValue("mdom_1", 10, "zstd", 3), mailDomainValue("mdom_1", 10, "lz4", 0))
	if resp.Diagnostics.HasError() || f.patch[1]["compression"] != "lz4" || got.Compression.ValueString() != "lz4" {
		t.Fatalf("to lz4: %v patch %v", resp.Diagnostics, f.patch)
	}
	f.old = true
	_, resp = update(mailDomainValue("mdom_1", 10, "off", 0), mailDomainValue("mdom_1", 10, "gz", 0))
	if !resp.Diagnostics.HasError() {
		t.Fatal("compression on an old apicpd must be an error")
	}
}

func TestMailDomainValidateCompression(t *testing.T) {
	r := &MailDomainResource{}
	sch := mailDomainSchema(t).Schema
	for _, c := range []struct {
		comp  string
		level int64
		ok    bool
	}{
		{"off", 0, true}, {"zstd", 0, true}, {"zstd", 19, true}, {"gz", 9, true}, {"bz2", 1, true}, {"lz4", 0, true},
		{"xz", 0, false}, {"zstd", 23, false}, {"lz4", 1, false}, {"gz", 10, false}, {"off", 3, false},
	} {
		var resp resource.ValidateConfigResponse
		r.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: sch, Raw: mailDomainValue("mdom_1", 0, c.comp, c.level)}}, &resp)
		if resp.Diagnostics.HasError() == c.ok {
			t.Errorf("%s/%d: ok %v, got %v", c.comp, c.level, c.ok, resp.Diagnostics)
		}
	}
}
