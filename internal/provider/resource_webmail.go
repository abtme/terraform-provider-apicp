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

// WebmailResource is apicp_webmail (PLAN.md §8 phase 4) - same zero-
// argument, admin-gated toggle shape as apicp_phpmyadmin/apicp_phppgadmin
// (see resource_admin_tool.go), but standalone since there's only one
// webmail app - no per-tool instantiation needed.
type WebmailResource struct {
	client *client.Client
}

var _ resource.Resource = &WebmailResource{}
var _ resource.ResourceWithImportState = &WebmailResource{}

func NewWebmailResource() resource.Resource { return &WebmailResource{} }

type WebmailResourceModel struct {
	ID      types.String `tfsdk:"id"`
	Enabled types.Bool   `tfsdk:"enabled"`
	URL     types.String `tfsdk:"url"`
}

func (r *WebmailResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webmail"
}

func (r *WebmailResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Enables Roundcube webmail on apicp's control-plane node - hosted only there, never per-account (PLAN.md §8 phase 4). Requires an admin-tier provider token. No configurable arguments: declaring this resource enables the tool, destroying it fully uninstalls the package. Real mailbox owners log into Roundcube's own form with their genuine `apicp_mailbox` credentials - this resource only controls whether the software exists at all.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{Computed: true},
			"url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Where Roundcube is reachable once enabled.",
			},
		},
	}
}

func (r *WebmailResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *WebmailResource) apply(st *client.WebmailStatus, m *WebmailResourceModel) {
	m.ID = types.StringValue("webmail")
	m.Enabled = types.BoolValue(st.Enabled)
	m.URL = types.StringValue(st.URL)
}

func (r *WebmailResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan WebmailResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	st, err := r.client.EnableWebmail()
	if err != nil {
		resp.Diagnostics.AddError("Error enabling webmail", err.Error())
		return
	}
	r.apply(st, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *WebmailResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state WebmailResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	st, err := r.client.GetWebmail()
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading webmail", err.Error())
		return
	}
	if !st.Enabled {
		// Disabled out-of-band - the resource no longer reflects real state.
		resp.State.RemoveResource(ctx)
		return
	}
	r.apply(st, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update — every attribute is Computed, nothing in config can change;
// implemented to satisfy the resource.Resource interface.
func (r *WebmailResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan WebmailResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *WebmailResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.DisableWebmail(); err != nil {
		resp.Diagnostics.AddError("Error disabling webmail", err.Error())
	}
}

func (r *WebmailResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
