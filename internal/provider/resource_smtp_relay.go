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

// SMTPRelayResource is apicp_smtp_relay (PLAN.md §2 "SMTP smarthost /
// relay") - one per apicp node, same "takes a parent ID, POST/GET/DELETE
// a sub-path" shape as apicp_dkim/apicp_ssh_access. Unlike those, every
// input (host/port/username/password) is operator-supplied, not
// apicp-generated - so, unlike apicp_database's password, there's
// nothing to treat as "computed and returned once": Password is a
// normal Required+Sensitive input straight from config. apicp still
// never echoes the real password back out on GET (see client.SMTPRelay's
// doc comment), so Read deliberately never touches state's Password
// field, matching apicp_dkim/apicp_database's own "state stays whatever
// it already was" convention for write-only secrets - here that's just
// belt-and-braces, since Password being an input (not Computed) means
// Terraform already sources it from config on every plan regardless.
// username/password are optional, together: a relay that accepts mail
// by source address needs no login.
type SMTPRelayResource struct{ client *client.Client }

var _ resource.Resource = &SMTPRelayResource{}
var _ resource.ResourceWithImportState = &SMTPRelayResource{}
var _ resource.ResourceWithValidateConfig = &SMTPRelayResource{}

func NewSMTPRelayResource() resource.Resource { return &SMTPRelayResource{} }

type SMTPRelayResourceModel struct {
	ID       types.String `tfsdk:"id"`
	NodeID   types.String `tfsdk:"node_id"`
	Host     types.String `tfsdk:"host"`
	Port     types.Int64  `tfsdk:"port"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
	Status   types.String `tfsdk:"status"`
}

func (r *SMTPRelayResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_smtp_relay"
}

func (r *SMTPRelayResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Configures a node's outbound SMTP smarthost/relay (PLAN.md §2) - sending mail directly from a hosting node is a deliverability liability, so every node's Postfix should relay through a real smarthost instead. At most one per `node_id`; applying new values replaces the node's existing relay config in place (same node, same resource).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Same value as `node_id` - apicp has exactly one relay config per node, so the node's own ID is this resource's ID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"node_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the apicp node (see `GET /v1/nodes`) to configure the relay on. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"host": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Smarthost hostname or IP.",
			},
			"port": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Smarthost port (typically 587 or 465).",
			},
			"username": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "SASL auth username for the smarthost. Set `username` and `password` together, or omit both for a relay that accepts mail by source address without a login (an empty string is not accepted: omit the attribute). With a login apicp requires TLS to the relay; without one TLS is used when the relay offers it (port 465 is always TLS from the first byte).",
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "SASL auth password. Set together with `username`, or omit both. apicp never returns this back out (not even redacted-vs-set, no drift detection possible on this field) - it's write-only from Terraform's point of view.",
			},
			"status": schema.StringAttribute{Computed: true},
		},
	}
}

// ValidateConfig: username and password go together (a relay either has a login or it does not),
// and an empty string is not a way to say "none" - omit the attribute. Unknown values (not yet
// resolved from another resource) are left for apply, when apicp validates the request again.
func (r *SMTPRelayResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
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

func (r *SMTPRelayResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// apply copies every field GET/create actually return. Password is
// deliberately not touched here - see the resource's own doc comment.
func (r *SMTPRelayResource) apply(relay *client.SMTPRelay, m *SMTPRelayResourceModel) {
	m.ID = types.StringValue(relay.NodeID)
	m.NodeID = types.StringValue(relay.NodeID)
	m.Host = types.StringValue(relay.Host)
	m.Port = types.Int64Value(int64(relay.Port))
	// username is Optional, not Computed: with no login apicp returns "" and the state must stay
	// null, or Terraform reports "inconsistent result after apply" against the planned null.
	if relay.Username != "" {
		m.Username = types.StringValue(relay.Username)
	} else {
		m.Username = types.StringNull()
	}
	m.Status = types.StringValue(relay.Status)
}

func (r *SMTPRelayResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SMTPRelayResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	relay, err := r.client.EnableSMTPRelay(plan.NodeID.ValueString(), plan.Host.ValueString(), plan.Port.ValueInt64(), plan.Username.ValueString(), plan.Password.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error enabling smtp relay", err.Error())
		return
	}
	r.apply(relay, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SMTPRelayResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SMTPRelayResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	relay, err := r.client.GetSMTPRelay(state.NodeID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading smtp relay", err.Error())
		return
	}
	r.apply(relay, &state) // state.Password stays whatever it already was
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update - host/port/username/password all just re-apply via the same
// idempotent enable call (no RequiresReplace on any of them, unlike
// node_id) - a real in-place update, not a replace.
func (r *SMTPRelayResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SMTPRelayResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	relay, err := r.client.EnableSMTPRelay(plan.NodeID.ValueString(), plan.Host.ValueString(), plan.Port.ValueInt64(), plan.Username.ValueString(), plan.Password.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error updating smtp relay", err.Error())
		return
	}
	r.apply(relay, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SMTPRelayResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SMTPRelayResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DisableSMTPRelay(state.NodeID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error disabling smtp relay", err.Error())
	}
}

// ImportState accepts the node_id - same rationale as apicp_dkim's
// ImportState. Password can't be recovered on import (apicp never
// returns it) - the imported resource will show a password diff on the
// next plan until the config's real value is supplied, same limitation
// noted in the resource's own schema description.
func (r *SMTPRelayResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node_id"), req.ID)...)
}
