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

var mailSettingsType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{
	"id": tftypes.String, "max_message_size_mb": tftypes.Number, "default_max_message_size_mb": tftypes.Number,
}}

func mailSettingsSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	var resp resource.SchemaResponse
	NewMailSettingsResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if d := resp.Schema.ValidateImplementation(context.Background()); d.HasError() {
		t.Fatalf("schema implementation: %v", d)
	}
	return resp
}

func mailSettingsValue(id any, size, def any) tftypes.Value {
	v := func(ty tftypes.Type, x any) tftypes.Value {
		if x == "unknown" {
			return tftypes.NewValue(ty, tftypes.UnknownValue)
		}
		return tftypes.NewValue(ty, x)
	}
	return tftypes.NewValue(mailSettingsType, map[string]tftypes.Value{
		"id": v(tftypes.String, id), "max_message_size_mb": v(tftypes.Number, size), "default_max_message_size_mb": v(tftypes.Number, def),
	})
}

// fakeMailSettings is apicpd's /v1/mail/settings: puts records every PUT body.
type fakeMailSettings struct {
	size int64
	puts []map[string]any
}

func (f *fakeMailSettings) server(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/mail/settings" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch r.Method {
		case http.MethodPut:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("PUT body: %v", err)
			}
			f.puts = append(f.puts, body)
			f.size = int64(body["max_message_size_mb"].(float64))
		case http.MethodGet:
		default:
			t.Errorf("unexpected %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"max_message_size_mb": f.size, "default_max_message_size_mb": 50,
			"updated_at": "2026-10-24T15:00:00Z", "compression": "zstd"}) // a field this provider does not know yet
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestMailSettingsCreateSetsTheSize(t *testing.T) {
	f := &fakeMailSettings{size: 50}
	r := &MailSettingsResource{client: client.New(f.server(t).URL, "token")}
	sch := mailSettingsSchema(t).Schema

	resp := resource.CreateResponse{State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(mailSettingsType, nil)}}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: sch, Raw: mailSettingsValue("unknown", 0, "unknown")}}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if len(f.puts) != 1 || len(f.puts[0]) != 1 || f.puts[0]["max_message_size_mb"] != float64(0) {
		t.Fatalf("PUTs = %v, want one with only max_message_size_mb 0 (unlimited)", f.puts)
	}
	var got MailSettingsResourceModel
	resp.State.Get(context.Background(), &got)
	if got.ID.ValueString() != "mail_settings" || got.MaxMessageSizeMB.ValueInt64() != 0 || got.DefaultMaxMessageSizeMB.ValueInt64() != 50 {
		t.Errorf("state = %+v", got)
	}
}

func TestMailSettingsDeleteRestoresTheDefault(t *testing.T) {
	f := &fakeMailSettings{size: 200}
	r := &MailSettingsResource{client: client.New(f.server(t).URL, "token")}
	sch := mailSettingsSchema(t).Schema

	var resp resource.DeleteResponse
	r.Delete(context.Background(), resource.DeleteRequest{State: tfsdk.State{Schema: sch, Raw: mailSettingsValue("mail_settings", 200, 50)}}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if len(f.puts) != 1 || f.puts[0]["max_message_size_mb"] != float64(50) {
		t.Fatalf("PUTs = %v, want the server's default (50)", f.puts)
	}
}

func TestMailSettingsOldApicpdIsAClearError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	t.Cleanup(srv.Close)
	r := &MailSettingsResource{client: client.New(srv.URL, "token")}
	sch := mailSettingsSchema(t).Schema

	cresp := resource.CreateResponse{State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(mailSettingsType, nil)}}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: sch, Raw: mailSettingsValue("unknown", 50, "unknown")}}, &cresp)
	if !cresp.Diagnostics.HasError() || !strings.Contains(cresp.Diagnostics.Errors()[0].Detail(), "/v1/mail/settings") {
		t.Fatalf("create on an old apicpd: %v", cresp.Diagnostics)
	}

	state := tfsdk.State{Schema: sch, Raw: mailSettingsValue("mail_settings", 50, 50)}
	rresp := resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, &rresp)
	if !rresp.Diagnostics.HasError() || !strings.Contains(rresp.Diagnostics.Errors()[0].Detail(), "needs the release after v0.6.18") {
		t.Fatalf("read on an old apicpd must be an error, not a silent removal: %v", rresp.Diagnostics)
	}
	if rresp.State.Raw.IsNull() {
		t.Fatal("state must not be dropped")
	}
}

func TestMailSettingsValidateConfig(t *testing.T) {
	sch := mailSettingsSchema(t).Schema
	for size, wantErr := range map[int64]bool{0: false, 1: false, 50: false, 10240: false, -1: true, 10241: true} {
		var resp resource.ValidateConfigResponse
		NewMailSettingsResource().(resource.ResourceWithValidateConfig).ValidateConfig(context.Background(),
			resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: sch, Raw: mailSettingsValue(nil, size, nil)}}, &resp)
		if got := resp.Diagnostics.HasError(); got != wantErr {
			t.Errorf("max_message_size_mb %d: error=%v, want %v", size, got, wantErr)
		}
	}
}
