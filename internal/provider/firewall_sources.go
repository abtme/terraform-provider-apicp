package provider

import (
	"context"
	"net"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// canonicalSource mirrors how apicp stores a firewall source: a bare IP gets
// an implicit /32 or /128, a CIDR is kept as written. Without this, a config
// of "203.0.113.5" would differ from apicp's "203.0.113.5/32" and Terraform
// would report an inconsistent result after apply and a perpetual diff.
func canonicalSource(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.Contains(s, "/") {
		return s
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return s
	}
	if ip.To4() != nil {
		return s + "/32"
	}
	return s + "/128"
}

// canonicalSourceSet is the canonical, de-duplicated, sorted form of sources,
// so two lists that mean the same thing compare equal whatever their order.
func canonicalSourceSet(sources []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(sources))
	for _, s := range sources {
		c := canonicalSource(s)
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}

func sameSources(a, b []string) bool {
	ca, cb := canonicalSourceSet(a), canonicalSourceSet(b)
	if len(ca) != len(cb) {
		return false
	}
	for i := range ca {
		if ca[i] != cb[i] {
			return false
		}
	}
	return true
}

// sourcesFromSet reads a set of strings; null and unknown are an empty list.
func sourcesFromSet(ctx context.Context, set types.Set) ([]string, diag.Diagnostics) {
	if set.IsNull() || set.IsUnknown() {
		return nil, nil
	}
	var out []string
	diags := set.ElementsAs(ctx, &out, false)
	return out, diags
}

// reconcileSources returns the value to store for a sources attribute: the
// planned (or previously stored) value when it means the same as what apicp
// reports - so the user's own spelling is kept and no diff appears - and
// apicp's canonical list when it really differs (drift) or nothing was
// specified.
func reconcileSources(ctx context.Context, known types.Set, api []string) (types.Set, diag.Diagnostics) {
	if !known.IsNull() && !known.IsUnknown() {
		have, diags := sourcesFromSet(ctx, known)
		if diags.HasError() {
			return known, diags
		}
		if sameSources(have, api) {
			return known, nil
		}
	}
	elems := make([]attr.Value, 0, len(api))
	for _, s := range canonicalSourceSet(api) {
		elems = append(elems, types.StringValue(s))
	}
	return types.SetValue(types.StringType, elems)
}
