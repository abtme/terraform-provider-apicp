package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

// SystemBackupConfigResource is apicp_system_backup_config - a singleton
// resource for apicp's own system-level backup scheduler. It manages only the
// settings the configuration sets (PATCH); see the schema description.
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

	BackupUnit        types.String `tfsdk:"backup_unit"`
	Verify            types.Bool   `tfsdk:"verify"`
	EncryptionWarning types.String `tfsdk:"encryption_warning"`
	LastRunWarnings   types.List   `tfsdk:"last_run_warnings"`
}

func (r *SystemBackupConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_backup_config"
}

func (r *SystemBackupConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Configures apicp's system-level backup scheduler - a singleton resource, one per apicp install. Requires an admin-tier provider token. Sets whether it runs, when (a time of day, optional days and timezone), where backups go, how many to keep, how they are written (format, compression, encryption), whether they are split per domain and whether each run is checked by reading it back. The backup server's login and the encryption keys are secrets: set them up in the control panel or the API (`/v1/system/backup-sftp`, `/v1/system/backup-encryption`), not in Terraform state.\n\n" +
			"**Only the arguments you set are managed.** On apicp v0.7.0 or later the provider changes settings with `PATCH /v1/system/backup-config` and sends only `enabled`, `destination` and the optional arguments set in the configuration; anything left out keeps the value saved on the server (set in the panel or with `apicp backup configure`). Removing an optional argument from the configuration puts that setting back to the server's default. The provider never writes back values it read: apicp shows effective values, and saving one as an explicit choice (for example `encrypt = false` read while the server had no key yet) would keep it after the default changes. `encryption_warning` says when backups are written unencrypted although the server has a key.\n\n" +
			"Older apicpd has no PATCH: the provider then replaces the whole config with `PUT` as before - every optional argument left out goes back to the server's default, `retention` (when left out) keeps the server's value, and `backup_unit` / `verify` are refused.\n\n" +
			"apicp v0.7.0 and later refuse a setting they do not know (a provider newer than the server) instead of ignoring it: applying is then an error naming the argument, and nothing is changed.",
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
				MarkdownDescription: "How many scheduled run folders to keep (older ones are removed only after a run with no failure), and how many one-off backups to keep of each part of an account (each website, mail domain and the settings archive). `0` keeps every backup; otherwise 1 to 99. Leave it out to keep the server's value (shown here).",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"format": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "How archives are written: `targz`, `zip` or `raw` (plain files in a folder). Omit to keep the server's setting; removing it from the configuration puts back the server's default.",
			},
			"compress": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Compress the archive file. Omit to keep the server's setting (its default is on); removing it from the configuration puts back that default. The parts from the node are already compressed, and raw files are unaffected.",
			},
			"encrypt": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Encrypt backups. Omit to keep the server's setting (its default is on whenever it has an encryption key); removing it from the configuration puts it back to that default. `true` without a key is refused. Keys are set up in the panel or API, never here.",
			},
			"backup_unit": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "How each account is split into archives: `domain` (the default: a settings archive per account plus one web and one mail archive per domain) or `account` (one archive per account, as before apicp v0.7.0). Omit to keep the server's setting. Needs apicp v0.7.0 or later.",
			},
			"verify": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "The restore check after each backup run, scheduled or one-off (`apicp_account_backup` shows it per archive); the server's default is on. Every archive is read back, decrypted, its manifest parsed and every member read to the end; a failed check fails the run, so older backups are not pruned. Over SFTP it transfers everything twice. Omit to keep the server's setting. Needs apicp v0.7.0 or later.",
			},
			"encryption_warning": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Set when the server has an encryption key but backups are written unencrypted (encryption was turned off). Empty otherwise, and from apicpd before v0.7.0.",
			},
			"last_run_warnings": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "What the latest scheduled run could not capture in full (`<file>: <warning>`); the run still succeeded. Empty from apicpd before v0.7.0.",
			},
			"schedule_time": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Time of day to run, 24-hour `HH:MM` (for example `03:30`). Omit to keep the server's setting; with none the scheduler runs every 24 hours. Removing it from the configuration clears it.",
			},
			"schedule_days": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Days to run on (`mon` to `sun`); none means every day. Omit to keep the server's setting; removing it from the configuration clears it. Needs `schedule_time`.",
			},
			"timezone": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "IANA timezone the schedule is read in, for example `Europe/London`; none means UTC. Omit to keep the server's setting; removing it from the configuration clears it. Needs `schedule_time`.",
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

// apply copies what the server reports into the model: the required and
// computed attributes only. The optional settings keep what the
// configuration says (null when it says nothing): the server shows
// effective values, and taking those into state would make Terraform save
// them as explicit choices on the next write.
func (r *SystemBackupConfigResource) apply(ctx context.Context, cfg *client.SystemBackupConfig, m *SystemBackupConfigResourceModel) diag.Diagnostics {
	m.ID = types.StringValue("system_backup_config")
	m.Enabled = types.BoolValue(cfg.Enabled)
	m.Destination = types.StringValue(cfg.Destination)
	m.Retention = types.Int64Value(int64(cfg.Retention))
	m.NextRunAt = types.StringValue(cfg.NextRunAt)
	m.LastRunAt = types.StringValue(cfg.LastRunAt)
	m.LastRunError = types.StringValue(cfg.LastRunError)
	m.EncryptionWarning = types.StringValue(cfg.EncryptionWarning)
	warnings := cfg.LastRunWarnings
	if warnings == nil {
		warnings = []string{}
	}
	list, diags := types.ListValueFrom(ctx, types.StringType, warnings)
	m.LastRunWarnings = list
	return diags
}

// refreshManaged takes the server's effective value of each optional
// setting the state manages (non-null), so a change made outside Terraform
// shows as drift. Settings the state does not manage stay null.
func refreshManaged(cfg *client.SystemBackupConfig, m *SystemBackupConfigResourceModel) {
	if !m.Compress.IsNull() {
		m.Compress = types.BoolValue(cfg.Compress)
	}
	if !m.Encrypt.IsNull() {
		m.Encrypt = types.BoolValue(cfg.Encrypt)
	}
	if !m.Verify.IsNull() && cfg.Verify != nil {
		m.Verify = types.BoolValue(*cfg.Verify)
	}
	if !m.BackupUnit.IsNull() && cfg.BackupUnit != "" {
		m.BackupUnit = types.StringValue(cfg.BackupUnit)
	}
}

func (r *SystemBackupConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config SystemBackupConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg := r.write(ctx, &config, nil, &resp.Diagnostics)
	if cfg == nil {
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, cfg, &plan)...)
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
	resp.Diagnostics.Append(r.apply(ctx, cfg, &state)...)
	refreshManaged(cfg, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SystemBackupConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, config, state SystemBackupConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg := r.write(ctx, &config, &state, &resp.Diagnostics)
	if cfg == nil {
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, cfg, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete turns the scheduler back off rather than erroring - there's no
// real "delete" concept for a singleton config, matching apicp_webmail's
// disable-on-destroy behavior. Every other setting stays as it is.
func (r *SystemBackupConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SystemBackupConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.PatchSystemBackupConfig(map[string]any{"enabled": false})
	if errors.Is(err, client.ErrPatchUnsupported) {
		_, err = r.client.SetSystemBackupConfig(putInput(ctx, &state, false, int(state.Retention.ValueInt64()), &resp.Diagnostics))
	}
	if err != nil {
		resp.Diagnostics.AddError("Error disabling system backup config", errorDetail(err))
	}
}

func (r *SystemBackupConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// write saves the configuration: PATCH with only what it sets (and resets
// of what prior managed and it no longer sets), or PUT on an apicpd without
// PATCH. prior is nil on create. Returns nil after adding an error.
func (r *SystemBackupConfigResource) write(ctx context.Context, config, prior *SystemBackupConfigResourceModel, diags *diag.Diagnostics) *client.SystemBackupConfig {
	body := patchBody(ctx, config, prior, diags)
	if diags.HasError() {
		return nil
	}
	cfg, err := r.client.PatchSystemBackupConfig(body)
	if errors.Is(err, client.ErrPatchUnsupported) {
		cfg, err = r.put(ctx, config, prior, diags)
		if diags.HasError() {
			return nil
		}
	}
	if err != nil {
		diags.AddError("Error setting system backup config", errorDetail(err))
		return nil
	}
	if cfg.EncryptionWarning != "" && !(known(config.Encrypt) && !config.Encrypt.ValueBool()) {
		diags.AddWarning("Backups are not encrypted", cfg.EncryptionWarning+
			"\n\nSet encrypt = true here to manage it with Terraform (omitting encrypt keeps what the server has saved).")
	}
	return cfg
}

// put is the write for an apicpd from before PATCH: the whole config is
// replaced, as this provider always did there. backup_unit and verify do not
// exist on such a server, so setting them is refused before anything is
// sent. retention, when the configuration leaves it out, keeps the server's
// value (PUT would set 0).
func (r *SystemBackupConfigResource) put(ctx context.Context, config, prior *SystemBackupConfigResourceModel, diags *diag.Diagnostics) (*client.SystemBackupConfig, error) {
	var newer []string
	if known(config.BackupUnit) {
		newer = append(newer, "backup_unit")
	}
	if known(config.Verify) {
		newer = append(newer, "verify")
	}
	if len(newer) > 0 {
		diags.AddError("Backup setting not supported by this apicpd",
			fmt.Sprintf("%s need apicp v0.7.0 or later (per-domain backups); this apicpd has no PATCH /v1/system/backup-config. Upgrade apicp, or leave them out. Nothing was changed.", strings.Join(newer, " and ")))
		return nil, nil
	}

	retention := 0
	switch {
	case known(config.Retention):
		retention = int(config.Retention.ValueInt64())
	case prior != nil && known(prior.Retention):
		retention = int(prior.Retention.ValueInt64())
	default:
		cur, err := r.client.GetSystemBackupConfig()
		if err != nil {
			return nil, fmt.Errorf("reading the current retention to keep it: %w", err)
		}
		retention = cur.Retention
	}
	return r.client.SetSystemBackupConfig(putInput(ctx, config, config.Enabled.ValueBool(), retention, diags))
}

func known(v attr.Value) bool { return !v.IsNull() && !v.IsUnknown() }

// patchBody is the PATCH for config: enabled and destination always, every
// optional setting config sets, and for one prior managed but config no
// longer sets, its reset (null for compress/encrypt/verify - the server's
// default - and empty for the rest). A setting neither sets is left out,
// so the server keeps what it has saved.
func patchBody(ctx context.Context, config, prior *SystemBackupConfigResourceModel, diags *diag.Diagnostics) map[string]any {
	body := map[string]any{
		"enabled":     config.Enabled.ValueBool(),
		"destination": config.Destination.ValueString(),
	}
	if known(config.Retention) {
		body["retention"] = config.Retention.ValueInt64()
	}
	was := func(get func(*SystemBackupConfigResourceModel) attr.Value) bool {
		return prior != nil && known(get(prior))
	}

	str := func(key string, get func(*SystemBackupConfigResourceModel) attr.Value) {
		if v := get(config); known(v) {
			body[key] = v.(types.String).ValueString()
		} else if was(get) {
			body[key] = ""
		}
	}
	str("format", func(m *SystemBackupConfigResourceModel) attr.Value { return m.Format })
	str("schedule_time", func(m *SystemBackupConfigResourceModel) attr.Value { return m.ScheduleTime })
	str("timezone", func(m *SystemBackupConfigResourceModel) attr.Value { return m.Timezone })
	str("backup_unit", func(m *SystemBackupConfigResourceModel) attr.Value { return m.BackupUnit })

	boolean := func(key string, get func(*SystemBackupConfigResourceModel) attr.Value) {
		if v := get(config); known(v) {
			body[key] = v.(types.Bool).ValueBool()
		} else if was(get) {
			body[key] = nil
		}
	}
	boolean("compress", func(m *SystemBackupConfigResourceModel) attr.Value { return m.Compress })
	boolean("encrypt", func(m *SystemBackupConfigResourceModel) attr.Value { return m.Encrypt })
	boolean("verify", func(m *SystemBackupConfigResourceModel) attr.Value { return m.Verify })

	if known(config.ScheduleDays) {
		days := []string{}
		diags.Append(config.ScheduleDays.ElementsAs(ctx, &days, false)...)
		body["schedule_days"] = days
	} else if prior != nil && known(prior.ScheduleDays) {
		body["schedule_days"] = []string{}
	}
	return body
}

// putInput builds the PUT body (an apicpd without PATCH) from the model:
// every setting the model holds; unset ones are left out, so the server
// uses its default for them.
func putInput(ctx context.Context, m *SystemBackupConfigResourceModel, enabled bool, retention int, diags *diag.Diagnostics) client.SystemBackupConfigInput {
	in := client.SystemBackupConfigInput{
		Enabled: enabled, Destination: m.Destination.ValueString(), Retention: retention,
		Format: m.Format.ValueString(), ScheduleTime: m.ScheduleTime.ValueString(), Timezone: m.Timezone.ValueString(),
	}
	if known(m.Compress) {
		v := m.Compress.ValueBool()
		in.Compress = &v
	}
	if known(m.Encrypt) {
		v := m.Encrypt.ValueBool()
		in.Encrypt = &v
	}
	if known(m.ScheduleDays) {
		diags.Append(m.ScheduleDays.ElementsAs(ctx, &in.ScheduleDays, false)...)
	}
	return in
}
