package provider

import (
	"context"
	"net"
	"net/netip"
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

// reconcileNames returns the value to store for a set of names (a
// restriction's lists): null stays null while apicp reports none, otherwise
// apicp's names.
func reconcileNames(ctx context.Context, known types.Set, api []string) (types.Set, diag.Diagnostics) {
	if len(api) == 0 && (known.IsNull() || known.IsUnknown()) {
		return types.SetNull(types.StringType), nil
	}
	if !known.IsNull() && !known.IsUnknown() {
		have, diags := sourcesFromSet(ctx, known)
		if diags.HasError() {
			return known, diags
		}
		if sameNames(have, api) {
			return known, nil
		}
	}
	elems := make([]attr.Value, 0, len(api))
	for _, s := range api {
		elems = append(elems, types.StringValue(s))
	}
	return types.SetValue(types.StringType, elems)
}

func sameNames(a, b []string) bool {
	set := map[string]bool{}
	for _, x := range a {
		set[x] = true
	}
	other := map[string]bool{}
	for _, x := range b {
		if !set[x] {
			return false
		}
		other[x] = true
	}
	return len(set) == len(other)
}

// canonicalListSource mirrors how apicp stores an address list entry
// (internal/firewall.NormalizeListSource): netip's canonical spelling, a bare
// address as /32 or /128, an IPv4-mapped address as IPv4. Entries apicp
// refuses are returned unchanged (apicp's error is the one to show).
func canonicalListSource(s string) string {
	s = strings.TrimSpace(s)
	var p netip.Prefix
	if strings.Contains(s, "/") {
		var err error
		if p, err = netip.ParsePrefix(s); err != nil {
			return s
		}
	} else {
		a, err := netip.ParseAddr(s)
		if err != nil {
			return s
		}
		p = netip.PrefixFrom(a, a.BitLen())
	}
	if a := p.Addr(); a.Is4In6() && p.Bits() >= 96 {
		p = netip.PrefixFrom(a.Unmap(), p.Bits()-96)
	}
	return p.String()
}

// reconcileListSources is reconcileSources for an address list's entries,
// compared the way apicp canonicalises them.
func reconcileListSources(ctx context.Context, known types.Set, api []string) (types.Set, diag.Diagnostics) {
	canon := func(xs []string) map[string]bool {
		out := map[string]bool{}
		for _, x := range xs {
			out[canonicalListSource(x)] = true
		}
		return out
	}
	if !known.IsNull() && !known.IsUnknown() {
		have, diags := sourcesFromSet(ctx, known)
		if diags.HasError() {
			return known, diags
		}
		a, b := canon(have), canon(api)
		same := len(a) == len(b)
		for k := range a {
			same = same && b[k]
		}
		if same {
			return known, nil
		}
	}
	elems := make([]attr.Value, 0, len(api))
	for _, s := range api {
		elems = append(elems, types.StringValue(s))
	}
	return types.SetValue(types.StringType, elems)
}
