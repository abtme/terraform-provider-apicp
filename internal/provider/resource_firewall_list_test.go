package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestFirewallSchemasAreValid(t *testing.T) {
	for name, mk := range map[string]func() resource.Resource{
		"list": NewFirewallListResource, "rule": NewFirewallRuleResource, "restriction": NewFirewallRestrictionResource,
	} {
		var resp resource.SchemaResponse
		mk().Schema(context.Background(), resource.SchemaRequest{}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("%s schema: %v", name, resp.Diagnostics)
		}
		if d := resp.Schema.ValidateImplementation(context.Background()); d.HasError() {
			t.Fatalf("%s schema implementation: %v", name, d)
		}
	}
	var resp resource.SchemaResponse
	NewFirewallRuleResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if _, ok := resp.Schema.Attributes["list"]; !ok {
		t.Error("apicp_firewall_rule has no list")
	}
	NewFirewallRestrictionResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if _, ok := resp.Schema.Attributes["lists"]; !ok {
		t.Error("apicp_firewall_restriction has no lists")
	}
}

func TestCanonicalListSource(t *testing.T) {
	for in, want := range map[string]string{
		"203.0.113.5":          "203.0.113.5/32",
		"2001:DB8::1":          "2001:db8::1/128",
		"2001:db8:0::/32":      "2001:db8::/32",
		"::ffff:192.0.2.9":     "192.0.2.9/32",
		"::ffff:192.0.2.0/120": "192.0.2.0/24",
		"junk":                 "junk",
	} {
		if got := canonicalListSource(in); got != want {
			t.Errorf("canonicalListSource(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReconcileListSourcesKeepsTheUsersSpelling(t *testing.T) {
	ctx := context.Background()
	known := strSet(t, "2001:DB8::1", "203.0.113.5")
	got, d := reconcileListSources(ctx, known, []string{"203.0.113.5/32", "2001:db8::1/128"})
	if d.HasError() || !got.Equal(known) {
		t.Fatalf("same meaning must keep the configured value: %v %v", got, d)
	}
	got, _ = reconcileListSources(ctx, known, []string{"203.0.113.6/32"})
	if e := elems(t, got); len(e) != 1 || e[0] != "203.0.113.6/32" {
		t.Fatalf("drift must show apicp's value: %v", e)
	}
}

func TestReconcileNames(t *testing.T) {
	ctx := context.Background()
	if got, _ := reconcileNames(ctx, types.SetNull(types.StringType), nil); !got.IsNull() {
		t.Error("no lists configured and none reported must stay null")
	}
	known := strSet(t, "a", "b")
	if got, _ := reconcileNames(ctx, known, []string{"b", "a"}); !got.Equal(known) {
		t.Error("order must not matter")
	}
	if got, _ := reconcileNames(ctx, types.SetNull(types.StringType), []string{"x"}); len(elems(t, got)) != 1 {
		t.Error("lists added outside Terraform must show")
	}
	if got, _ := reconcileNames(ctx, strSet(t), nil); got.IsNull() {
		t.Error("an empty configured set stays empty, not null")
	}
}
