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

// adminToolResource backs both apicp_phpmyadmin and apicp_phppgadmin
// (PLAN.md §8 phase 3) - identical shape and behavior, so implemented
// once and instantiated per tool name, rather than duplicating the
// Create/Read/Delete logic across two files. Deliberately has no
// settable arguments at all: declaring the resource enables the tool,
// removing it disables (and apicp fully uninstalls, not just hides) it -
// "either there or isn't", the user's own words for the toggle's design.
type adminToolResource struct {
	client   *client.Client
	tool     string
	typeName string
}

var _ resource.Resource = &adminToolResource{}
var _ resource.ResourceWithImportState = &adminToolResource{}

func NewPHPMyAdminResource() resource.Resource {
	return &adminToolResource{tool: "phpmyadmin", typeName: "phpmyadmin"}
}

func NewPHPPgAdminResource() resource.Resource {
	return &adminToolResource{tool: "phppgadmin", typeName: "phppgadmin"}
}

type AdminToolResourceModel struct {
	ID      types.String `tfsdk:"id"`
	Enabled types.Bool   `tfsdk:"enabled"`
	URL     types.String `tfsdk:"url"`
}

func (r *adminToolResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.typeName
}

func (r *adminToolResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf(
			"Enables %s on apicp's control-plane node - hosted only there, never per-account (PLAN.md §8 phase 3). Requires an admin-tier provider token. No configurable arguments: declaring this resource enables the tool, destroying it fully uninstalls the package (not just hides the vhost) so a disabled tool carries zero extra attack surface.",
			r.tool,
		),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{Computed: true},
			"url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Where the tool is reachable once enabled.",
			},
		},
	}
}

func (r *adminToolResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *adminToolResource) apply(st *client.AdminToolStatus, m *AdminToolResourceModel) {
	m.ID = types.StringValue(st.Tool)
	m.Enabled = types.BoolValue(st.Enabled)
	m.URL = types.StringValue(st.URL)
}

func (r *adminToolResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AdminToolResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	st, err := r.client.EnableAdminTool(r.tool)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Error enabling %s", r.tool), err.Error())
		return
	}
	r.apply(st, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *adminToolResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AdminToolResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	st, err := r.client.GetAdminTool(r.tool)
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Error reading %s", r.tool), err.Error())
		return
	}
	if !st.Enabled {
		// Disabled out-of-band (e.g. another admin called the API
		// directly) - the resource no longer reflects real state.
		resp.State.RemoveResource(ctx)
		return
	}
	r.apply(st, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update — every attribute is Computed, nothing in config can change;
// implemented to satisfy the resource.Resource interface.
func (r *adminToolResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AdminToolResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *adminToolResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.DisableAdminTool(r.tool); err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Error disabling %s", r.tool), err.Error())
	}
}

func (r *adminToolResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
