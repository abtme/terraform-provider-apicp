package provider

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

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

// SSHAccessResourceModel represents ONE key's grant on a vhost, not the
// vhost's whole authorized set — apicp's ssh-access API is additive
// (PLAN.md-era single-key-per-vhost was replaced 2026-09-17 with a real
// set), so several apicp_ssh_access resources can target the same
// vhost_id with different public_key values and coexist: each one's
// Create/Delete adds/removes just its own key, never touching the
// others'.
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
		MarkdownDescription: "Grants real SSH/SFTP login on an `apicp_web_domain`'s own Unix user, using a caller-supplied public key. Key-only — apicp never generates or stores a password for this, and never installs a separate FTP daemon; access reuses the node's existing sshd. Several `apicp_ssh_access` resources can target the same `vhost_id` with different keys — each grants/revokes only its own key, so multiple people can each have their own key active on the same vhost at once.",
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
				MarkdownDescription: "SSH public key (e.g. `ssh-ed25519 AAAA... comment`) authorized to log in. Changing this revokes the old key and grants the new one (replaces the resource) — it no longer rewrites a key in place, since apicp's API now supports several keys being active on the same vhost simultaneously and \"rotating in place\" would be ambiguous about which key to remove.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
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

// applyAccess fills in the fields apicp's response actually tells us
// about this grant as a whole (id, unix_user, status) — PublicKey is
// deliberately left untouched by the caller, since a's PublicKeys is the
// vhost's complete set, not this one resource's own key.
func (r *SSHAccessResource) applyAccess(a *client.SSHAccess, m *SSHAccessResourceModel) {
	m.ID = types.StringValue(a.ID)
	m.VhostID = types.StringValue(a.VhostID)
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
	// The vhost has SOME keys authorized, but is this resource's own key
	// still one of them? Someone could have revoked just this key
	// out-of-band (or every key, which the 404 case above already
	// covers) without touching Terraform.
	if !slices.Contains(a.PublicKeys, state.PublicKey.ValueString()) {
		resp.State.RemoveResource(ctx)
		return
	}
	r.applyAccess(a, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable in practice — vhost_id and public_key both
// RequiresReplace, and every other attribute is Computed — but the
// resource.Resource interface requires an implementation regardless.
// Mirrors Create's behavior (idempotent: the key is already granted).
func (r *SSHAccessResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
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

func (r *SSHAccessResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SSHAccessResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DisableSSHAccessKey(state.VhostID.ValueString(), state.PublicKey.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error disabling ssh access", err.Error())
	}
}

// ImportState accepts "vhost_id/public_key" — vhost_id alone is no
// longer enough to identify a single grant now that several can coexist
// on the same vhost.
func (r *SSHAccessResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected format: vhost_id/public_key")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vhost_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("public_key"), parts[1])...)
}
