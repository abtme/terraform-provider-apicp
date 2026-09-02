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

var _ resource.Resource = &DatabaseResource{}
var _ resource.ResourceWithImportState = &DatabaseResource{}

func NewDatabaseResource() resource.Resource { return &DatabaseResource{} }

type DatabaseResource struct{ client *client.Client }

type DatabaseResourceModel struct {
	ID       types.String `tfsdk:"id"`
	Engine   types.String `tfsdk:"engine"`
	Name     types.String `tfsdk:"name"`
	Username types.String `tfsdk:"username"`
	NodeID   types.String `tfsdk:"node_id"`
	Status   types.String `tfsdk:"status"`
	Password types.String `tfsdk:"password"`
}

func (r *DatabaseResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database"
}

func (r *DatabaseResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a MySQL/MariaDB or PostgreSQL database (plus a matching admin user of the same name) via apicp. The generated password is returned exactly once, on create — apicp never stores or re-returns it, so `password` is only known immediately after `terraform apply` creates the resource; it reads back as unknown (not empty) after an import or a refresh of an existing resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"engine": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`mysql` (MariaDB) or `postgresql`. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Database (and matching admin username) — apicp's current MVP uses one user per database, named the same as the database. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"username": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"node_id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status": schema.StringAttribute{Computed: true},
			"password": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Generated database password. Only populated in the apply that creates this resource — see the resource description.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *DatabaseResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// applyDatabase copies every field GET/DELETE endpoints actually return.
// Password is deliberately not touched here — Create sets it separately,
// since GET never returns it (see the resource's own MarkdownDescription).
func (r *DatabaseResource) applyDatabase(d *client.Database, m *DatabaseResourceModel) {
	m.ID = types.StringValue(d.ID)
	m.Engine = types.StringValue(d.Engine)
	m.Name = types.StringValue(d.Name)
	m.Username = types.StringValue(d.Username)
	m.NodeID = types.StringValue(d.NodeID)
	m.Status = types.StringValue(d.Status)
}

func (r *DatabaseResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DatabaseResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d, err := r.client.CreateDatabase(plan.Engine.ValueString(), plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating database", err.Error())
		return
	}
	r.applyDatabase(d, &plan)
	plan.Password = types.StringValue(d.Password)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabaseResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DatabaseResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d, err := r.client.GetDatabase(state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading database", err.Error())
		return
	}
	r.applyDatabase(d, &state) // state.Password stays whatever it already was
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update — every input attribute requires replace, so this never fires in
// practice; implemented to satisfy the resource.Resource interface.
func (r *DatabaseResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DatabaseResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatabaseResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DatabaseResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteDatabase(state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting database", err.Error())
	}
}

func (r *DatabaseResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
