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

// SystemBackupConfigResource is apicp_system_backup_config (PLAN.md §8
// phase 5, §7.1) - a singleton resource controlling apicp's own
// system-level backup scheduler. Unlike apicp_phpmyadmin/apicp_webmail's
// zero-argument toggle shape, this has two real settable arguments
// (enabled, destination) since the user explicitly asked for the API
// (and so this provider) to expose ON/OFF *and where*, deliberately
// never the backup mechanics themselves (schedule, retention, what gets
// archived) - those are an apicpd-internal decision, not configurable
// here or through the API at all.
type SystemBackupConfigResource struct {
	client *client.Client
}

var _ resource.Resource = &SystemBackupConfigResource{}
var _ resource.ResourceWithImportState = &SystemBackupConfigResource{}

func NewSystemBackupConfigResource() resource.Resource { return &SystemBackupConfigResource{} }

type SystemBackupConfigResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Enabled      types.Bool   `tfsdk:"enabled"`
	Destination  types.String `tfsdk:"destination"`
	LastRunAt    types.String `tfsdk:"last_run_at"`
	LastRunError types.String `tfsdk:"last_run_error"`
}

func (r *SystemBackupConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_backup_config"
}

func (r *SystemBackupConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Configures apicp's system-level backup scheduler (PLAN.md §7.1/§8 phase 5) - a singleton resource, one per apicp install. Requires an admin-tier provider token. Only `enabled` and `destination` are settable: the backup mechanics themselves (what gets archived, on what schedule) are an internal apicpd decision, never exposed through this resource or the underlying API - by explicit design, not an oversight.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: "Whether apicpd's internal backup scheduler is turned on.",
			},
			"destination": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Where the scheduler writes archives. Currently only `\"local\"` is supported (apicp's own filesystem, PLAN.md §7.3's `LocalTarget`).",
			},
			"last_run_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC3339 timestamp of the scheduler's last run, if it has run at least once.",
			},
			"last_run_error": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Error from the scheduler's last run, if it failed.",
			},
		},
	}
}

func (r *SystemBackupConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SystemBackupConfigResource) apply(cfg *client.SystemBackupConfig, m *SystemBackupConfigResourceModel) {
	m.ID = types.StringValue("system_backup_config")
	m.Enabled = types.BoolValue(cfg.Enabled)
	m.Destination = types.StringValue(cfg.Destination)
	m.LastRunAt = types.StringValue(cfg.LastRunAt)
	m.LastRunError = types.StringValue(cfg.LastRunError)
}

func (r *SystemBackupConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SystemBackupConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg, err := r.client.SetSystemBackupConfig(plan.Enabled.ValueBool(), plan.Destination.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error setting system backup config", err.Error())
		return
	}
	r.apply(cfg, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SystemBackupConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SystemBackupConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg, err := r.client.GetSystemBackupConfig()
	if err != nil {
		resp.Diagnostics.AddError("Error reading system backup config", err.Error())
		return
	}
	r.apply(cfg, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SystemBackupConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SystemBackupConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg, err := r.client.SetSystemBackupConfig(plan.Enabled.ValueBool(), plan.Destination.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error setting system backup config", err.Error())
		return
	}
	r.apply(cfg, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete turns the scheduler back off rather than erroring - there's no
// real "delete" concept for a singleton config, matching apicp_webmail's
// disable-on-destroy behavior.
func (r *SystemBackupConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SystemBackupConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.SetSystemBackupConfig(false, state.Destination.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error disabling system backup config", err.Error())
	}
}

func (r *SystemBackupConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
