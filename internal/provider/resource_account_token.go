package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

// AccountTokenResource is apicp_account_token. apicp can mint tokens but
// has no endpoint to list or revoke them, so this resource is create-only.
type AccountTokenResource struct{ client *client.Client }

var _ resource.Resource = &AccountTokenResource{}

func NewAccountTokenResource() resource.Resource { return &AccountTokenResource{} }

type AccountTokenResourceModel struct {
	ID        types.String `tfsdk:"id"`
	AccountID types.String `tfsdk:"account_id"`
	Label     types.String `tfsdk:"label"`
	Scopes    types.Set    `tfsdk:"scopes"`
	TOTPCode  types.String `tfsdk:"totp_code"`
	Token     types.String `tfsdk:"token"`
}

func (r *AccountTokenResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_token"
}

func (r *AccountTokenResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Mints an API bearer token for an `apicp_account`, scoped to the scopes you list. An admin can mint any scopes for any account; a reseller only for itself or its descendants, and never `*` or a scope it doesn't hold itself. The token value is returned once, at creation, and is kept (sensitive) in Terraform state - treat the state as a secret. **apicp has no way to list or revoke tokens**, so destroying this resource only removes it from state: the token stays valid on the server. It is also why there is no import. To rotate, replace the resource, then revoke the old token on apicp's side out of band. If the account has `apicp_totp` enabled, apicp requires a current `totp_code` to mint.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"account_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the `apicp_account` the token belongs to. Changing this replaces the resource.",
				PlanModifiers:       replace,
			},
			"label": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Free-text name for the token. Changing this replaces the resource.",
				PlanModifiers:       replace,
			},
			"scopes": schema.SetAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Scopes the token holds, e.g. `[\"mail:read\", \"mail:write\"]`. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.Set{setplanmodifier.RequiresReplace()},
			},
			"totp_code": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Current one-time code, needed only when the account has TOTP enabled. Codes expire in seconds, so this is only used at creation and is never re-checked.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"token": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The bearer token. Only available from the apply that created it.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *AccountTokenResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AccountTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AccountTokenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var scopes []string
	resp.Diagnostics.Append(plan.Scopes.ElementsAs(ctx, &scopes, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tok, err := r.client.CreateAccountToken(plan.AccountID.ValueString(), plan.Label.ValueString(), scopes, plan.TOTPCode.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating account token", err.Error())
		return
	}
	plan.ID = types.StringValue(plan.AccountID.ValueString() + "/" + plan.Label.ValueString())
	plan.Token = types.StringValue(tok.Token)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read keeps state as it is: apicp cannot report on a token after minting it.
func (r *AccountTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AccountTokenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update - every configurable attribute replaces (totp_code is only used at creation).
func (r *AccountTokenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state AccountTokenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	plan.ID, plan.Token = state.ID, state.Token
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete only forgets the token: apicp has no revoke endpoint.
func (r *AccountTokenResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}
