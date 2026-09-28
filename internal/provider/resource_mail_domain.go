package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

var _ resource.Resource = &MailDomainResource{}
var _ resource.ResourceWithImportState = &MailDomainResource{}

func NewMailDomainResource() resource.Resource { return &MailDomainResource{} }

type MailDomainResource struct{ client *client.Client }

type MailDomainResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	UnixUser    types.String `tfsdk:"unix_user"`
	DisableIMAP types.Bool   `tfsdk:"disable_imap"`
	DisablePOP3 types.Bool   `tfsdk:"disable_pop3"`
	Status      types.String `tfsdk:"status"`
}

func (m *MailDomainResourceModel) protocols() client.Protocols {
	return client.Protocols{DisableIMAP: m.DisableIMAP.ValueBool(), DisablePOP3: m.DisablePOP3.ValueBool()}
}

func (r *MailDomainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mail_domain"
}

func (r *MailDomainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a mail domain (Postfix + Dovecot, SQL-backed virtual maps) via apicp. Creates a dedicated Unix user for this domain's mailbox storage — see `unix_user`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Mail domain, e.g. `example.com`. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"unix_user": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Dedicated Unix user apicp created for this domain's mailbox storage.",
			},
			"disable_imap": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Switch IMAP off for every mailbox in this domain (logins over IMAP fail; webmail uses IMAP, so it stops too). Mail is still delivered. Default `false`.",
			},
			"disable_pop3": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Switch POP3 off for every mailbox in this domain. Mail is still delivered. Default `false`.",
			},
			"status": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *MailDomainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *MailDomainResource) applyDomain(d *client.MailDomain, m *MailDomainResourceModel) {
	m.ID = types.StringValue(d.ID)
	m.Name = types.StringValue(d.Name)
	m.UnixUser = types.StringValue(d.UnixUser)
	m.DisableIMAP = types.BoolValue(d.DisableIMAP)
	m.DisablePOP3 = types.BoolValue(d.DisablePOP3)
	m.Status = types.StringValue(d.Status)
}

func (r *MailDomainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan MailDomainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d, err := r.client.CreateMailDomain(plan.Name.ValueString(), plan.protocols())
	if err != nil {
		resp.Diagnostics.AddError("Error creating mail domain", err.Error())
		return
	}
	r.applyDomain(d, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MailDomainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state MailDomainResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d, err := r.client.GetMailDomain(state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading mail domain", err.Error())
		return
	}
	r.applyDomain(d, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update - name requires replace, so only the protocol switches change here.
func (r *MailDomainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state MailDomainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.SetMailDomainProtocols(state.ID.ValueString(), plan.protocols())
	if err != nil {
		resp.Diagnostics.AddError("Error updating mail domain", err.Error())
		return
	}
	r.applyDomain(d, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MailDomainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state MailDomainResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteMailDomain(state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting mail domain", err.Error())
	}
}

func (r *MailDomainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
