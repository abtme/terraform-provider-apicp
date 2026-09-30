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

// MailTLSResource is apicp_mail_tls: a node-wide toggle like apicp_antispam.
type MailTLSResource struct{ client *client.Client }

var _ resource.Resource = &MailTLSResource{}
var _ resource.ResourceWithImportState = &MailTLSResource{}

func NewMailTLSResource() resource.Resource { return &MailTLSResource{} }

type MailTLSResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Enabled        types.Bool   `tfsdk:"enabled"`
	Host           types.String `tfsdk:"host"`
	SubmissionPort types.Int64  `tfsdk:"submission_port"`
}

func (r *MailTLSResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mail_tls"
}

func (r *MailTLSResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Turns on TLS for the whole mail server (IMAP/POP3 STARTTLS and SMTP submission on port 587) on apicp's control-plane node, using the control-plane hostname's certificate, which apicp issues if it doesn't exist yet. Requires an admin-tier provider token. No configurable arguments: declaring this resource enables it, destroying it switches it back off.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{Computed: true},
			"host": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Hostname mail clients connect to (the control-plane host).",
			},
			"submission_port": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "SMTP submission port (587).",
			},
		},
	}
}

func (r *MailTLSResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *MailTLSResource) apply(st *client.MailTLSStatus, m *MailTLSResourceModel) {
	m.ID = types.StringValue("mail_tls")
	m.Enabled = types.BoolValue(st.Enabled)
	m.Host = types.StringValue(st.Host)
	m.SubmissionPort = types.Int64Value(int64(st.SubmissionPort))
}

func (r *MailTLSResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan MailTLSResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	st, err := r.client.EnableMailTLS()
	if err != nil {
		resp.Diagnostics.AddError("Error enabling mail tls", err.Error())
		return
	}
	r.apply(st, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MailTLSResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state MailTLSResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	st, err := r.client.GetMailTLS()
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading mail tls", err.Error())
		return
	}
	if !st.Enabled {
		// Disabled out-of-band: the resource no longer reflects real state.
		resp.State.RemoveResource(ctx)
		return
	}
	r.apply(st, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update - every attribute is Computed; implemented to satisfy the interface.
func (r *MailTLSResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan MailTLSResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MailTLSResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.DisableMailTLS(); err != nil {
		resp.Diagnostics.AddError("Error disabling mail tls", err.Error())
	}
}

func (r *MailTLSResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
