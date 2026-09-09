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

// FirewallResource is apicp_firewall (PLAN.md §8 phase 7) - the
// node-wide default-DROP allowlist enforcement toggle. Same zero-
// argument admin-gated shape as apicp_webmail: declaring this resource
// enables enforcement (with every currently-declared apicp_firewall_rule
// applied, plus apicp's own fixed baseline of SSH/HTTP/HTTPS/SMTP/IMAP/
// IMAPS/DNS/8080/8443 - always allowed regardless of any rule), and
// destroying it disables enforcement entirely, returning the node to
// exactly as open as it was before this feature was ever used.
type FirewallResource struct {
	client *client.Client
}

var _ resource.Resource = &FirewallResource{}
var _ resource.ResourceWithImportState = &FirewallResource{}

func NewFirewallResource() resource.Resource { return &FirewallResource{} }

type FirewallResourceModel struct {
	ID      types.String `tfsdk:"id"`
	Enabled types.Bool   `tfsdk:"enabled"`
}

func (r *FirewallResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall"
}

func (r *FirewallResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Enables apicp's node-wide default-DROP firewall enforcement (PLAN.md §8 phase 7). Requires an admin-tier provider token. No configurable arguments: declaring this resource enables enforcement, destroying it disables enforcement entirely (stored `apicp_firewall_rule`s are left alone server-side and reapplied if enforcement is re-enabled later). SSH, apicp's own control-plane API/agent ports, and every real service this node runs (HTTP/S, SMTP, IMAP/IMAPS, DNS) are always allowed regardless of any rule - see apicp's own docs for the fixed baseline.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{Computed: true},
		},
	}
}

func (r *FirewallResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *FirewallResource) apply(cfg *client.FirewallConfig, m *FirewallResourceModel) {
	m.ID = types.StringValue("firewall")
	m.Enabled = types.BoolValue(cfg.Enabled)
}

func (r *FirewallResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FirewallResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg, err := r.client.EnableFirewall()
	if err != nil {
		resp.Diagnostics.AddError("Error enabling firewall", err.Error())
		return
	}
	r.apply(cfg, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FirewallResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FirewallResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg, err := r.client.GetFirewallConfig()
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading firewall config", err.Error())
		return
	}
	if !cfg.Enabled {
		// Disabled out-of-band - the resource no longer reflects real state.
		resp.State.RemoveResource(ctx)
		return
	}
	r.apply(cfg, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update — every attribute is Computed, nothing in config can change;
// implemented to satisfy the resource.Resource interface.
func (r *FirewallResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan FirewallResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FirewallResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.DisableFirewall(); err != nil {
		resp.Diagnostics.AddError("Error disabling firewall", err.Error())
	}
}

func (r *FirewallResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
