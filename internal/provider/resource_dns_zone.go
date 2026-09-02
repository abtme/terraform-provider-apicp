package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

var _ resource.Resource = &DNSZoneResource{}
var _ resource.ResourceWithImportState = &DNSZoneResource{}

func NewDNSZoneResource() resource.Resource { return &DNSZoneResource{} }

type DNSZoneResource struct{ client *client.Client }

type DNSZoneResourceModel struct {
	ID     types.String `tfsdk:"id"`
	Name   types.String `tfsdk:"name"`
	Status types.String `tfsdk:"status"`
}

func (r *DNSZoneResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_zone"
}

func (r *DNSZoneResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a DNS zone on apicp's own PowerDNS deployment. Created with no NS records — apicp does not manage registrar delegation.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Zone name, e.g. `example.com`. Stored canonically (trailing dot) by apicp regardless of how it's written here. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"status": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *DNSZoneResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	r.client = c
}

// applyZone strips the trailing dot apicp's API always adds (its own
// internal/dns.Canonical) back off before storing into state — Name is a
// Required (not Computed) attribute, so Terraform's post-apply
// consistency check requires the stored value to exactly equal what the
// caller configured; found live (2026-09-02) as a real "Provider produced
// inconsistent result after apply" error on the very first test apply.
func (r *DNSZoneResource) applyZone(z *client.Zone, m *DNSZoneResourceModel) {
	m.ID = types.StringValue(z.ID)
	m.Name = types.StringValue(strings.TrimSuffix(z.Name, "."))
	m.Status = types.StringValue(z.Status)
}

func (r *DNSZoneResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DNSZoneResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	z, err := r.client.CreateZone(plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating DNS zone", err.Error())
		return
	}
	r.applyZone(z, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DNSZoneResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DNSZoneResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	z, err := r.client.GetZone(state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading DNS zone", err.Error())
		return
	}
	r.applyZone(z, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update — name requires replace, so there's nothing left to update in place.
func (r *DNSZoneResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DNSZoneResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DNSZoneResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DNSZoneResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteZone(state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting DNS zone", err.Error())
	}
}

func (r *DNSZoneResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
