package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
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
	Retention    types.Int64  `tfsdk:"retention"`
	Format       types.String `tfsdk:"format"`
	Compress     types.Bool   `tfsdk:"compress"`
	Encrypt      types.Bool   `tfsdk:"encrypt"`
	ScheduleTime types.String `tfsdk:"schedule_time"`
	ScheduleDays types.Set    `tfsdk:"schedule_days"`
	Timezone     types.String `tfsdk:"timezone"`
	NextRunAt    types.String `tfsdk:"next_run_at"`
	LastRunAt    types.String `tfsdk:"last_run_at"`
	LastRunError types.String `tfsdk:"last_run_error"`
}

func (r *SystemBackupConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_backup_config"
}

func (r *SystemBackupConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Configures apicp's system-level backup scheduler - a singleton resource, one per apicp install. Requires an admin-tier provider token. Sets whether it runs, when (a time of day, optional days and timezone), where backups go, how many to keep, and how they are written (format, compression, encryption). The backup server's login and the encryption keys are secrets: set them up in the control panel or the API (`/v1/system/backup-sftp`, `/v1/system/backup-encryption`), not in Terraform state.",
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
				MarkdownDescription: "Where the scheduler writes archives. `\"local\"` (apicp's own filesystem, the default) or `\"sftp\"` (a remote SFTP server such as a Hetzner Storage Box; the server must have its APICP_BACKUP_SFTP_* settings configured or the API refuses it).",
			},
			"retention": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				MarkdownDescription: "How many backups to keep per account (and of the system state): after each new backup the older ones beyond this are deleted. `0` (the default) keeps every backup; otherwise 1 to 99.",
			},
			"format": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "How archives are written: `targz`, `zip` or `raw` (plain files in a folder). Omit for the server's default.",
			},
			"compress": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Compress the archive file. Omit for the server default (on). The parts from the node are already compressed, and raw files are unaffected.",
			},
			"encrypt": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Encrypt backups. Omit for the server default: on whenever it has an encryption key. `true` without a key is refused. Keys are set up in the panel or API, never here.",
			},
			"schedule_time": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Time of day to run, 24-hour `HH:MM` (for example `03:30`). Omit to run every 24 hours.",
			},
			"schedule_days": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Days to run on (`mon` to `sun`). Omit for every day. Needs `schedule_time`.",
			},
			"timezone": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "IANA timezone the schedule is read in, for example `Europe/London`. Omit for UTC. Needs `schedule_time`.",
			},
			"next_run_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC3339 time of the next scheduled run, when enabled with a schedule.",
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
	m.Retention = types.Int64Value(int64(cfg.Retention))
	m.NextRunAt = types.StringValue(cfg.NextRunAt)
	m.LastRunAt = types.StringValue(cfg.LastRunAt)
	m.LastRunError = types.StringValue(cfg.LastRunError)
}

func (r *SystemBackupConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SystemBackupConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg, err := r.client.SetSystemBackupConfig(configInput(ctx, &plan, plan.Enabled.ValueBool(), &resp.Diagnostics))
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
	cfg, err := r.client.SetSystemBackupConfig(configInput(ctx, &plan, plan.Enabled.ValueBool(), &resp.Diagnostics))
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
	if _, err := r.client.SetSystemBackupConfig(configInput(ctx, &state, false, &resp.Diagnostics)); err != nil {
		resp.Diagnostics.AddError("Error disabling system backup config", err.Error())
	}
}

func (r *SystemBackupConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// configInput builds the PUT body from the model. The server replaces the
// whole config, so every setting the model holds is sent.
func configInput(ctx context.Context, m *SystemBackupConfigResourceModel, enabled bool, diags *diag.Diagnostics) client.SystemBackupConfigInput {
	in := client.SystemBackupConfigInput{
		Enabled: enabled, Destination: m.Destination.ValueString(), Retention: int(m.Retention.ValueInt64()),
		Format: m.Format.ValueString(), ScheduleTime: m.ScheduleTime.ValueString(), Timezone: m.Timezone.ValueString(),
	}
	if !m.Compress.IsNull() && !m.Compress.IsUnknown() {
		v := m.Compress.ValueBool()
		in.Compress = &v
	}
	if !m.Encrypt.IsNull() && !m.Encrypt.IsUnknown() {
		v := m.Encrypt.ValueBool()
		in.Encrypt = &v
	}
	if !m.ScheduleDays.IsNull() && !m.ScheduleDays.IsUnknown() {
		diags.Append(m.ScheduleDays.ElementsAs(ctx, &in.ScheduleDays, false)...)
	}
	return in
}
