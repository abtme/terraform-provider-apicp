package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
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
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	UnixUser         types.String `tfsdk:"unix_user"`
	DisableIMAP      types.Bool   `tfsdk:"disable_imap"`
	DisablePOP3      types.Bool   `tfsdk:"disable_pop3"`
	MaxEmailsPerHour types.Int64  `tfsdk:"max_emails_per_hour"`
	RelayID          types.String `tfsdk:"relay_id"`
	Status           types.String `tfsdk:"status"`
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
			"max_emails_per_hour": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				MarkdownDescription: "Outgoing mail (recipients) from this domain's mailboxes and its website over a rolling hour, on top of the account's package `max_emails_per_hour`. Mail over it is refused for now (451): a website's mail stays queued and goes out later by itself, a mail program keeps it in its Outbox to send again. `0` (default) = unlimited.",
			},
			"relay_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "ID of the relay (an `apicp_mail_relay`, or the default relay's ID) this domain's outgoing mail goes through: mail whose envelope sender is in this domain. `\"\"` = the server's default relay (bounces and every other sender always use that one). Leave it out to leave the relay as it is on the server (the panel and `apicp maildomain relay` set it too) - it then shows the server's value. Only an admin account's token may set it. Needs apicp v0.8.0 or later.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status": schema.StringAttribute{Computed: true},
		},
	}
}

// wantRelay is the relay_id the plan asks for, or nil when it leaves the relay as it is.
func (m *MailDomainResourceModel) wantRelay() *string {
	if m.RelayID.IsNull() || m.RelayID.IsUnknown() {
		return nil
	}
	v := m.RelayID.ValueString()
	return &v
}

// relayTooOld is the error when apicpd took the request but did not set the
// relay: apicpd before v0.8.0 ignores relay_id.
func relayTooOld(want string, d *client.MailDomain) (string, bool) {
	if d.RelayID == want {
		return "", false
	}
	return fmt.Sprintf("apicpd did not set relay_id %q (it reports %q): relays per mail domain need apicp v0.8.0 or later. "+
		"Upgrade apicpd, or remove relay_id from the configuration.", want, d.RelayID), true
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
	m.MaxEmailsPerHour = types.Int64Value(int64(d.MaxEmailsPerHour))
	m.RelayID = types.StringValue(d.RelayID)
	m.Status = types.StringValue(d.Status)
}

func (r *MailDomainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan MailDomainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	relay := ""
	if want := plan.wantRelay(); want != nil {
		relay = *want
	}
	d, err := r.client.CreateMailDomain(plan.Name.ValueString(), plan.protocols(), int(plan.MaxEmailsPerHour.ValueInt64()), relay)
	if err != nil {
		resp.Diagnostics.AddError("Error creating mail domain", errorDetail(err))
		return
	}
	r.applyDomain(d, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	// The domain exists (in state, so Terraform marks it tainted and replaces it next time).
	if detail, old := relayTooOld(relay, d); old {
		resp.Diagnostics.AddAttributeError(path.Root("relay_id"), "relay_id not supported", detail)
	}
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

// Update - name requires replace, so only the protocol switches, the
// sending limit and the relay change here.
func (r *MailDomainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state MailDomainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// relay_id is sent only when it changes, so other changes work on an apicpd before v0.8.0.
	relay := plan.wantRelay()
	if relay != nil && *relay == state.RelayID.ValueString() {
		relay = nil
	}
	d, err := r.client.UpdateMailDomain(state.ID.ValueString(), plan.protocols(), int(plan.MaxEmailsPerHour.ValueInt64()), relay)
	if err != nil {
		resp.Diagnostics.AddError("Error updating mail domain", errorDetail(err))
		return
	}
	if relay != nil {
		if detail, old := relayTooOld(*relay, d); old {
			resp.Diagnostics.AddAttributeError(path.Root("relay_id"), "relay_id not supported", detail)
			return
		}
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
