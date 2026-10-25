package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

// accountBackupCreateTimeout bounds how long Create waits for apicp to make
// the backup. apicpd makes it in a process of its own and is polled; if this
// runs out, the backup carries on on the server and can be imported once it
// is made. The provider has no per-resource timeouts block yet, so this is
// fixed.
const accountBackupCreateTimeout = 30 * time.Minute

// AccountBackupResource is apicp_account_backup: one point-in-time backup of
// an account - one archive from an apicpd that makes one per account, or the
// set one "back up now" makes with per-domain backups (a settings archive
// plus a web and a mail archive per domain).
type AccountBackupResource struct{ client *client.Client }

var _ resource.Resource = &AccountBackupResource{}
var _ resource.ResourceWithImportState = &AccountBackupResource{}

func NewAccountBackupResource() resource.Resource { return &AccountBackupResource{} }

type AccountBackupResourceModel struct {
	ID        types.String `tfsdk:"id"`
	AccountID types.String `tfsdk:"account_id"`
	CreatedAt types.String `tfsdk:"created_at"`
	SizeBytes types.Int64  `tfsdk:"size_bytes"`
	RunID     types.String `tfsdk:"run_id"`
	Backups   types.List   `tfsdk:"backups"`
}

// accountBackupArchive is one element of backups.
type accountBackupArchive struct {
	ID          types.String `tfsdk:"id"`
	Scope       types.String `tfsdk:"scope"`
	Domain      types.String `tfsdk:"domain"`
	Name        types.String `tfsdk:"name"`
	SizeBytes   types.Int64  `tfsdk:"size_bytes"`
	Verified    types.Bool   `tfsdk:"verified"`
	VerifyError types.String `tfsdk:"verify_error"`
}

var accountBackupArchiveType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"id": types.StringType, "scope": types.StringType, "domain": types.StringType, "name": types.StringType,
	"size_bytes": types.Int64Type, "verified": types.BoolType, "verify_error": types.StringType,
}}

func (r *AccountBackupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_backup"
}

func (r *AccountBackupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Takes one backup of an `apicp_account` - everything it owns (websites, mailboxes, databases, DNS zones, settings) - stored on apicp. " +
			"With per-domain backups (apicp v0.7.0 on, `backup_unit = \"domain\"`, the default) one backup is several archives: the account's settings archive plus one web and one mail archive per domain, listed in `backups`. " +
			"With `backup_unit = \"account\"`, or an older apicpd, it is one archive, and `backups` lists just that one.\n\n" +
			"It is a point-in-time snapshot: applying creates one backup, and a later apply does not take another. apicp makes the backup in a process of its own; creating waits for it, checking every few seconds, for up to 30 minutes. If that runs out the backup carries on on the server and is not in Terraform state - import it once it is made (see Import). " +
			"If a backup of the same account is already running (started from the CLI, the panel or another resource), creating waits for it to end and then takes a backup of its own, so each resource owns its backup. A backup that could not capture something in full, or whose old one-off backups could not be removed afterwards, is kept, with a warning. " +
			"If the run fails part way (a part could not be stored), apicp v0.7.0 deletes the archives it had made and creating is an error; any it could not delete are left marked incomplete (apicp does not restore them). " +
			"If every archive was made but apicp's restore check of one fails, the archives stay on the server (`verified = false`, with `verify_error`) and creating is an error too. In both cases archives left on the server are kept in state and the resource is marked tainted, so the next apply deletes them and takes a new backup (and destroying deletes them); `terraform untaint` keeps them instead. " +
			"apicp's one-off backup retention (`apicp_system_backup_config.retention`, per part) can remove the archives of older backups: an archive gone from the server drops out of `backups`, and the resource is gone once none is left.\n\n" +
			"To take a fresh one, replace the resource (`terraform apply -replace=...`, or `replace_triggered_by`). Destroying deletes every archive of the backup. Restoring is an action, not a state, so it is not exposed here - use apicp's API or CLI (`POST /v1/accounts/{id}/backups/restore-account` with `backups` set to these IDs, or on apicp v0.7.0 `batch_id` set to `run_id`; or `apicp backup restore-account <account> --backups ...`; one archive: `POST /v1/accounts/{id}/backups/{backup_id}/restore`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID of the backup's first archive: the account's settings archive with per-domain backups, the only archive otherwise.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"account_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the `apicp_account` to back up. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "When the backup was taken (RFC3339; the first archive's time).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"size_bytes": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Size of the stored backup: all its archives together (the one archive's size on an apicpd that makes one per account).",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"run_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The apicp backup run that made it (`arun_...`), which is also the `batch_id` apicp v0.7.0 records on each of its archives: when the server reports it, the backup is every archive of the account with that batch ID. Empty from an apicpd that made the backup within the request, and after an import by archive IDs.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"backups": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Every archive of this backup, the settings archive first.",
				PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown()},
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":     schema.StringAttribute{Computed: true, MarkdownDescription: "The archive's backup ID (`bkp_...`)."},
					"scope":  schema.StringAttribute{Computed: true, MarkdownDescription: "What the archive holds: `account` (the account's settings), `domain-web` (one website), `domain-mail` (one mail domain), or empty for a whole-account archive."},
					"domain": schema.StringAttribute{Computed: true, MarkdownDescription: "The website or mail domain of a `domain-web` / `domain-mail` archive; empty otherwise."},
					"name": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The archive's file in the backup location, e.g. `manual/root._.alice.alice.example.web.<time>.<id>.tar.gz`; empty from an apicpd that does not say.",
					},
					"size_bytes": schema.Int64Attribute{Computed: true, MarkdownDescription: "Size of the stored archive."},
					"verified": schema.BoolAttribute{
						Computed:            true,
						MarkdownDescription: "Result of apicp's restore check for this archive (read back, decrypted, every member read): `true` passed, `false` failed (see `verify_error`; apicp restores it only when asked to with `allow_unverified`). Null when it was not checked: the server's `verify` setting is off, the archive is from before apicp v0.7.0, or the apicpd does not report it. Refreshed from the server's backup list on every read.",
					},
					"verify_error": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Why the restore check failed; empty when it passed, was not run or the apicpd does not report it.",
					},
				}},
			},
		},
	}
}

func (r *AccountBackupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// applyAccountBackups puts the archives of the backup (first one first) into the model.
func applyAccountBackups(ctx context.Context, accountID string, archives []client.AccountBackup, m *AccountBackupResourceModel) diag.Diagnostics {
	first := archives[0]
	m.ID = types.StringValue(first.ID)
	m.AccountID = types.StringValue(accountID)
	m.CreatedAt = types.StringValue(first.CreatedAt)

	var total int64
	elems := make([]accountBackupArchive, len(archives))
	for i, b := range archives {
		total += b.SizeBytes
		elems[i] = accountBackupArchive{
			ID: types.StringValue(b.ID), Scope: types.StringValue(b.Scope), Domain: types.StringValue(b.Domain),
			Name: types.StringValue(b.Name()), SizeBytes: types.Int64Value(b.SizeBytes), Verified: types.BoolPointerValue(b.Verified),
			VerifyError: types.StringValue(b.VerifyError),
		}
	}
	m.SizeBytes = types.Int64Value(total)
	list, diags := types.ListValueFrom(ctx, accountBackupArchiveType, elems)
	m.Backups = list
	return diags
}

// archiveIDs is the IDs of the archives the state holds, the first one
// first: the backups list, or id alone (state from before backups, or an
// import of one archive).
func (m *AccountBackupResourceModel) archiveIDs(ctx context.Context) ([]string, diag.Diagnostics) {
	var ids []string
	var diags diag.Diagnostics
	if !m.Backups.IsNull() && !m.Backups.IsUnknown() {
		var elems []accountBackupArchive
		diags = m.Backups.ElementsAs(ctx, &elems, false)
		for _, e := range elems {
			if id := e.ID.ValueString(); id != "" {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 && m.ID.ValueString() != "" {
		ids = []string{m.ID.ValueString()}
	}
	return ids, diags
}

func (r *AccountBackupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AccountBackupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	waitCtx, cancel := context.WithTimeout(ctx, accountBackupCreateTimeout)
	defer cancel()
	res, err := r.client.CreateAccountBackup(waitCtx, plan.AccountID.ValueString())
	var failed *client.AccountBackupFailedError
	if errors.As(err, &failed) {
		r.createFailed(ctx, &plan, failed, resp)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error creating account backup", err.Error())
		return
	}
	if res.Warning != "" {
		resp.Diagnostics.AddWarning("Account backup made with a warning",
			fmt.Sprintf("Backup %s was made, but: %s", res.Backup.ID, res.Warning))
	}
	if len(res.Warnings) > 0 {
		resp.Diagnostics.AddWarning("Account backup could not capture everything",
			fmt.Sprintf("Backup %s was made, but some archives are not complete (a restore says so again):\n- %s", res.Backup.ID, strings.Join(res.Warnings, "\n- ")))
	}

	archives := res.Backups
	if len(archives) == 0 {
		archives = []client.AccountBackup{*res.Backup}
	}
	resp.Diagnostics.Append(applyAccountBackups(ctx, plan.AccountID.ValueString(), archives, &plan)...)
	plan.RunID = types.StringValue(runIDOf(res.RunID, archives))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// runIDOf is the backup's run ID: the batch ID its archives carry (apicp
// v0.7.0 on, the same as the run's ID), else the run's.
func runIDOf(runID string, archives []client.AccountBackup) string {
	if len(archives) > 0 && archives[0].BatchID != "" {
		return archives[0].BatchID
	}
	return runID
}

// createFailed reports a backup run that failed. With nothing left on the
// server (apicp v0.7.0 deletes the parts of a run that failed part way) it
// is a plain error and nothing goes into state. Archives it left - all of
// them when only the restore check failed, else parts apicp could not
// delete (marked incomplete) - are put into state with the error, which
// makes Terraform mark the resource tainted: they stay tracked, the next
// apply deletes them and takes a new backup, and destroy deletes them.
func (r *AccountBackupResource) createFailed(ctx context.Context, plan *AccountBackupResourceModel, failed *client.AccountBackupFailedError, resp *resource.CreateResponse) {
	left := failed.Run.Archives()
	if len(left) == 0 {
		resp.Diagnostics.AddError("Error creating account backup", failed.Error()+". Nothing of it is left on the server or in Terraform state.")
		return
	}
	resp.Diagnostics.Append(applyAccountBackups(ctx, plan.AccountID.ValueString(), left, plan)...)
	plan.RunID = types.StringValue(runIDOf(failed.Run.RunID, left))
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	summary := "Account backup failed"
	if failed.RestoreCheckFailed() {
		summary = "Account backup failed its restore check"
	}
	resp.Diagnostics.AddError(summary, failed.Error()+
		".\n\nThe archives left on the server are kept in Terraform state and the resource is marked tainted: the next apply deletes them and takes a new backup, and destroying deletes them. "+
		"To keep them instead, run terraform untaint on this resource.")
}

// selectArchives picks this backup's archives from the account's backup
// list: every archive whose batch_id is runID (apicp v0.7.0 on), else the
// archives the state names that are still there (older apicpd, archives
// made before batch IDs, or an import by archive IDs). The settings archive
// comes first, then the state's order, then any other archive of the batch.
func selectArchives(all []client.AccountBackup, runID string, ids []string) []client.AccountBackup {
	byID := make(map[string]client.AccountBackup, len(all))
	for _, b := range all {
		byID[b.ID] = b
	}
	batch := client.BatchArchives(all, runID)
	if len(batch) == 0 {
		var still []client.AccountBackup
		for _, id := range ids {
			if b, ok := byID[id]; ok {
				still = append(still, b)
			}
		}
		return still
	}
	inBatch := make(map[string]bool, len(batch))
	for _, b := range batch {
		inBatch[b.ID] = true
	}
	var out []client.AccountBackup
	seen := map[string]bool{}
	for _, id := range ids {
		if inBatch[id] && !seen[id] {
			out = append(out, byID[id])
			seen[id] = true
		}
	}
	for _, b := range batch {
		if !seen[b.ID] {
			out = append(out, b)
			seen[b.ID] = true
		}
	}
	client.SettingsFirst(out)
	return out
}

func (r *AccountBackupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AccountBackupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ids, diags := state.archiveIDs(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	all, err := r.client.ListAccountBackups(state.AccountID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading account backup", err.Error())
		return
	}
	still := selectArchives(all, state.RunID.ValueString(), ids)
	if len(still) == 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(applyAccountBackups(ctx, state.AccountID.ValueString(), still, &state)...)
	if state.RunID.IsNull() || state.RunID.IsUnknown() {
		state.RunID = types.StringValue("")
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update - account_id replaces and everything else is Computed.
func (r *AccountBackupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AccountBackupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete deletes every archive of the backup; one already gone is fine.
func (r *AccountBackupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AccountBackupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ids, diags := state.archiveIDs(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	var failed []string
	for _, id := range ids {
		if err := r.client.DeleteAccountBackup(state.AccountID.ValueString(), id); err != nil && !errors.Is(err, client.ErrNotFound) {
			failed = append(failed, id+": "+err.Error())
		}
	}
	if len(failed) > 0 {
		resp.Diagnostics.AddError("Error deleting account backup",
			fmt.Sprintf("%d of %d archives could not be deleted:\n- %s", len(failed), len(ids), strings.Join(failed, "\n- ")))
	}
}

// ImportState accepts "account_id/backup_id" (one archive),
// "account_id/backup_id,backup_id,..." (the archives of one backup, the
// settings archive first) or "account_id/arun_..." (every archive a backup
// run made: found by batch_id in the account's backup list on apicp v0.7.0,
// else from the run, which apicp keeps for an account's latest 10 runs).
func (r *AccountBackupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "Expected account_id/backup_id, account_id/backup_id,backup_id,... or account_id/run_id (arun_...)")
		return
	}
	accountID := parts[0]

	var archives []client.AccountBackup
	runID := ""
	if strings.HasPrefix(parts[1], "arun_") {
		if r.client == nil {
			resp.Diagnostics.AddError("Provider not configured", "importing by run ID needs the provider's apicp connection")
			return
		}
		runID = parts[1]
		archives = r.importRun(accountID, runID, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
	} else {
		for _, id := range strings.Split(parts[1], ",") {
			if id = strings.TrimSpace(id); id != "" {
				archives = append(archives, client.AccountBackup{ID: id})
			}
		}
		if len(archives) == 0 {
			resp.Diagnostics.AddError("Invalid import ID", "no backup ID after the account ID")
			return
		}
	}

	m := AccountBackupResourceModel{RunID: types.StringValue(runID)}
	resp.Diagnostics.Append(applyAccountBackups(ctx, accountID, archives, &m)...)
	// the read that follows fills in the rest of each archive
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// importRun finds the archives of one back-up-now run: those with that
// batch_id in the account's backup list (apicp v0.7.0 on - this works however
// long ago the run was), else the run's own record (older apicpd, or
// archives from before batch IDs; apicp keeps an account's latest 10 runs).
func (r *AccountBackupResource) importRun(accountID, runID string, diags *diag.Diagnostics) []client.AccountBackup {
	all, err := r.client.ListAccountBackups(accountID)
	if err != nil {
		diags.AddError("Error reading account backups", fmt.Sprintf("account %s: %v", accountID, err))
		return nil
	}
	if batch := client.BatchArchives(all, runID); len(batch) > 0 {
		var incomplete []string
		for _, b := range batch {
			if b.Incomplete {
				incomplete = append(incomplete, b.ID)
			}
		}
		if len(incomplete) == 0 {
			return batch
		}
		run, err := r.client.GetAccountBackupRun(accountID, runID)
		if err == nil && run.Status == client.AccountBackupRunRunning {
			diags.AddError("Backup run not finished", fmt.Sprintf("run %s is still running; import it once it has finished", runID))
			return nil
		}
		diags.AddWarning("Importing incomplete archives",
			fmt.Sprintf("Run %s did not finish: archive(s) %s are marked incomplete and apicp does not restore them. They are imported so that destroying this resource deletes them.", runID, strings.Join(incomplete, ", ")))
		return batch
	}

	run, err := r.client.GetAccountBackupRun(accountID, runID)
	if err != nil {
		diags.AddError("Error reading backup run", fmt.Sprintf("run %s of account %s: no archive of the account's backup list has that batch ID, and the run could not be read: %v (apicp keeps an account's latest 10 runs; import by archive IDs instead)", runID, accountID, err))
		return nil
	}
	if run.Status != client.AccountBackupRunFinished || len(run.Archives()) == 0 {
		diags.AddError("Backup run not finished", fmt.Sprintf("run %s is %s and has no backup to import yet", run.RunID, run.Status))
		return nil
	}
	return run.Archives()
}
