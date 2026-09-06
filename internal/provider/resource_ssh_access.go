package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

var _ resource.Resource = &SSHAccessResource{}
var _ resource.ResourceWithImportState = &SSHAccessResource{}

func NewSSHAccessResource() resource.Resource { return &SSHAccessResource{} }

type SSHAccessResource struct{ client *client.Client }

type SSHAccessResourceModel struct {
	ID        types.String `tfsdk:"id"`
	VhostID   types.String `tfsdk:"vhost_id"`
	PublicKey types.String `tfsdk:"public_key"`
	UnixUser  types.String `tfsdk:"unix_user"`
	Status    types.String `tfsdk:"status"`
}

func (r *SSHAccessResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssh_access"
}

func (r *SSHAccessResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Grants real SSH/SFTP login on an `apicp_web_domain`'s own Unix user, using a caller-supplied public key. Key-only — apicp never generates or stores a password for this, and never installs a separate FTP daemon; access reuses the node's existing sshd.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"vhost_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the `apicp_web_domain` to grant SSH/SFTP access to. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"public_key": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "SSH public key (e.g. `ssh-ed25519 AAAA... comment`) authorized to log in. Changing this rotates the installed key in place.",
			},
			"unix_user": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The vhost's own Unix user this grant applies to — connect as `ssh <unix_user>@<node host>`.",
			},
			"status": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *SSHAccessResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SSHAccessResource) applyAccess(a *client.SSHAccess, m *SSHAccessResourceModel) {
	m.ID = types.StringValue(a.ID)
	m.VhostID = types.StringValue(a.VhostID)
	m.PublicKey = types.StringValue(a.PublicKey)
	m.UnixUser = types.StringValue(a.UnixUser)
	m.Status = types.StringValue(a.Status)
}

func (r *SSHAccessResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SSHAccessResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	a, err := r.client.EnableSSHAccess(plan.VhostID.ValueString(), plan.PublicKey.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error enabling ssh access", err.Error())
		return
	}
	r.applyAccess(a, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SSHAccessResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SSHAccessResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	a, err := r.client.GetSSHAccess(state.VhostID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading ssh access", err.Error())
		return
	}
	r.applyAccess(a, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update re-sends the (possibly rotated) public_key — apicp's
// EnableSSHAccess endpoint is idempotent and simply overwrites
// authorized_keys, so a key rotation doesn't need RequiresReplace.
func (r *SSHAccessResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SSHAccessResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	a, err := r.client.EnableSSHAccess(plan.VhostID.ValueString(), plan.PublicKey.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error rotating ssh access key", err.Error())
		return
	}
	r.applyAccess(a, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SSHAccessResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SSHAccessResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DisableSSHAccess(state.VhostID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error disabling ssh access", err.Error())
	}
}

// ImportState accepts the vhost_id — same rationale as
// apicp_tls_certificate's ImportState.
func (r *SSHAccessResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vhost_id"), req.ID)...)
}
