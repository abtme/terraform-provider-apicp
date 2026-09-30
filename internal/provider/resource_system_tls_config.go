package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

// SystemTLSConfigResource is apicp_system_tls_config - a singleton like
// apicp_system_backup_config.
type SystemTLSConfigResource struct{ client *client.Client }

var _ resource.Resource = &SystemTLSConfigResource{}
var _ resource.ResourceWithImportState = &SystemTLSConfigResource{}

func NewSystemTLSConfigResource() resource.Resource { return &SystemTLSConfigResource{} }

type SystemTLSConfigResourceModel struct {
	ID               types.String `tfsdk:"id"`
	ACMEDirectoryURL types.String `tfsdk:"acme_directory_url"`
	ListenTLSEnabled types.Bool   `tfsdk:"listen_tls_enabled"`
	ListenTLSAddr    types.String `tfsdk:"listen_tls_addr"`
}

func (r *SystemTLSConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_tls_config"
}

func (r *SystemTLSConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Sets the ACME directory apicp uses for every certificate it issues or renews (web domain certificates, the control-plane host's certificate, and apicpd's own API listener). A singleton, one per apicp install; requires an admin-tier provider token. Takes effect for the next issuance or renewal and does not touch certificates already issued. **apicpd keeps this setting in memory only**: after an apicpd restart it reverts to `APICP_ACME_DIRECTORY_URL`, and the next `terraform plan` will show the difference. Destroying this resource only removes it from Terraform state - apicp has no way to restore the previous value, so the current directory stays in effect.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"acme_directory_url": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ACME directory URL, e.g. `https://acme-v02.api.letsencrypt.org/directory` or Let's Encrypt's staging directory for testing.",
			},
			"listen_tls_enabled": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether apicpd serves its own API over TLS. Read-only: set at apicpd startup (`APICP_LISTEN_TLS_ENABLED`).",
			},
			"listen_tls_addr": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Address apicpd's TLS listener binds to. Read-only: set at apicpd startup (`APICP_LISTEN_TLS_ADDR`).",
			},
		},
	}
}

func (r *SystemTLSConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SystemTLSConfigResource) apply(cfg *client.TLSConfig, m *SystemTLSConfigResourceModel) {
	m.ID = types.StringValue("system_tls_config")
	m.ACMEDirectoryURL = types.StringValue(cfg.ACMEDirectoryURL)
	m.ListenTLSEnabled = types.BoolValue(cfg.ListenTLSEnabled)
	m.ListenTLSAddr = types.StringValue(cfg.ListenTLSAddr)
}

func (r *SystemTLSConfigResource) set(ctx context.Context, plan *SystemTLSConfigResourceModel, diags interface{ AddError(string, string) }) bool {
	cfg, err := r.client.SetTLSConfig(plan.ACMEDirectoryURL.ValueString())
	if err != nil {
		diags.AddError("Error setting system tls config", err.Error())
		return false
	}
	r.apply(cfg, plan)
	return true
}

func (r *SystemTLSConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SystemTLSConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !r.set(ctx, &plan, &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SystemTLSConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SystemTLSConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg, err := r.client.GetTLSConfig()
	if err != nil {
		resp.Diagnostics.AddError("Error reading system tls config", err.Error())
		return
	}
	r.apply(cfg, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SystemTLSConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SystemTLSConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !r.set(ctx, &plan, &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete only forgets the resource: there is nothing to reset it to.
func (r *SystemTLSConfigResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *SystemTLSConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
