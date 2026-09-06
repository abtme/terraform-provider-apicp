package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

var _ resource.Resource = &CronJobResource{}
var _ resource.ResourceWithImportState = &CronJobResource{}

func NewCronJobResource() resource.Resource { return &CronJobResource{} }

type CronJobResource struct{ client *client.Client }

type CronJobResourceModel struct {
	ID           types.String `tfsdk:"id"`
	VhostID      types.String `tfsdk:"vhost_id"`
	Schedule     types.String `tfsdk:"schedule"`
	Command      types.String `tfsdk:"command"`
	UnixUser     types.String `tfsdk:"unix_user"`
	Status       types.String `tfsdk:"status"`
	LastRunAt    types.String `tfsdk:"last_run_at"`
	LastExitCode types.Int64  `tfsdk:"last_exit_code"`
	LastOutput   types.String `tfsdk:"last_output"`
}

func (r *CronJobResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cron_job"
}

func (r *CronJobResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Schedules a command to run periodically as an `apicp_web_domain`'s own Unix user, via that user's crontab. Containment is plain Unix permissions (the job runs as that unprivileged account, same boundary `apicp_ssh_access` already gives it) — not an additional sandboxing layer. Run history (`last_run_at`/`last_exit_code`/`last_output`) is never stored by apicp itself; it's read live from the job's log file on the node every time this resource is read.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"vhost_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the `apicp_web_domain` this job runs under. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"schedule": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Standard 5-field crontab schedule, e.g. `*/5 * * * *`.",
			},
			"command": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Command to run, as the vhost's own Unix user. No sandboxing beyond that user's normal Unix permissions.",
			},
			"unix_user": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The vhost's own Unix user this job runs as.",
			},
			"status": schema.StringAttribute{Computed: true},
			"last_run_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Timestamp (RFC 3339) of the job's last completed run, if it has ever run.",
			},
			"last_exit_code": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Exit code of the job's last completed run, if it has ever run.",
			},
			"last_output": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Tail (last 4KB) of the job's last run's combined stdout/stderr.",
			},
		},
	}
}

func (r *CronJobResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *CronJobResource) applyJob(j *client.CronJob, m *CronJobResourceModel) {
	m.ID = types.StringValue(j.ID)
	m.VhostID = types.StringValue(j.VhostID)
	m.Schedule = types.StringValue(j.Schedule)
	m.Command = types.StringValue(j.Command)
	m.UnixUser = types.StringValue(j.UnixUser)
	m.Status = types.StringValue(j.Status)
	if j.LastRunAt != "" {
		m.LastRunAt = types.StringValue(j.LastRunAt)
	} else {
		m.LastRunAt = types.StringNull()
	}
	if j.LastExitCode != nil {
		m.LastExitCode = types.Int64Value(*j.LastExitCode)
	} else {
		m.LastExitCode = types.Int64Null()
	}
	if j.LastOutput != "" {
		m.LastOutput = types.StringValue(j.LastOutput)
	} else {
		m.LastOutput = types.StringNull()
	}
}

func (r *CronJobResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan CronJobResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	j, err := r.client.CreateCronJob(plan.VhostID.ValueString(), plan.Schedule.ValueString(), plan.Command.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating cron job", err.Error())
		return
	}
	r.applyJob(j, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *CronJobResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state CronJobResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	j, err := r.client.GetCronJob(state.VhostID.ValueString(), state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading cron job", err.Error())
		return
	}
	r.applyJob(j, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update sends the (possibly changed) schedule/command via PATCH —
// apicp reapplies the user's full crontab afterward, so neither field
// needs RequiresReplace.
func (r *CronJobResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan CronJobResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	schedule := plan.Schedule.ValueString()
	command := plan.Command.ValueString()
	j, err := r.client.UpdateCronJob(plan.VhostID.ValueString(), plan.ID.ValueString(), &schedule, &command)
	if err != nil {
		resp.Diagnostics.AddError("Error updating cron job", err.Error())
		return
	}
	r.applyJob(j, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *CronJobResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state CronJobResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteCronJob(state.VhostID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting cron job", err.Error())
	}
}

// ImportState accepts "vhost_id/job_id" — same nested-under-parent shape
// as apicp_mailbox's own ImportState, since a cron job's ID alone isn't
// enough to route apicp's nested REST path.
func (r *CronJobResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected format: vhost_id/job_id")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vhost_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
