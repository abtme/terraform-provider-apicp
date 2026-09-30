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

// AccountBackupResource is apicp_account_backup: one point-in-time backup.
type AccountBackupResource struct{ client *client.Client }

var _ resource.Resource = &AccountBackupResource{}
var _ resource.ResourceWithImportState = &AccountBackupResource{}

func NewAccountBackupResource() resource.Resource { return &AccountBackupResource{} }

type AccountBackupResourceModel struct {
	ID        types.String `tfsdk:"id"`
	AccountID types.String `tfsdk:"account_id"`
	CreatedAt types.String `tfsdk:"created_at"`
	SizeBytes types.Int64  `tfsdk:"size_bytes"`
}

func (r *AccountBackupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_backup"
}

func (r *AccountBackupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Takes one backup of an `apicp_account` - everything it owns (website docroots, mailboxes, database dumps) in a single archive stored on apicp. It is a point-in-time snapshot: applying creates one backup, and a later apply does not take another. To take a fresh one, replace the resource (`terraform apply -replace=...`, or `replace_triggered_by`). Destroying deletes the stored backup. Restoring is an action, not a state, so it is not exposed here - use `POST /v1/accounts/{id}/backups/{backup_id}/restore` on apicp's API.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"account_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the `apicp_account` to back up. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"created_at": schema.StringAttribute{Computed: true, MarkdownDescription: "When the backup was taken (RFC3339)."},
			"size_bytes": schema.Int64Attribute{Computed: true, MarkdownDescription: "Size of the stored archive."},
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

func (r *AccountBackupResource) apply(b *client.AccountBackup, m *AccountBackupResourceModel) {
	m.ID = types.StringValue(b.ID)
	m.AccountID = types.StringValue(b.AccountID)
	m.CreatedAt = types.StringValue(b.CreatedAt)
	m.SizeBytes = types.Int64Value(b.SizeBytes)
}

func (r *AccountBackupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AccountBackupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	b, err := r.client.CreateAccountBackup(plan.AccountID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating account backup", err.Error())
		return
	}
	r.apply(b, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AccountBackupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AccountBackupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	b, err := r.client.GetAccountBackup(state.AccountID.ValueString(), state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading account backup", err.Error())
		return
	}
	r.apply(b, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update - account_id replaces and everything else is Computed.
func (r *AccountBackupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AccountBackupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AccountBackupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AccountBackupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteAccountBackup(state.AccountID.ValueString(), state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Error deleting account backup", err.Error())
	}
}

// ImportState accepts "account_id/backup_id".
func (r *AccountBackupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected format: account_id/backup_id")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("account_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
