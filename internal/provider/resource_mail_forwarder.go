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

var _ resource.Resource = &MailForwarderResource{}
var _ resource.ResourceWithImportState = &MailForwarderResource{}

func NewMailForwarderResource() resource.Resource { return &MailForwarderResource{} }

type MailForwarderResource struct{ client *client.Client }

type MailForwarderResourceModel struct {
	ID           types.String `tfsdk:"id"`
	MailDomainID types.String `tfsdk:"mail_domain_id"`
	LocalPart    types.String `tfsdk:"local_part"`
	Email        types.String `tfsdk:"email"`
	Destinations types.Set    `tfsdk:"destinations"`
	Status       types.String `tfsdk:"status"`
}

func (r *MailForwarderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mail_forwarder"
}

func (r *MailForwarderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an email forwarder within an `apicp_mail_domain`: mail for `local_part@domain` is redirected to one or more addresses, on any domain. A forwarder takes precedence over an `apicp_mailbox` with the same address — list the address itself among `destinations` to keep a copy in the mailbox. With `local_part = \"*\"` it is the domain's catch-all, receiving mail for addresses that have neither a mailbox nor a forwarder.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mail_domain_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the `apicp_mail_domain` this forwarder belongs to. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"local_part": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Local part of the forwarded address (before the `@`), or `*` for the domain's catch-all. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"destinations": schema.SetAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Addresses the mail is forwarded to (1 to 20, lower-case, bare `user@host` form). For a catch-all, a destination in the same domain must be an existing mailbox, or apicp refuses it (it would loop back into the catch-all).",
			},
			"email": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *MailForwarderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *MailForwarderResource) apply(ctx context.Context, f *client.Forwarder, m *MailForwarderResourceModel) error {
	dests, diags := types.SetValueFrom(ctx, types.StringType, f.Destinations)
	if diags.HasError() {
		return fmt.Errorf("destinations: %s", diags.Errors()[0].Detail())
	}
	m.ID = types.StringValue(f.ID)
	m.MailDomainID = types.StringValue(f.MailDomainID)
	m.LocalPart = types.StringValue(f.LocalPart)
	m.Email = types.StringValue(f.Email)
	m.Destinations = dests
	m.Status = types.StringValue(f.Status)
	return nil
}

func destinationsOf(ctx context.Context, s types.Set) ([]string, error) {
	var out []string
	if diags := s.ElementsAs(ctx, &out, false); diags.HasError() {
		return nil, fmt.Errorf("%s", diags.Errors()[0].Detail())
	}
	return out, nil
}

func (r *MailForwarderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan MailForwarderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	dests, err := destinationsOf(ctx, plan.Destinations)
	if err != nil {
		resp.Diagnostics.AddError("Invalid destinations", err.Error())
		return
	}
	f, err := r.client.CreateForwarder(plan.MailDomainID.ValueString(), plan.LocalPart.ValueString(), dests)
	if err != nil {
		resp.Diagnostics.AddError("Error creating forwarder", err.Error())
		return
	}
	if err := r.apply(ctx, f, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading forwarder", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MailForwarderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state MailForwarderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	f, err := r.client.GetForwarder(state.MailDomainID.ValueString(), state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading forwarder", err.Error())
		return
	}
	if err := r.apply(ctx, f, &state); err != nil {
		resp.Diagnostics.AddError("Error reading forwarder", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update - only the destinations change in place; everything else replaces.
func (r *MailForwarderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state MailForwarderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	dests, err := destinationsOf(ctx, plan.Destinations)
	if err != nil {
		resp.Diagnostics.AddError("Invalid destinations", err.Error())
		return
	}
	f, err := r.client.UpdateForwarder(state.MailDomainID.ValueString(), state.ID.ValueString(), dests)
	if err != nil {
		resp.Diagnostics.AddError("Error updating forwarder", err.Error())
		return
	}
	if err := r.apply(ctx, f, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading forwarder", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MailForwarderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state MailForwarderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteForwarder(state.MailDomainID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting forwarder", err.Error())
	}
}

// ImportState accepts "mail_domain_id/forwarder_id".
func (r *MailForwarderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected format: mail_domain_id/forwarder_id")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mail_domain_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
