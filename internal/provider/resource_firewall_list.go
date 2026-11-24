package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

// FirewallListResource is apicp_firewall_list: a named set of addresses used by
// name from accept rules (apicp_firewall_rule.list) and restrictions
// (apicp_firewall_restriction.lists). apicp's PATCH changes it in place.
type FirewallListResource struct{ client *client.Client }

var _ resource.Resource = &FirewallListResource{}
var _ resource.ResourceWithImportState = &FirewallListResource{}

func NewFirewallListResource() resource.Resource { return &FirewallListResource{} }

type FirewallListResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Sources   types.Set    `tfsdk:"sources"`
	Comment   types.String `tfsdk:"comment"`
	Addresses types.Set    `tfsdk:"addresses"`
}

func (r *FirewallListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_list"
}

func (r *FirewallListResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A named firewall address list: addresses and CIDRs (IPv4 and IPv6 mixed) kept once and used by name from accept rules (`apicp_firewall_rule`'s `list`) and restrictions (`apicp_firewall_restriction`'s `lists`). Changing it changes every rule and restriction that uses it at once; a change that would take your own address out of a restricted SSH/API/agent port is refused. apicp refuses to delete or rename a list while a rule or restriction uses it, so reference it (`apicp_firewall_list.x.name`) to get the order right. apicp also has built-in, read-only templates usable by name the same way (`ovh-monitoring`: OVHcloud's dedicated-server monitoring, `apicp firewall templates list`); they are not resources. Import with the list's id or name.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: idUseState()},
			"name": schema.StringAttribute{Required: true,
				MarkdownDescription: "a-z, 0-9 and -, starting with a letter; unique, and not a built-in template's name. Rules and restrictions use the list by this name."},
			"sources": schema.SetAttribute{Required: true, ElementType: types.StringType,
				MarkdownDescription: "Addresses or CIDRs (at least one, at most 1000). A bare address means /32 or /128. A CIDR with host bits set (`192.0.2.5/24`) and every-address ranges (`0.0.0.0/0`, `::/0`) are refused."},
			"comment": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("")},
			"addresses": schema.SetAttribute{Computed: true, ElementType: types.StringType,
				MarkdownDescription: "What the list lets in now (apicp's canonical form of `sources`)."},
		},
	}
}

func (r *FirewallListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (m *FirewallListResourceModel) set(ctx context.Context, l *client.FirewallList) diag.Diagnostics {
	m.ID, m.Name, m.Comment = types.StringValue(l.ID), types.StringValue(l.Name), types.StringValue(l.Comment)
	sources, diags := reconcileListSources(ctx, m.Sources, l.Sources)
	m.Sources = sources
	av := make([]attr.Value, 0, len(l.Addresses))
	for _, a := range l.Addresses {
		av = append(av, types.StringValue(a))
	}
	m.Addresses = types.SetValueMust(types.StringType, av)
	return diags
}

func (r *FirewallListResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m FirewallListResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sources, diags := sourcesFromSet(ctx, m.Sources)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	l, err := r.client.CreateFirewallList(client.FirewallListInput{Name: m.Name.ValueString(), Sources: sources, Comment: m.Comment.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Error creating the firewall address list", err.Error())
		return
	}
	resp.Diagnostics.Append(m.set(ctx, l)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *FirewallListResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m FirewallListResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	l, err := r.client.GetFirewallList(m.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading the firewall address list", err.Error())
		return
	}
	if l.Builtin {
		resp.Diagnostics.AddError("Not an address list of your own", fmt.Sprintf("%q is a built-in template: use it by name, it cannot be managed as a resource", l.Name))
		return
	}
	resp.Diagnostics.Append(m.set(ctx, l)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *FirewallListResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m FirewallListResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sources, diags := sourcesFromSet(ctx, m.Sources)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	name, comment := m.Name.ValueString(), m.Comment.ValueString()
	l, err := r.client.UpdateFirewallList(m.ID.ValueString(), client.FirewallListPatch{Name: &name, Sources: &sources, Comment: &comment})
	if err != nil {
		resp.Diagnostics.AddError("Error updating the firewall address list", err.Error())
		return
	}
	resp.Diagnostics.Append(m.set(ctx, l)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *FirewallListResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m FirewallListResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteFirewallList(m.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Error removing the firewall address list", err.Error())
	}
}

// ImportState accepts the list's id or name; Read stores the id.
func (r *FirewallListResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
