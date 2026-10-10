package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

// FirewallSiteResource is apicp_firewall_site: a connection on a dynamic address,
// kept as a DNS name that follows it (the same resource apipbx has).
type FirewallSiteResource struct{ client *client.Client }

var _ resource.Resource = &FirewallSiteResource{}
var _ resource.ResourceWithImportState = &FirewallSiteResource{}

func NewFirewallSiteResource() resource.Resource { return &FirewallSiteResource{} }

type FirewallSiteResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Host      types.String `tfsdk:"host"`
	Enabled   types.Bool   `tfsdk:"enabled"`
	Ports     types.Set    `tfsdk:"ports"`
	Addresses types.Set    `tfsdk:"addresses"`
}

func (r *FirewallSiteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_site"
}

func (r *FirewallSiteResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A connection on a dynamic address (an office or home whose ISP renumbers it), kept as a DNS name that follows it (dynamic DNS, or any name you keep pointing at it). Its current addresses are let through every restricted port of the firewall (or only the listed `ports`), on top of each `apicp_firewall_restriction`'s own sources, and the firewall is re-applied when the name's address changes (checked every minute, `APICP_FIREWALL_REFRESH`; a lookup that fails keeps the last address). A restriction may then list no source of its own: \"SSH from my sites only\". A port restricted to sites whose names have not resolved yet stays closed, never open. Sites count for the lockout guard. Anything that can write to the name can open the firewall to its address: use a dynamic DNS account you control. Import with the site id.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: idUseState()},
			"name":    schema.StringAttribute{Required: true, MarkdownDescription: "A label, unique among sites."},
			"host":    schema.StringAttribute{Required: true, MarkdownDescription: "The DNS name that follows the connection (or a fixed address)."},
			"enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
			"ports": schema.SetAttribute{Optional: true, Computed: true, ElementType: types.Int64Type, Default: setdefault.StaticValue(types.SetValueMust(types.Int64Type, []attr.Value{})),
				MarkdownDescription: "Restrict the site to these firewall ports (22, 8080, 143...). Empty is every restricted port."},
			"addresses": schema.SetAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "What the name resolves to now."},
		},
	}
}

func (r *FirewallSiteResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (m *FirewallSiteResourceModel) set(s *client.FirewallSite) {
	m.ID, m.Name, m.Host, m.Enabled = types.StringValue(s.ID), types.StringValue(s.Name), types.StringValue(s.Host), types.BoolValue(s.Enabled)
	pv := make([]attr.Value, 0, len(s.Ports))
	for _, p := range s.Ports {
		pv = append(pv, types.Int64Value(int64(p)))
	}
	m.Ports = types.SetValueMust(types.Int64Type, pv)
	av := make([]attr.Value, 0, len(s.Addresses))
	for _, a := range s.Addresses {
		av = append(av, types.StringValue(a))
	}
	m.Addresses = types.SetValueMust(types.StringType, av)
}

func portsOf(ctx context.Context, s types.Set) []int {
	var raw []int64
	if s.IsNull() || s.IsUnknown() {
		return []int{}
	}
	_ = s.ElementsAs(ctx, &raw, false)
	out := make([]int, 0, len(raw))
	for _, p := range raw {
		out = append(out, int(p))
	}
	return out
}

func (r *FirewallSiteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m FirewallSiteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	en := m.Enabled.ValueBool()
	s, err := r.client.CreateFirewallSite(client.FirewallSiteInput{Name: m.Name.ValueString(), Host: m.Host.ValueString(), Enabled: &en, Ports: portsOf(ctx, m.Ports)})
	if err != nil {
		resp.Diagnostics.AddError("Error creating the firewall site", err.Error())
		return
	}
	m.set(s)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *FirewallSiteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m FirewallSiteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.client.GetFirewallSite(m.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading the firewall site", err.Error())
		return
	}
	m.set(s)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *FirewallSiteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m FirewallSiteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name, host, en, ports := m.Name.ValueString(), m.Host.ValueString(), m.Enabled.ValueBool(), portsOf(ctx, m.Ports)
	s, err := r.client.UpdateFirewallSite(m.ID.ValueString(), client.FirewallSitePatch{Name: &name, Host: &host, Enabled: &en, Ports: &ports})
	if err != nil {
		resp.Diagnostics.AddError("Error updating the firewall site", err.Error())
		return
	}
	m.set(s)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *FirewallSiteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m FirewallSiteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteFirewallSite(m.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Error removing the firewall site", err.Error())
	}
}

func (r *FirewallSiteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func idUseState() []planmodifier.String {
	return []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
}
