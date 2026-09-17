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

// TOTPResource is apicp_totp (PLAN.md §8 phase 10) - enables TOTP-gated
// token issuance for an apicp_account. Same "fully Computed, secret
// returned once" shape as apicp_dkim, except apicp's own GET
// .../totp never 404s (it always reports {"enabled": bool}) - so unlike
// apicp_dkim/apicp_smtp_relay, Read distinguishes "gone" by checking
// that boolean itself rather than an HTTP 404, and RemoveResource's
// itself when it comes back false (e.g. an admin disabled it directly
// against the API, outside Terraform).
type TOTPResource struct{ client *client.Client }

var _ resource.Resource = &TOTPResource{}
var _ resource.ResourceWithImportState = &TOTPResource{}

func NewTOTPResource() resource.Resource { return &TOTPResource{} }

type TOTPResourceModel struct {
	ID         types.String `tfsdk:"id"`
	AccountID  types.String `tfsdk:"account_id"`
	Secret     types.String `tfsdk:"secret"`
	OTPAuthURL types.String `tfsdk:"otpauth_url"`
}

func (r *TOTPResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_totp"
}

func (r *TOTPResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Enables TOTP-gated token issuance for an `apicp_account` (PLAN.md §8 phase 10) - apicp generates the secret server-side. `secret`/`otpauth_url` are only known immediately after the `terraform apply` that creates this resource; apicp never returns the secret again after that (it reads back as unknown, not empty, after an import or a refresh of an existing resource) - re-enrolling (e.g. to reset a lost authenticator) means tainting and recreating this resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Same value as `account_id` - apicp has at most one TOTP enrollment per account.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"account_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the `apicp_account` to enroll. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"secret": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Base32 TOTP secret. Only populated in the apply that creates this resource - see the resource description.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"otpauth_url": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "otpauth:// URL an authenticator app can import directly (encodes the same secret) - render this as a QR code yourself if needed, apicp doesn't generate one.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *TOTPResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TOTPResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan TOTPResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	e, err := r.client.EnableTOTP(plan.AccountID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error enabling totp", err.Error())
		return
	}
	plan.ID = types.StringValue(e.AccountID)
	plan.Secret = types.StringValue(e.Secret)
	plan.OTPAuthURL = types.StringValue(e.OTPAuthURL)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TOTPResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state TOTPResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	enabled, err := r.client.GetTOTPStatus(state.AccountID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading totp status", err.Error())
		return
	}
	if !enabled {
		resp.State.RemoveResource(ctx)
		return
	}
	// secret/otpauth_url stay whatever they already were - apicp's GET
	// never returns them, see the resource's own doc comment.
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update - account_id is the only input and it forces replacement, so
// this never fires in practice; implemented to satisfy resource.Resource.
func (r *TOTPResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan TOTPResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TOTPResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state TOTPResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DisableTOTP(state.AccountID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error disabling totp", err.Error())
	}
}

// ImportState accepts the account_id - same rationale as apicp_dkim's
// ImportState. secret/otpauth_url can't be recovered on import (apicp
// never returns them again) - they read back as empty until the next
// recreate.
func (r *TOTPResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("account_id"), req.ID)...)
}
