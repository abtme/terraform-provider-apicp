package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

// FirewallRuleResource is apicp_firewall_rule (PLAN.md §8 phase 7) - a
// per-rule CRUD resource for apicp's node-wide default-DROP firewall.
// apicp exposes no update endpoint for a rule (internal/api/firewall.go
// only has Create/List/Get/Delete) - every attribute forces a replace.
// Rules are evaluated in creation order, before the fixed baseline
// allow-list (see apicp_firewall's docs) - a specific override (e.g.
// "drop port 22 from one attacker IP") takes effect rather than being
// shadowed by the baseline's generic "allow 22 from anywhere".
type FirewallRuleResource struct {
	client *client.Client
}

var _ resource.Resource = &FirewallRuleResource{}
var _ resource.ResourceWithImportState = &FirewallRuleResource{}

func NewFirewallRuleResource() resource.Resource { return &FirewallRuleResource{} }

type FirewallRuleResourceModel struct {
	ID       types.String `tfsdk:"id"`
	Source   types.String `tfsdk:"source"`
	Sources  types.Set    `tfsdk:"sources"`
	Port     types.String `tfsdk:"port"`
	Protocol types.String `tfsdk:"protocol"`
	Action   types.String `tfsdk:"action"`
	Comment  types.String `tfsdk:"comment"`
}

func (r *FirewallRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_rule"
}

func (r *FirewallRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one rule in apicp's node-wide default-DROP firewall (PLAN.md §8 phase 7, requires `apicp_firewall` to actually be enabled for this to have any live effect). apicp refuses to create a drop/reject rule with no `source` restriction that would block SSH (22) or apicp's own control-plane API/agent ports (8080/8443) for everyone - the single mistake most likely to cause an unrecoverable lockout with no remote recovery path.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"source": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "CIDR (v4 or v6) or bare IP this rule applies to. Omit for \"anywhere\" (`0.0.0.0/0`/`::/0`). Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"sources": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Further CIDRs or bare IPs (any mix of IPv4 and IPv6) this rule applies to, in addition to `source`. One rule covers all of them. With neither `source` nor `sources` the rule applies to anywhere. A drop/reject rule for SSH/8080/8443 is only accepted when every source is specific. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.Set{setplanmodifier.RequiresReplace(), setplanmodifier.UseStateForUnknown()},
			},
			"port": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Single port (`\"22\"`) or inclusive range (`\"1000:2000\"`). Omit for all ports. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"protocol": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`tcp`, `udp`, or `all`. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"action": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`accept`, `drop`, or `reject`. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Free-text note. Changing this replaces the resource (no update endpoint exists server-side).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
		},
	}
}

func (r *FirewallRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *FirewallRuleResource) applyRule(ctx context.Context, rule *client.FirewallRule, m *FirewallRuleResourceModel) diag.Diagnostics {
	m.ID = types.StringValue(rule.ID)
	m.Source = types.StringValue(rule.Source)
	sources, diags := reconcileSources(ctx, m.Sources, rule.Sources)
	m.Sources = sources
	m.Port = types.StringValue(rule.Port)
	m.Protocol = types.StringValue(rule.Protocol)
	m.Action = types.StringValue(rule.Action)
	m.Comment = types.StringValue(rule.Comment)
	return diags
}

func (r *FirewallRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FirewallRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sources, diags := sourcesFromSet(ctx, plan.Sources)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	rule, err := r.client.CreateFirewallRule(
		plan.Source.ValueString(), sources, plan.Port.ValueString(),
		plan.Protocol.ValueString(), plan.Action.ValueString(), plan.Comment.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Error creating firewall rule", err.Error())
		return
	}
	resp.Diagnostics.Append(r.applyRule(ctx, rule, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FirewallRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FirewallRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rule, err := r.client.GetFirewallRule(state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading firewall rule", err.Error())
		return
	}
	resp.Diagnostics.Append(r.applyRule(ctx, rule, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update — every attribute forces a replace (see Schema), so this is
// unreachable in practice; implemented to satisfy the resource.Resource
// interface.
func (r *FirewallRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan FirewallRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FirewallRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state FirewallRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteFirewallRule(state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting firewall rule", err.Error())
	}
}

func (r *FirewallRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
