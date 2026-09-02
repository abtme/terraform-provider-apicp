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

var _ resource.Resource = &TLSCertificateResource{}
var _ resource.ResourceWithImportState = &TLSCertificateResource{}

func NewTLSCertificateResource() resource.Resource { return &TLSCertificateResource{} }

type TLSCertificateResource struct{ client *client.Client }

type TLSCertificateResourceModel struct {
	ID        types.String `tfsdk:"id"`
	VhostID   types.String `tfsdk:"vhost_id"`
	Domain    types.String `tfsdk:"domain"`
	Status    types.String `tfsdk:"status"`
	NotBefore types.String `tfsdk:"not_before"`
	NotAfter  types.String `tfsdk:"not_after"`
	IssuedAt  types.String `tfsdk:"issued_at"`
}

func (r *TLSCertificateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tls_certificate"
}

func (r *TLSCertificateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Issues a Let's Encrypt TLS certificate (ACME HTTP-01) for an `apicp_web_domain` and installs it, enabling HTTPS on that vhost. Kept as a separate resource from `apicp_web_domain` so a domain can go HTTP-only -> HTTPS (or back) without recreating the vhost.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"vhost_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the `apicp_web_domain` to issue a certificate for. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"domain": schema.StringAttribute{Computed: true},
			"status": schema.StringAttribute{Computed: true},
			"not_before": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Certificate validity start (RFC 3339).",
			},
			"not_after": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Certificate validity end (RFC 3339) — apicp does not auto-renew yet; re-apply before this to renew.",
			},
			"issued_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *TLSCertificateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TLSCertificateResource) applyCert(cert *client.Certificate, m *TLSCertificateResourceModel) {
	m.ID = types.StringValue(cert.ID)
	m.VhostID = types.StringValue(cert.VhostID)
	m.Domain = types.StringValue(cert.Domain)
	m.Status = types.StringValue(cert.Status)
	m.NotBefore = types.StringValue(cert.NotBefore)
	m.NotAfter = types.StringValue(cert.NotAfter)
	m.IssuedAt = types.StringValue(cert.IssuedAt)
}

func (r *TLSCertificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan TLSCertificateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cert, err := r.client.IssueCertificate(plan.VhostID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error issuing TLS certificate", err.Error())
		return
	}
	r.applyCert(cert, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TLSCertificateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state TLSCertificateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cert, err := r.client.GetCertificate(state.VhostID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading TLS certificate", err.Error())
		return
	}
	r.applyCert(cert, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update — vhost_id is the only input attribute and it requires replace,
// so there is nothing left that can trigger an in-place update.
func (r *TLSCertificateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan TLSCertificateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TLSCertificateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state TLSCertificateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteCertificate(state.VhostID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting TLS certificate", err.Error())
	}
}

// ImportState accepts the vhost_id (there's no independent way to look up
// a certificate other than by the vhost it belongs to).
func (r *TLSCertificateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vhost_id"), req.ID)...)
}
