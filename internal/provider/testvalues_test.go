package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// unknownValue marks an attribute as unknown in objectValue.
type unknownValue struct{}

// resourceSchema is a resource's schema, checked.
func resourceSchema(t *testing.T, r resource.Resource) schema.Schema {
	t.Helper()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if d := resp.Schema.ValidateImplementation(context.Background()); d.HasError() {
		t.Fatalf("schema implementation: %v", d)
	}
	return resp.Schema
}

// objectValue builds a value of the schema's object type: attributes not in
// vals are null; unknownValue{} is unknown; []string fills a list or set of
// strings. Other values go to tftypes.NewValue as they are.
func objectValue(t *testing.T, sch schema.Schema, vals map[string]any) tftypes.Value {
	t.Helper()
	typ := sch.Type().TerraformType(context.Background()).(tftypes.Object)
	out := map[string]tftypes.Value{}
	for name, at := range typ.AttributeTypes {
		v, ok := vals[name]
		switch {
		case !ok:
			out[name] = tftypes.NewValue(at, nil)
		case v == (unknownValue{}):
			out[name] = tftypes.NewValue(at, tftypes.UnknownValue)
		default:
			if ss, isStrings := v.([]string); isStrings {
				elems := make([]tftypes.Value, len(ss))
				for i, s := range ss {
					elems[i] = tftypes.NewValue(tftypes.String, s)
				}
				out[name] = tftypes.NewValue(at, elems)
				continue
			}
			out[name] = tftypes.NewValue(at, v)
		}
	}
	for name := range vals {
		if _, ok := typ.AttributeTypes[name]; !ok {
			t.Fatalf("no attribute %q in the schema", name)
		}
	}
	return tftypes.NewValue(typ, out)
}
