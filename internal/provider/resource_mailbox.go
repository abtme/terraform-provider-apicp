package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

var _ resource.Resource = &MailboxResource{}
var _ resource.ResourceWithImportState = &MailboxResource{}

func NewMailboxResource() resource.Resource { return &MailboxResource{} }

type MailboxResource struct{ client *client.Client }

type MailboxResourceModel struct {
	ID           types.String `tfsdk:"id"`
	MailDomainID types.String `tfsdk:"mail_domain_id"`
	LocalPart    types.String `tfsdk:"local_part"`
	Email        types.String `tfsdk:"email"`
	Status       types.String `tfsdk:"status"`
	Password     types.String `tfsdk:"password"`
}

func (r *MailboxResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mailbox"
}

func (r *MailboxResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a mailbox within an `apicp_mail_domain`. Like `apicp_database`'s password, the generated mailbox password is returned exactly once by apicp, on create — see `password`'s own description. apicp supports resetting a mailbox's password, but that isn't exposed as a Terraform-managed attribute (a secret rotation isn't naturally expressed as a plan diff) — use apicp's API directly for that.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mail_domain_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the `apicp_mail_domain` this mailbox belongs to. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"local_part": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Local part of the email address (before the `@`). Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"email": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status": schema.StringAttribute{Computed: true},
			"password": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Generated mailbox password. Only populated in the apply that creates this resource — see the resource description.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *MailboxResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// applyMailbox copies every field GET/DELETE endpoints actually return.
// Password is deliberately not touched here — see resource_database.go's
// applyDatabase for the same reasoning.
func (r *MailboxResource) applyMailbox(mbox *client.Mailbox, m *MailboxResourceModel) {
	m.ID = types.StringValue(mbox.ID)
	m.MailDomainID = types.StringValue(mbox.MailDomainID)
	m.LocalPart = types.StringValue(mbox.LocalPart)
	m.Email = types.StringValue(mbox.Email)
	m.Status = types.StringValue(mbox.Status)
}

func (r *MailboxResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan MailboxResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mbox, err := r.client.CreateMailbox(plan.MailDomainID.ValueString(), plan.LocalPart.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating mailbox", err.Error())
		return
	}
	r.applyMailbox(mbox, &plan)
	plan.Password = types.StringValue(mbox.Password)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MailboxResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state MailboxResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mbox, err := r.client.GetMailbox(state.MailDomainID.ValueString(), state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading mailbox", err.Error())
		return
	}
	r.applyMailbox(mbox, &state) // state.Password stays whatever it already was
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update — every input attribute requires replace, so this never fires in
// practice; implemented to satisfy the resource.Resource interface.
func (r *MailboxResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan MailboxResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MailboxResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state MailboxResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteMailbox(state.MailDomainID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting mailbox", err.Error())
	}
}

// ImportState accepts "mail_domain_id/mailbox_id".
func (r *MailboxResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected format: mail_domain_id/mailbox_id")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mail_domain_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
