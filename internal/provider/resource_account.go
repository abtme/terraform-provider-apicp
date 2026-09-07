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

var _ resource.Resource = &AccountResource{}
var _ resource.ResourceWithImportState = &AccountResource{}

func NewAccountResource() resource.Resource { return &AccountResource{} }

type AccountResource struct{ client *client.Client }

type AccountResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Tier            types.String `tfsdk:"tier"`
	ParentAccountID types.String `tfsdk:"parent_account_id"`
	PackageID       types.String `tfsdk:"package_id"`
	CreatedAt       types.String `tfsdk:"created_at"`
}

func (r *AccountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account"
}

func (r *AccountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an apicp tenancy Account (PLAN.md §8 phase 1) — Admin → Reseller → User. An admin-authenticated provider may create a reseller or user account (or a second admin); a reseller-authenticated provider may only create user accounts, always parented to itself regardless of `parent_account_id`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"tier": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`admin`, `reseller`, or `user`. Changing this replaces the resource — apicp has no re-tiering API (would need to re-validate every descendant account).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"parent_account_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Defaults to the caller's own account. Ignored (and forced to the caller's own account) when the provider authenticates as a reseller. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()},
			},
			"package_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Resource-limit apicp_package to assign — must belong to the tier being created/patched (`for_tier=\"reseller\"` for a reseller account, `for_tier=\"user\"` for a user account) and must have been created by this same caller account. Omit for an admin account (admins are always unlimited).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"created_at": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *AccountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AccountResource) applyAccount(a *client.Account, m *AccountResourceModel) {
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.Tier = types.StringValue(a.Tier)
	m.ParentAccountID = types.StringValue(a.ParentAccountID)
	m.PackageID = types.StringValue(a.PackageID)
	m.CreatedAt = types.StringValue(a.CreatedAt)
}

func (r *AccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	a, err := r.client.CreateAccount(plan.Name.ValueString(), plan.Tier.ValueString(), plan.ParentAccountID.ValueString(), plan.PackageID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating account", err.Error())
		return
	}
	r.applyAccount(a, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	a, err := r.client.GetAccount(state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading account", err.Error())
		return
	}
	r.applyAccount(a, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update patches name/package_id only - tier and parent_account_id both
// require replace (see their PlanModifiers), so this never sees a change
// to either.
func (r *AccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := plan.Name.ValueString()
	packageID := plan.PackageID.ValueString()
	a, err := r.client.PatchAccount(plan.ID.ValueString(), &name, &packageID)
	if err != nil {
		resp.Diagnostics.AddError("Error updating account", err.Error())
		return
	}
	r.applyAccount(a, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteAccount(state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting account", err.Error())
	}
}

func (r *AccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
