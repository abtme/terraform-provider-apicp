package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

var _ resource.Resource = &DNSRecordResource{}
var _ resource.ResourceWithImportState = &DNSRecordResource{}

func NewDNSRecordResource() resource.Resource { return &DNSRecordResource{} }

type DNSRecordResource struct{ client *client.Client }

type DNSRecordResourceModel struct {
	ID      types.String `tfsdk:"id"`
	ZoneID  types.String `tfsdk:"zone_id"`
	Name    types.String `tfsdk:"name"`
	Type    types.String `tfsdk:"type"`
	Content types.String `tfsdk:"content"`
	TTL     types.Int64  `tfsdk:"ttl"`
	Status  types.String `tfsdk:"status"`
}

func (r *DNSRecordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_record"
}

func (r *DNSRecordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one DNS record within an `apicp_dns_zone`. PowerDNS has no per-record concept, only whole rrsets — apicp recomputes the full rrset for this record's (zone, name, type) on every create/update/delete, so multiple `apicp_dns_record`s sharing one name+type (e.g. round-robin `A`s) compose correctly.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"zone_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the `apicp_dns_zone` this record belongs to. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record name, relative to the zone (`@` for the zone apex, `www` for a label, or a fully-qualified name ending in `.`). Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record type: `A`, `AAAA`, `CNAME`, `TXT`, `MX`, `NS`, `SRV`, or `CAA`. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"content": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record content/value.",
			},
			"ttl": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "TTL in seconds. Defaults to apicp's own default (300) if omitted.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"status": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *DNSRecordResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DNSRecordResource) applyRecord(rec *client.Record, m *DNSRecordResourceModel) {
	m.ID = types.StringValue(rec.ID)
	m.ZoneID = types.StringValue(rec.ZoneID)
	m.Name = types.StringValue(rec.Name)
	m.Type = types.StringValue(rec.Type)
	m.Content = types.StringValue(rec.Content)
	m.TTL = types.Int64Value(int64(rec.TTL))
	m.Status = types.StringValue(rec.Status)
}

func (r *DNSRecordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DNSRecordResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var ttl int
	if !plan.TTL.IsNull() && !plan.TTL.IsUnknown() {
		ttl = int(plan.TTL.ValueInt64())
	}

	rec, err := r.client.CreateRecord(plan.ZoneID.ValueString(), plan.Name.ValueString(), plan.Type.ValueString(), plan.Content.ValueString(), ttl)
	if err != nil {
		resp.Diagnostics.AddError("Error creating DNS record", err.Error())
		return
	}
	r.applyRecord(rec, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DNSRecordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DNSRecordResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rec, err := r.client.GetRecord(state.ZoneID.ValueString(), state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading DNS record", err.Error())
		return
	}
	r.applyRecord(rec, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DNSRecordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state DNSRecordResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ttl := int(state.TTL.ValueInt64())
	if !plan.TTL.IsNull() && !plan.TTL.IsUnknown() {
		ttl = int(plan.TTL.ValueInt64())
	}

	rec, err := r.client.UpdateRecord(state.ZoneID.ValueString(), state.ID.ValueString(), plan.Content.ValueString(), ttl)
	if err != nil {
		resp.Diagnostics.AddError("Error updating DNS record", err.Error())
		return
	}
	r.applyRecord(rec, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DNSRecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DNSRecordResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteRecord(state.ZoneID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting DNS record", err.Error())
	}
}

// ImportState accepts "zone_id/record_id".
func (r *DNSRecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected format: zone_id/record_id")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("zone_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
