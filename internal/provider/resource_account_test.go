package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

func TestAccountCreateRefusalsAreReadable(t *testing.T) {
	for _, tc := range []struct {
		code             int
		body             string
		summary, details string
	}{
		{409, `{"error":"an account named \"Alice\" already exists (account names are unique, ignoring case)"}`, "Account name already in use", "terraform import"},
		{422, `{"error":"a user account's parent must be an admin or reseller account (parent_account_id is a user account)"}`, "Invalid account", "admin -> reseller -> user"},
		{422, `{"error":"admin accounts cannot have a package"}`, "Invalid account", "admin accounts cannot have a package"},
		{500, `{"error":"database locked"}`, "Error creating account", "database locked"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.code)
			_, _ = w.Write([]byte(tc.body))
		}))
		r := &AccountResource{client: client.New(srv.URL, "token")}
		sch := resourceSchema(t, r)
		plan := objectValue(t, sch, map[string]any{"id": unknownValue{}, "name": "alice", "tier": "user",
			"parent_account_id": "acct_u", "package_id": unknownValue{}, "created_at": unknownValue{}})
		resp := resource.CreateResponse{State: nullState(tfsdk.State{Schema: sch})}
		r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: sch, Raw: plan}}, &resp)
		srv.Close()

		if !resp.Diagnostics.HasError() {
			t.Fatalf("%d: no error", tc.code)
		}
		e := resp.Diagnostics.Errors()[0]
		if e.Summary() != tc.summary || !strings.Contains(e.Detail(), tc.details) {
			t.Errorf("%d: %q / %q", tc.code, e.Summary(), e.Detail())
		}
		if tc.code == 422 && strings.Contains(tc.body, "package") && strings.Contains(e.Detail(), "hierarchy") {
			t.Errorf("the hierarchy note is only for parent errors: %q", e.Detail())
		}
	}
}
