package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

// Create waits for apicp's backup run and keeps a run warning as a warning,
// not an error: the backup was made.
func TestAccountBackupCreateWaitsAndWarns(t *testing.T) {
	old := client.AccountBackupPollInterval
	client.AccountBackupPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { client.AccountBackupPollInterval = old })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/accounts/acct_1/backups":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"run_id":"arun_1","account_id":"acct_1","status":"running","stage":"starting"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/accounts/acct_1/backups/runs/arun_1":
			_, _ = w.Write([]byte(`{"run_id":"arun_1","account_id":"acct_1","status":"finished","warning":"old backups could not be removed",
				"backup":{"id":"bkp_1","account_id":"acct_1","created_at":"2026-10-22T10:00:00Z","size_bytes":42}}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)

	r := &AccountBackupResource{client: client.New(srv.URL, "token")}
	var sch resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &sch)

	typ := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id": tftypes.String, "account_id": tftypes.String, "created_at": tftypes.String, "size_bytes": tftypes.Number,
	}}
	unknown := func(ty tftypes.Type) tftypes.Value { return tftypes.NewValue(ty, tftypes.UnknownValue) }
	plan := tfsdk.Plan{Schema: sch.Schema, Raw: tftypes.NewValue(typ, map[string]tftypes.Value{
		"id": unknown(tftypes.String), "account_id": tftypes.NewValue(tftypes.String, "acct_1"),
		"created_at": unknown(tftypes.String), "size_bytes": unknown(tftypes.Number),
	})}

	resp := resource.CreateResponse{State: tfsdk.State{Schema: sch.Schema, Raw: tftypes.NewValue(typ, nil)}}
	r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() != 1 || !strings.Contains(resp.Diagnostics.Warnings()[0].Detail(), "old backups could not be removed") {
		t.Errorf("want one warning about the prune, got %v", resp.Diagnostics)
	}

	var got AccountBackupResourceModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &got)...)
	if got.ID.ValueString() != "bkp_1" || got.SizeBytes.ValueInt64() != 42 || got.AccountID.ValueString() != "acct_1" {
		t.Errorf("state = %+v", got)
	}
}
