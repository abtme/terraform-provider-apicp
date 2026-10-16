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
	Aliases      types.Set    `tfsdk:"aliases"`
	NodeID       types.String `tfsdk:"node_id"`
	DocumentRoot types.String `tfsdk:"document_root"`
	UnixUser     types.String `tfsdk:"unix_user"`
	Status       types.String `tfsdk:"status"`
	TLSEnabled   types.Bool   `tfsdk:"tls_enabled"`
	PHPVersion   types.String `tfsdk:"php_version"`
	NodeVersion  types.String `tfsdk:"node_version"`
	AppEntry     types.String `tfsdk:"app_entry"`
	AppPort      types.Int64  `tfsdk:"app_port"`
}

func (m *WebDomainResourceModel) runtime() client.VhostRuntime {
	return client.VhostRuntime{PHPVersion: m.PHPVersion.ValueString(), NodeVersion: m.NodeVersion.ValueString(), AppEntry: m.AppEntry.ValueString()}
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
			"aliases": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Extra host names this vhost serves from the same document root, e.g. `www.example.com`. " +
					"Lower case; each name must be unique across all vhosts (as a domain or an alias) and must resolve to the node. " +
					"One `apicp_tls_certificate` covers the domain and every alias: adding an alias to a domain that already has a certificate " +
					"orders a new one for all the names (if that fails the apply fails and nothing changes); removing an alias orders nothing, " +
					"and the next automatic renewal leaves the name out.",
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
			"php_version": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "PHP-FPM version to serve this domain with: `5.6`, `7.2`, `7.4`, `8.0`, `8.1`, `8.2`, `8.3`, or `8.4`. Omit (or set to `\"\"`) for static content only, the default — apicp never installs a PHP version itself, a domain can only select one already provisioned on its node. Cannot be set together with `node_version`.",
			},
			"node_version": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Run a Node.js app for this domain instead of static files or PHP: `22` or `24`. apicp runs it as the domain's Unix user from `/home/<unix_user>/app` " +
					"(deploy it there over SSH, then touch `~/app/restart.txt`), listening on `127.0.0.1:app_port`, and nginx proxies to it. " +
					"Cannot be set together with `php_version`. Setting it back to `\"\"` (or removing it) stops and removes the app's service; its files stay.",
			},
			"app_entry": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The Node.js app's start file, relative to `~/app`. Omit for `dist/server/entry.mjs`, what Astro's Node adapter (and EmDash) builds.",
			},
			"app_port": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The loopback port apicp gave the Node.js app (passed to it as `PORT`); 0 without `node_version`.",
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

// aliasSet turns the API's alias list into the attribute value. An empty list stays null when the
// configuration left the attribute out, so a vhost without aliases does not show a perpetual diff.
func aliasSet(ctx context.Context, aliases []string, current types.Set) (types.Set, diag.Diagnostics) {
	if len(aliases) == 0 && (current.IsNull() || current.IsUnknown()) {
		return types.SetNull(types.StringType), nil
	}

	// never a nil slice: SetValueFrom turns that into a null set, but an empty list that was configured
	// must come back as an empty set or Terraform reports an inconsistent result after apply
	return types.SetValueFrom(ctx, types.StringType, append([]string{}, aliases...))
}

func aliasList(ctx context.Context, s types.Set) ([]string, diag.Diagnostics) {
	if s.IsNull() || s.IsUnknown() {
		return nil, nil
	}

	var out []string

	return out, s.ElementsAs(ctx, &out, false)
}

func (r *WebDomainResource) applyVhost(ctx context.Context, v *client.Vhost, m *WebDomainResourceModel) diag.Diagnostics {
	aliases, diags := aliasSet(ctx, v.Aliases, m.Aliases)
	m.Aliases = aliases
	m.ID = types.StringValue(v.ID)
	m.Domain = types.StringValue(v.Domain)
	m.NodeID = types.StringValue(v.NodeID)
	m.DocumentRoot = types.StringValue(v.DocumentRoot)
	m.UnixUser = types.StringValue(v.UnixUser)
	m.Status = types.StringValue(v.Status)
	m.TLSEnabled = types.BoolValue(v.TLSEnabled)
	m.PHPVersion = types.StringValue(v.PHPVersion)
	m.NodeVersion = types.StringValue(v.NodeVersion)
	m.AppEntry = types.StringValue(v.AppEntry)
	m.AppPort = types.Int64Value(int64(v.AppPort))

	return diags
}

func (r *WebDomainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan WebDomainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	aliases, diags := aliasList(ctx, plan.Aliases)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	v, err := r.client.CreateVhost(plan.Domain.ValueString(), aliases, plan.runtime())
	if err != nil {
		resp.Diagnostics.AddError("Error creating web domain", err.Error())
		return
	}
	resp.Diagnostics.Append(r.applyVhost(ctx, v, &plan)...)
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
	resp.Diagnostics.Append(r.applyVhost(ctx, v, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *WebDomainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state WebDomainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	aliases, diags := aliasList(ctx, plan.Aliases)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	v, err := r.client.PatchVhost(state.ID.ValueString(), plan.Domain.ValueString(), aliases, plan.runtime())
	if err != nil {
		resp.Diagnostics.AddError("Error updating web domain", err.Error())
		return
	}
	resp.Diagnostics.Append(r.applyVhost(ctx, v, &plan)...)
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
