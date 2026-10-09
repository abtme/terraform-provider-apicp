package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestAliasSetKeepsNullWhenNothingWasConfigured(t *testing.T) {
	ctx := context.Background()

	// attribute left out of the configuration, vhost has none: stays null (no perpetual diff)
	got, diags := aliasSet(ctx, nil, types.SetNull(types.StringType))
	if diags.HasError() || !got.IsNull() {
		t.Errorf("no aliases, none configured: got %v, %v", got, diags)
	}

	// explicitly configured empty: stays an empty set
	empty := types.SetValueMust(types.StringType, []attr.Value{})

	got, diags = aliasSet(ctx, nil, empty)
	if diags.HasError() || got.IsNull() || len(got.Elements()) != 0 {
		t.Errorf("no aliases, empty configured: got %v, %v", got, diags)
	}

	// aliases on the server always show, whatever the configuration said (drift becomes visible)
	got, diags = aliasSet(ctx, []string{"www.example.test"}, types.SetNull(types.StringType))
	if diags.HasError() || len(got.Elements()) != 1 {
		t.Errorf("aliases present: got %v, %v", got, diags)
	}
}

func TestAliasListRoundTrip(t *testing.T) {
	ctx := context.Background()

	set := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("b.example.test"), types.StringValue("a.example.test")})

	got, diags := aliasList(ctx, set)
	if diags.HasError() || len(got) != 2 {
		t.Fatalf("got %v, %v", got, diags)
	}

	if l, diags := aliasList(ctx, types.SetNull(types.StringType)); diags.HasError() || l != nil {
		t.Errorf("null set must give a nil list, got %v, %v", l, diags)
	}
}
