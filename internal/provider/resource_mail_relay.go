package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

// MailRelayResource is apicp_mail_relay: one named outbound relay of a node
// (apicp v0.8.0, /v1/nodes/{id}/smtp-relays). A node can have several; the
// default one carries all outgoing mail, and an apicp_mail_domain's relay_id
// sends that domain's mail through another. Same write-only password
// treatment as apicp_smtp_relay: apicp only returns it in the create
// response, so Read never touches state's password (it comes from config).
type MailRelayResource struct{ client *client.Client }

var _ resource.Resource = &MailRelayResource{}
var _ resource.ResourceWithImportState = &MailRelayResource{}
var _ resource.ResourceWithValidateConfig = &MailRelayResource{}

func NewMailRelayResource() resource.Resource { return &MailRelayResource{} }

type MailRelayResourceModel struct {
	ID       types.String `tfsdk:"id"`
	NodeID   types.String `tfsdk:"node_id"`
	Name     types.String `tfsdk:"name"`
	Host     types.String `tfsdk:"host"`
	Port     types.Int64  `tfsdk:"port"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
	Default  types.Bool   `tfsdk:"default"`
	Status   types.String `tfsdk:"status"`
}

func (r *MailRelayResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mail_relay"
}

func (r *MailRelayResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "One outbound relay (smart host) of an apicp node. A node can have several (apicp v0.8.0 or later): the `default` one carries all outgoing mail, and a mail domain whose `relay_id` names another relay sends through that one instead (mail whose envelope sender is in the domain; bounces and every other sender use the default). " +
			"With a login apicp requires TLS to the relay; without one TLS is used when the relay offers it; port 465 is TLS from the first byte. " +
			"`apicp_smtp_relay` manages the node's default relay too: use one or the other for the default relay of a node, not both. apicp refuses to delete a relay while mail domains use it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The relay's ID (`relay_...`) - what `apicp_mail_domain.relay_id` takes.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"node_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the apicp node (see `GET /v1/nodes`; the server's own node on a single-server install, e.g. `node-1`). Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name, unique among the node's relays: 1-63 lower-case letters, digits, `.`, `-` or `_`.",
			},
			"host": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The relay's host name or IPv4 address.",
			},
			"port": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The relay's port (typically 587, or 465 for TLS from the first byte).",
			},
			"username": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "SASL login for the relay. Set `username` and `password` together, or omit both for a relay that accepts mail by the server's address (an empty string is not accepted: omit the attribute).",
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "SASL password. Set together with `username`, or omit both. apicp never returns it (write-only: no drift detection), and the provider sends it again with every change, since apicp only sends a stored password to the host it was given for.",
			},
			"default": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Make this the node's default relay (the one that was default stops being it). Default `false`: a relay that is not the default is only used by the mail domains that name it. Set it on at most one relay of a node.",
			},
			"status": schema.StringAttribute{Computed: true},
		},
	}
}

// ValidateConfig: the same login rules as apicp_smtp_relay.
func (r *MailRelayResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var username, password types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("username"), &username)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("password"), &password)...)
	if resp.Diagnostics.HasError() || username.IsUnknown() || password.IsUnknown() {
		return
	}
	for name, v := range map[string]types.String{"username": username, "password": password} {
		if !v.IsNull() && v.ValueString() == "" {
			resp.Diagnostics.AddAttributeError(path.Root(name), "Empty "+name,
				"Omit the attribute for a relay that needs no login; an empty string is not accepted.")
		}
	}
	if username.IsNull() != password.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("password"), "username and password go together",
			"Set both for a relay that needs a login, or omit both for a relay that accepts mail without one.")
	}
}

func (r *MailRelayResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// apply copies what apicp returns; the password is left as it is (config's).
func (r *MailRelayResource) apply(relay *client.SMTPRelay, m *MailRelayResourceModel) {
	m.ID = types.StringValue(relay.ID)
	m.NodeID = types.StringValue(relay.NodeID)
	m.Name = types.StringValue(relay.Name)
	m.Host = types.StringValue(relay.Host)
	m.Port = types.Int64Value(int64(relay.Port))
	// username is Optional, not Computed: no login reads back as "" and must stay null.
	if relay.Username != "" {
		m.Username = types.StringValue(relay.Username)
	} else {
		m.Username = types.StringNull()
	}
	m.Default = types.BoolValue(relay.Default)
	m.Status = types.StringValue(relay.Status)
}

// relayError explains a 404/405 from an apicpd without named relays.
func relayError(err error) string {
	var ae *client.APIError
	if errors.Is(err, client.ErrNotFound) || (errors.As(err, &ae) && ae.StatusCode == 405) {
		return "This apicpd has no named relays (/v1/nodes/{id}/smtp-relays): they need apicp v0.8.0 or later, or the node does not exist. (" + err.Error() + ")"
	}
	return errorDetail(err)
}

func (r *MailRelayResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan MailRelayResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	relay, err := r.client.CreateRelay(plan.NodeID.ValueString(), client.SMTPRelayCreate{
		Name: plan.Name.ValueString(), Host: plan.Host.ValueString(), Port: plan.Port.ValueInt64(),
		Username: plan.Username.ValueString(), Password: plan.Password.ValueString(), Default: plan.Default.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating mail relay", relayError(err))
		return
	}
	r.apply(relay, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MailRelayResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state MailRelayResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	relay, err := r.client.GetRelay(state.NodeID.ValueString(), state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading mail relay", err.Error())
		return
	}
	r.apply(relay, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update sends every argument: the password has to travel with a new host or
// port anyway, and with it apicp's checks see the whole relay.
func (r *MailRelayResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state MailRelayResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name, host, port := plan.Name.ValueString(), plan.Host.ValueString(), plan.Port.ValueInt64()
	username, password, def := plan.Username.ValueString(), plan.Password.ValueString(), plan.Default.ValueBool()
	patch := client.SMTPRelayPatch{Name: &name, Host: &host, Port: &port, Username: &username, Default: &def}
	if username != "" {
		patch.Password = &password
	}
	relay, err := r.client.UpdateRelay(state.NodeID.ValueString(), state.ID.ValueString(), patch)
	if err != nil {
		resp.Diagnostics.AddError("Error updating mail relay", relayError(err))
		return
	}
	r.apply(relay, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MailRelayResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state MailRelayResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteRelay(state.NodeID.ValueString(), state.ID.ValueString())
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Error deleting mail relay", err.Error())
	}
}

// ImportState takes "node_id/relay_id". The password cannot be read back, so
// the next plan shows it until the configuration's value is applied.
func (r *MailRelayResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, id, ok := strings.Cut(req.ID, "/")
	if !ok || node == "" || id == "" {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("want node_id/relay_id (e.g. node-1/relay_0123456789ab), got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node_id"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
