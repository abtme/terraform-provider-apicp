package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

var _ resource.Resource = &PackageResource{}
var _ resource.ResourceWithImportState = &PackageResource{}

func NewPackageResource() resource.Resource { return &PackageResource{} }

type PackageResource struct{ client *client.Client }

// PackageResourceModel is deliberately flattened (no nested "limits"
// object) - simpler to write and to reference from other configs than a
// nested block, and this provider has no existing precedent for a nested
// object attribute to follow.
type PackageResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	OwnerAccountID types.String `tfsdk:"owner_account_id"`
	ForTier        types.String `tfsdk:"for_tier"`
	MaxWebDomains  types.Int64  `tfsdk:"max_web_domains"`
	MaxDatabases   types.Int64  `tfsdk:"max_databases"`
	MaxMailDomains types.Int64  `tfsdk:"max_mail_domains"`
	MaxMailboxes   types.Int64  `tfsdk:"max_mailboxes"`
	MaxCronJobs    types.Int64  `tfsdk:"max_cron_jobs"`
	MaxSubAccounts types.Int64  `tfsdk:"max_sub_accounts"`
}

func (r *PackageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_package"
}

func (r *PackageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an apicp resource-limit Package, assignable to a reseller or user apicp_account (PLAN.md §8 phase 1). Packages have no update API — apicp only supports create/read/delete, so changing any attribute replaces the resource. A limit left unset defaults to 0, which means 'not allowed', not 'unlimited' — there is no unlimited value.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"owner_account_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The account that created this package (resolved from the caller's own bearer token, not settable) — an admin's packages are usable by any account it assigns them to, a reseller's only by its own users.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"for_tier": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`reseller` or `user` — which account tier this package may be assigned to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"max_web_domains":  packageLimitAttribute(),
			"max_databases":    packageLimitAttribute(),
			"max_mail_domains": packageLimitAttribute(),
			"max_mailboxes":    packageLimitAttribute(),
			"max_cron_jobs":    packageLimitAttribute(),
			"max_sub_accounts": schemaWithDescription(packageLimitAttribute(), "Only meaningful on a for_tier=\"reseller\" package — how many user accounts that reseller may create."),
		},
	}
}

func packageLimitAttribute() schema.Int64Attribute {
	return schema.Int64Attribute{
		Optional:      true,
		Computed:      true,
		Default:       int64default.StaticInt64(0),
		PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
	}
}

func schemaWithDescription(a schema.Int64Attribute, desc string) schema.Int64Attribute {
	a.MarkdownDescription = desc
	return a
}

func (r *PackageResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *PackageResource) applyPackage(p *client.Package, m *PackageResourceModel) {
	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	m.OwnerAccountID = types.StringValue(p.OwnerAccountID)
	m.ForTier = types.StringValue(p.ForTier)
	m.MaxWebDomains = types.Int64Value(int64(p.Limits.MaxWebDomains))
	m.MaxDatabases = types.Int64Value(int64(p.Limits.MaxDatabases))
	m.MaxMailDomains = types.Int64Value(int64(p.Limits.MaxMailDomains))
	m.MaxMailboxes = types.Int64Value(int64(p.Limits.MaxMailboxes))
	m.MaxCronJobs = types.Int64Value(int64(p.Limits.MaxCronJobs))
	m.MaxSubAccounts = types.Int64Value(int64(p.Limits.MaxSubAccounts))
}

func (r *PackageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PackageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	limits := client.PackageLimits{
		MaxWebDomains:  int(plan.MaxWebDomains.ValueInt64()),
		MaxDatabases:   int(plan.MaxDatabases.ValueInt64()),
		MaxMailDomains: int(plan.MaxMailDomains.ValueInt64()),
		MaxMailboxes:   int(plan.MaxMailboxes.ValueInt64()),
		MaxCronJobs:    int(plan.MaxCronJobs.ValueInt64()),
		MaxSubAccounts: int(plan.MaxSubAccounts.ValueInt64()),
	}
	p, err := r.client.CreatePackage(plan.Name.ValueString(), plan.ForTier.ValueString(), limits)
	if err != nil {
		resp.Diagnostics.AddError("Error creating package", err.Error())
		return
	}
	r.applyPackage(p, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PackageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PackageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	p, err := r.client.GetPackage(state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading package", err.Error())
		return
	}
	r.applyPackage(p, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update — every attribute requires replace, so this never fires in
// practice; implemented to satisfy the resource.Resource interface.
func (r *PackageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan PackageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PackageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PackageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeletePackage(state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting package", err.Error())
	}
}

func (r *PackageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
