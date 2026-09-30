package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

// FirewallRestrictionResource is apicp_firewall_restriction: one of the
// firewall's fixed service ports (SSH, the API, mail, web, DNS...) limited to
// a list of source addresses. apicp's PUT is create-or-replace, so unlike
// apicp_firewall_rule this updates in place.
type FirewallRestrictionResource struct{ client *client.Client }

var _ resource.Resource = &FirewallRestrictionResource{}
var _ resource.ResourceWithImportState = &FirewallRestrictionResource{}

func NewFirewallRestrictionResource() resource.Resource { return &FirewallRestrictionResource{} }

type FirewallRestrictionResourceModel struct {
	ID      types.String `tfsdk:"id"`
	Port    types.Int64  `tfsdk:"port"`
	Sources types.Set    `tfsdk:"sources"`
	Comment types.String `tfsdk:"comment"`
	Force   types.Bool   `tfsdk:"force"`
}

func (r *FirewallRestrictionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_restriction"
}

func (r *FirewallRestrictionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Limits one of the firewall's fixed service ports to a list of source addresses, so only those reach it and everyone else is dropped. This is how to restrict apicp's own API (`8080`), SSH (`22`) or a mail/web/DNS port to certain IPs. Ports that can be restricted: 22, 25, 53, 80, 110, 143, 443, 587, 993, 995, 8080 (the API) and 8443 (agents). Like rules it is stored while `apicp_firewall` is off and takes effect when that is on. A family with no listed source has the port closed there, so restricting to IPv4 addresses only also closes it over IPv6.\n\n**Lockout protection.** For `22`, `8080` and `8443`, apicp refuses a restriction whose `sources` do not include the address Terraform is connecting from (apicp's own view of the connection, so behind a reverse proxy list the proxy), since that would lock you out with no remote way back. A caller on the node itself is always fine. For SSH only, `force = true` overrides it, for when the machine that uses SSH is not the one running Terraform. Restricting `8443` also cuts off every agent whose address is not listed. Destroying the resource opens the port to everyone again.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"port": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The service port to restrict. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"sources": schema.SetAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "CIDRs or bare IPs, any mix of IPv4 and IPv6, that may reach the port. At least one; `0.0.0.0/0` and `::/0` are refused (that is no restriction - destroy the resource instead).",
				Validators:          []validator.Set{nonEmptySet{}},
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Free-text note.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"force": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "SSH (`22`) only: allow a restriction that does not include the address Terraform connects from. Ignored for every other port.",
			},
		},
	}
}

func (r *FirewallRestrictionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *FirewallRestrictionResource) apply(ctx context.Context, rs *client.FirewallRestriction, m *FirewallRestrictionResourceModel) diag.Diagnostics {
	m.ID = types.StringValue(strconv.Itoa(rs.Port))
	m.Port = types.Int64Value(int64(rs.Port))
	m.Comment = types.StringValue(rs.Comment)
	// force is an input only apicp keeps for SSH; keep what was configured.
	if m.Force.IsNull() || m.Force.IsUnknown() {
		m.Force = types.BoolValue(rs.Force)
	}
	sources, diags := reconcileSources(ctx, m.Sources, rs.Sources)
	m.Sources = sources
	return diags
}

func (r *FirewallRestrictionResource) put(ctx context.Context, m *FirewallRestrictionResourceModel) diag.Diagnostics {
	sources, diags := sourcesFromSet(ctx, m.Sources)
	if diags.HasError() {
		return diags
	}
	rs, err := r.client.SetFirewallRestriction(int(m.Port.ValueInt64()), sources, m.Comment.ValueString(), m.Force.ValueBool())
	if err != nil {
		diags.AddError("Error setting the firewall restriction", err.Error())
		return diags
	}
	diags.Append(r.apply(ctx, rs, m)...)
	return diags
}

func (r *FirewallRestrictionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FirewallRestrictionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.put(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FirewallRestrictionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FirewallRestrictionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rs, err := r.client.GetFirewallRestriction(int(state.Port.ValueInt64()))
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading the firewall restriction", err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, rs, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *FirewallRestrictionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan FirewallRestrictionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.put(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FirewallRestrictionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state FirewallRestrictionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteFirewallRestriction(int(state.Port.ValueInt64())); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Error removing the firewall restriction", err.Error())
	}
}

// ImportState accepts the port number.
func (r *FirewallRestrictionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	port, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", "expected the restricted port number, e.g. 8080")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("port"), port)...)
}

// nonEmptySet rejects an empty set at plan time: a restriction with no
// sources would close the port to everyone, which is never what is meant
// (apicp refuses it too).
type nonEmptySet struct{}

func (nonEmptySet) Description(context.Context) string { return "must contain at least one element" }
func (v nonEmptySet) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (nonEmptySet) ValidateSet(_ context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if len(req.ConfigValue.Elements()) == 0 {
		resp.Diagnostics.AddAttributeError(req.Path, "Empty sources",
			"A restriction needs at least one source; with none it would close the port to everyone. Destroy the resource to open the port again.")
	}
}
