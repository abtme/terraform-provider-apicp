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

// MailDomainTLSResource is apicp_mail_domain_tls - per apicp_mail_domain,
// same shape as apicp_mail_client_config.
type MailDomainTLSResource struct{ client *client.Client }

var _ resource.Resource = &MailDomainTLSResource{}
var _ resource.ResourceWithImportState = &MailDomainTLSResource{}

func NewMailDomainTLSResource() resource.Resource { return &MailDomainTLSResource{} }

type MailDomainTLSResourceModel struct {
	ID           types.String `tfsdk:"id"`
	MailDomainID types.String `tfsdk:"mail_domain_id"`
	Host         types.String `tfsdk:"host"`
	NotBefore    types.String `tfsdk:"not_before"`
	NotAfter     types.String `tfsdk:"not_after"`
}

func (r *MailDomainTLSResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mail_domain_tls"
}

func (r *MailDomainTLSResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Gives an `apicp_mail_domain` its own `mail.<domain>` TLS certificate, so the domain's mail clients connect to a name under the domain itself and it no longer depends on the server's own host name (moving or renaming the server only changes that domain's `mail.<domain>` A record). Opt-in per domain; domains without this resource keep using the node's shared certificate, which stays the default for clients that send no server name. Issues a Let's Encrypt certificate over HTTP-01, so `mail.<domain>` must already resolve to the node and port 80 be reachable, or apply fails and the node is left unchanged. Requires `apicp_mail_tls` (use `depends_on`) and Postfix 3.4+. Renewed automatically. Create this **before** `apicp_mail_client_config` for the same domain, so the published records name `mail.<domain>`; apicp refuses either order change while the other exists.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mail_domain_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the `apicp_mail_domain`. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"host": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The certificate's name, `mail.<domain>`: what the domain's mail clients connect to.",
			},
			"not_before": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Start of the certificate's validity.",
			},
			"not_after": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "End of the certificate's validity. Changes when apicp renews it.",
			},
		},
	}
}

func (r *MailDomainTLSResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *MailDomainTLSResource) apply(t *client.MailDomainTLS, m *MailDomainTLSResourceModel) {
	m.ID = types.StringValue(t.MailDomainID)
	m.MailDomainID = types.StringValue(t.MailDomainID)
	m.Host = types.StringValue(t.Host)
	m.NotBefore = types.StringValue(t.NotBefore)
	m.NotAfter = types.StringValue(t.NotAfter)
}

func (r *MailDomainTLSResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan MailDomainTLSResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	t, err := r.client.EnableMailDomainTLS(plan.MailDomainID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error issuing the mail domain certificate", err.Error())
		return
	}
	r.apply(t, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MailDomainTLSResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state MailDomainTLSResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	t, err := r.client.GetMailDomainTLS(state.MailDomainID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading the mail domain certificate", err.Error())
		return
	}
	r.apply(t, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update - everything but mail_domain_id is Computed (and that replaces);
// implemented to satisfy the interface.
func (r *MailDomainTLSResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan MailDomainTLSResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MailDomainTLSResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state MailDomainTLSResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DisableMailDomainTLS(state.MailDomainID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Error removing the mail domain certificate", err.Error())
	}
}

// ImportState accepts the mail_domain_id.
func (r *MailDomainTLSResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mail_domain_id"), req.ID)...)
}
