package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

var _ resource.Resource = &WebDomainResource{}
var _ resource.ResourceWithImportState = &WebDomainResource{}

func NewWebDomainResource() resource.Resource { return &WebDomainResource{} }

type WebDomainResource struct{ client *client.Client }

type WebDomainResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Domain       types.String `tfsdk:"domain"`
	NodeID       types.String `tfsdk:"node_id"`
	DocumentRoot types.String `tfsdk:"document_root"`
	UnixUser     types.String `tfsdk:"unix_user"`
	Status       types.String `tfsdk:"status"`
	TLSEnabled   types.Bool   `tfsdk:"tls_enabled"`
}

func (r *WebDomainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_web_domain"
}

func (r *WebDomainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a web vhost (nginx-served domain) in apicp.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"domain": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Domain name, e.g. `example.com`. Changing this renames the vhost in place (apicp removes the old nginx config and reapplies under the new domain) rather than replacing the resource.",
			},
			"node_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The apicp node this vhost is provisioned on. apicp's current single-node MVP always assigns its one configured node — this attribute is read-only, not settable, since the API does not yet honor a caller-supplied node selection.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"document_root": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Filesystem path apicp serves this domain from.",
			},
			"unix_user": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Per-domain Unix user apicp created for filesystem isolation.",
			},
			"status": schema.StringAttribute{
				Computed: true,
			},
			"tls_enabled": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether a TLS certificate is currently installed — managed via `apicp_tls_certificate`, not directly on this resource.",
			},
		},
	}
}

func (r *WebDomainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *WebDomainResource) applyVhost(v *client.Vhost, m *WebDomainResourceModel) {
	m.ID = types.StringValue(v.ID)
	m.Domain = types.StringValue(v.Domain)
	m.NodeID = types.StringValue(v.NodeID)
	m.DocumentRoot = types.StringValue(v.DocumentRoot)
	m.UnixUser = types.StringValue(v.UnixUser)
	m.Status = types.StringValue(v.Status)
	m.TLSEnabled = types.BoolValue(v.TLSEnabled)
}

func (r *WebDomainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan WebDomainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	v, err := r.client.CreateVhost(plan.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating web domain", err.Error())
		return
	}
	r.applyVhost(v, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *WebDomainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state WebDomainResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	v, err := r.client.GetVhost(state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading web domain", err.Error())
		return
	}
	r.applyVhost(v, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *WebDomainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state WebDomainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	v, err := r.client.RenameVhost(state.ID.ValueString(), plan.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error renaming web domain", err.Error())
		return
	}
	r.applyVhost(v, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *WebDomainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state WebDomainResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteVhost(state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting web domain", err.Error())
	}
}

func (r *WebDomainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
