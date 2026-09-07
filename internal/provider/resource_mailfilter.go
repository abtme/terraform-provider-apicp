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

// mailFilterResource backs both apicp_antispam (Rspamd) and
// apicp_antivirus (ClamAV) (PLAN.md §8 phase 6) - same identical shape
// and behavior as adminToolResource backing apicp_phpmyadmin/
// apicp_phppgadmin, so implemented once and instantiated per toggle
// rather than duplicated. Node-wide admin-gated toggles, unlike
// apicp_dkim's per-mail-domain shape - confirmed with the user before
// building: spam/AV filtering wires into Postfix as one milter chain for
// the whole mail server, not per domain.
type mailFilterResource struct {
	client   *client.Client
	typeName string
	enable   func(*client.Client) (*client.MailFilterStatus, error)
	get      func(*client.Client) (*client.MailFilterStatus, error)
	disable  func(*client.Client) error
}

var _ resource.Resource = &mailFilterResource{}
var _ resource.ResourceWithImportState = &mailFilterResource{}

func NewAntispamResource() resource.Resource {
	return &mailFilterResource{
		typeName: "antispam",
		enable:   (*client.Client).EnableAntispam,
		get:      (*client.Client).GetAntispam,
		disable:  (*client.Client).DisableAntispam,
	}
}

func NewAntivirusResource() resource.Resource {
	return &mailFilterResource{
		typeName: "antivirus",
		enable:   (*client.Client).EnableAntivirus,
		get:      (*client.Client).GetAntivirus,
		disable:  (*client.Client).DisableAntivirus,
	}
}

type MailFilterResourceModel struct {
	ID      types.String `tfsdk:"id"`
	Enabled types.Bool   `tfsdk:"enabled"`
}

func (r *mailFilterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.typeName
}

func (r *mailFilterResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf(
			"Enables %s for the whole mail server on apicp's control-plane node (PLAN.md §8 phase 6). Requires an admin-tier provider token. No configurable arguments: declaring this resource enables it, destroying it fully uninstalls the package and removes it from Postfix's milter chain.",
			r.typeName,
		),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{Computed: true},
		},
	}
}

func (r *mailFilterResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *mailFilterResource) apply(st *client.MailFilterStatus, m *MailFilterResourceModel) {
	m.ID = types.StringValue(r.typeName)
	m.Enabled = types.BoolValue(st.Enabled)
}

func (r *mailFilterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan MailFilterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	st, err := r.enable(r.client)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Error enabling %s", r.typeName), err.Error())
		return
	}
	r.apply(st, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mailFilterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state MailFilterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	st, err := r.get(r.client)
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Error reading %s", r.typeName), err.Error())
		return
	}
	if !st.Enabled {
		// Disabled out-of-band - the resource no longer reflects real
		// state, same reasoning as adminToolResource's Read.
		resp.State.RemoveResource(ctx)
		return
	}
	r.apply(st, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update — every attribute is Computed, nothing in config can change;
// implemented to satisfy the resource.Resource interface.
func (r *mailFilterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan MailFilterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mailFilterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.disable(r.client); err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Error disabling %s", r.typeName), err.Error())
	}
}

func (r *mailFilterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
