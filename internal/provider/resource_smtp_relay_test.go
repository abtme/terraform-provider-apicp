package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

func relaySchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	var resp resource.SchemaResponse
	NewSMTPRelayResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", resp.Diagnostics)
	}
	if d := resp.Schema.ValidateImplementation(context.Background()); d.HasError() {
		t.Fatalf("schema implementation: %v", d)
	}
	return resp
}

// config builds a resource config with the given username/password (nil = omitted, a *string = set).
func relayConfig(t *testing.T, username, password *string, unknown ...string) tfsdk.Config {
	t.Helper()
	sch := relaySchema(t).Schema
	val := func(name string, v *string) tftypes.Value {
		for _, u := range unknown {
			if u == name {
				return tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
			}
		}
		if v == nil {
			return tftypes.NewValue(tftypes.String, nil)
		}
		return tftypes.NewValue(tftypes.String, *v)
	}
	attrs := map[string]tftypes.Type{"id": tftypes.String, "node_id": tftypes.String, "host": tftypes.String, "port": tftypes.Number,
		"username": tftypes.String, "password": tftypes.String, "status": tftypes.String}
	host, node := "relay.example.com", "node-1"
	return tfsdk.Config{Schema: sch, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attrs}, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, nil), "node_id": tftypes.NewValue(tftypes.String, node),
		"host": tftypes.NewValue(tftypes.String, host), "port": tftypes.NewValue(tftypes.Number, 587),
		"username": val("username", username), "password": val("password", password), "status": tftypes.NewValue(tftypes.String, nil),
	})}
}

func str(s string) *string { return &s }

func TestSMTPRelaySchemaIsValidAndLoginIsOptional(t *testing.T) {
	sch := relaySchema(t).Schema
	for _, name := range []string{"username", "password"} {
		a := sch.Attributes[name]
		if a.IsRequired() || !a.IsOptional() {
			t.Errorf("%s must be optional: a relay that accepts mail by source address needs no login", name)
		}
	}
	if !sch.Attributes["password"].IsSensitive() {
		t.Error("password must stay sensitive")
	}
}

func TestSMTPRelayValidateConfig(t *testing.T) {
	cases := []struct {
		name     string
		user, pw *string
		unknown  []string
		wantErr  bool
	}{
		{"login", str("apikey"), str("s3cret"), nil, false},
		{"no login at all", nil, nil, nil, false},
		{"username only", str("apikey"), nil, nil, true},
		{"password only", nil, str("s3cret"), nil, true},
		{"empty username", str(""), str("p"), nil, true},
		{"empty password", str("u"), str(""), nil, true},
		{"both empty strings (meant 'none')", str(""), str(""), nil, true},
		{"password not known yet (from another resource)", str("u"), nil, []string{"password"}, false},
		{"username not known yet", nil, str("p"), []string{"username"}, false},
	}
	for _, c := range cases {
		var resp resource.ValidateConfigResponse
		NewSMTPRelayResource().(resource.ResourceWithValidateConfig).ValidateConfig(context.Background(),
			resource.ValidateConfigRequest{Config: relayConfig(t, c.user, c.pw, c.unknown...)}, &resp)
		if got := resp.Diagnostics.HasError(); got != c.wantErr {
			t.Errorf("%s: error=%v, want %v (%v)", c.name, got, c.wantErr, resp.Diagnostics)
		}
	}
}

// With no login apicp returns username "": the state must be null to match a planned null,
// or Terraform fails the apply with "inconsistent result".
func TestSMTPRelayApplyKeepsUsernameNullWithoutALogin(t *testing.T) {
	r := &SMTPRelayResource{}
	m := SMTPRelayResourceModel{Username: types.StringNull(), Password: types.StringNull()}
	r.apply(&client.SMTPRelay{NodeID: "node-1", Host: "relay.internal.example", Port: 25, Username: "", Status: "active"}, &m)
	if !m.Username.IsNull() {
		t.Fatalf("username = %v, want null", m.Username)
	}
	if m.Host.ValueString() != "relay.internal.example" || m.Port.ValueInt64() != 25 || m.ID.ValueString() != "node-1" {
		t.Fatalf("%+v", m)
	}
	if !m.Password.IsNull() {
		t.Fatal("the password must never be touched")
	}
}

func TestSMTPRelayApplyKeepsALoginAndTheWriteOnlyPassword(t *testing.T) {
	r := &SMTPRelayResource{}
	m := SMTPRelayResourceModel{Password: types.StringValue("s3cret")}
	r.apply(&client.SMTPRelay{NodeID: "node-1", Host: "smtp.example.com", Port: 587, Username: "apikey", Status: "active"}, &m)
	if m.Username.ValueString() != "apikey" || m.Password.ValueString() != "s3cret" {
		t.Fatalf("%+v", m)
	}
}
