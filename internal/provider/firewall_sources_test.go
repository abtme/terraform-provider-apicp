package provider

import (
	"context"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestCanonicalSource(t *testing.T) {
	for in, want := range map[string]string{
		"203.0.113.5":    "203.0.113.5/32",
		"203.0.113.0/24": "203.0.113.0/24",
		"2001:db8::1":    "2001:db8::1/128",
		"2001:db8::/32":  "2001:db8::/32",
		" 203.0.113.5 ":  "203.0.113.5/32",
		"":               "",
		"junk":           "junk", // left for apicp to reject
	} {
		if got := canonicalSource(in); got != want {
			t.Errorf("canonicalSource(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSameSourcesIgnoresSpellingOrderAndDuplicates(t *testing.T) {
	if !sameSources([]string{"203.0.113.5", "2001:db8::/32"}, []string{"2001:db8::/32", "203.0.113.5/32", "203.0.113.5/32"}) {
		t.Error("bare IP vs /32, order and duplicates must not matter")
	}
	if sameSources([]string{"203.0.113.5"}, []string{"203.0.113.6"}) {
		t.Error("different addresses must differ")
	}
	if sameSources([]string{"203.0.113.5"}, []string{"203.0.113.5", "10.0.0.0/8"}) {
		t.Error("an extra source must differ")
	}
	if !sameSources(nil, []string{}) {
		t.Error("nil and empty are the same")
	}
}

func strSet(t *testing.T, vals ...string) types.Set {
	t.Helper()
	elems := make([]attr.Value, len(vals))
	for i, v := range vals {
		elems[i] = types.StringValue(v)
	}
	s, diags := types.SetValue(types.StringType, elems)
	if diags.HasError() {
		t.Fatal(diags)
	}
	return s
}

func elems(t *testing.T, s types.Set) []string {
	t.Helper()
	out, diags := sourcesFromSet(context.Background(), s)
	if diags.HasError() {
		t.Fatal(diags)
	}
	return out
}

// The user's own spelling must survive apply and refresh when apicp's
// canonical form means the same thing - otherwise Terraform reports an
// inconsistent result after apply, or a diff on every plan.
func TestReconcileSourcesKeepsTheConfiguredSpelling(t *testing.T) {
	configured := strSet(t, "203.0.113.5", "2001:db8::1")
	got, diags := reconcileSources(context.Background(), configured, []string{"2001:db8::1/128", "203.0.113.5/32"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if !got.Equal(configured) {
		t.Errorf("got %v, want the configured %v unchanged", got, configured)
	}
}

// Real drift - someone changed the list outside Terraform - is reported.
func TestReconcileSourcesReportsDrift(t *testing.T) {
	got, diags := reconcileSources(context.Background(), strSet(t, "203.0.113.5"), []string{"203.0.113.5/32", "198.51.100.0/24"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if want := []string{"198.51.100.0/24", "203.0.113.5/32"}; !reflect.DeepEqual(canonicalSourceSet(elems(t, got)), want) {
		t.Errorf("drift not reported: got %v, want %v", elems(t, got), want)
	}
}

// Nothing configured (unknown at plan time, or null after an import) takes
// apicp's list, or an empty known set when it has none.
func TestReconcileSourcesWithNothingConfigured(t *testing.T) {
	ctx := context.Background()
	got, _ := reconcileSources(ctx, types.SetUnknown(types.StringType), []string{"203.0.113.5/32"})
	if got.IsUnknown() || got.IsNull() || !reflect.DeepEqual(elems(t, got), []string{"203.0.113.5/32"}) {
		t.Errorf("unknown must resolve to apicp's list, got %v", got)
	}
	got, _ = reconcileSources(ctx, types.SetNull(types.StringType), nil)
	if got.IsUnknown() || got.IsNull() || len(got.Elements()) != 0 {
		t.Errorf("no sources anywhere must be a known empty set, got %v", got)
	}
}
