package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

// mailSettingsID is the one id the singleton apicp_mail_settings has.
const mailSettingsID = "mail_settings"

// maxMessageSizeMBLimit is apicp's upper bound for max_message_size_mb (10 GB).
const maxMessageSizeMBLimit = 10240

// MailSettingsResource is apicp_mail_settings: the mail server's
// server-wide settings (GET/PUT /v1/mail/settings). A singleton like
// apicp_system_tls_config - there is always a value on the server, so
// destroying the resource puts apicp's default back rather than removing
// anything.
type MailSettingsResource struct{ client *client.Client }

var _ resource.Resource = &MailSettingsResource{}
var _ resource.ResourceWithImportState = &MailSettingsResource{}
var _ resource.ResourceWithValidateConfig = &MailSettingsResource{}

func NewMailSettingsResource() resource.Resource { return &MailSettingsResource{} }

type MailSettingsResourceModel struct {
	ID                      types.String `tfsdk:"id"`
	MaxMessageSizeMB        types.Int64  `tfsdk:"max_message_size_mb"`
	DefaultMaxMessageSizeMB types.Int64  `tfsdk:"default_max_message_size_mb"`
}

func (r *MailSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mail_settings"
}

func (r *MailSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The mail server's server-wide settings on apicp's control-plane node. Requires an admin-tier provider token. One per server: declare it once. Destroying it puts apicp's default back (`default_max_message_size_mb`); it never leaves the server without a value. Needs apicp after v0.6.18.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Always `mail_settings`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"max_message_size_mb": schema.Int64Attribute{
				Required: true,
				MarkdownDescription: fmt.Sprintf("Largest message the mail server accepts, incoming and outgoing (Postfix `message_size_limit`), in MB (1024 × 1024 bytes). `0` = unlimited, otherwise 1 to %d. "+
					"It is the whole message as sent: attachments grow by about a third when encoded, so 50 MB lets through attachments up to roughly 36 MB. A larger message is refused, so its sender gets a bounce.", maxMessageSizeMBLimit),
			},
			"default_max_message_size_mb": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "apicp's own default - what the server has until this is set, and what destroying this resource goes back to.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *MailSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *MailSettingsResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var size types.Int64
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("max_message_size_mb"), &size)...)
	if resp.Diagnostics.HasError() || size.IsNull() || size.IsUnknown() {
		return
	}
	if v := size.ValueInt64(); v < 0 || v > maxMessageSizeMBLimit {
		resp.Diagnostics.AddAttributeError(path.Root("max_message_size_mb"), "Invalid message size",
			fmt.Sprintf("max_message_size_mb must be 0 (unlimited) or 1 to %d, got %d.", maxMessageSizeMBLimit, v))
	}
}

func (r *MailSettingsResource) apply(st *client.MailSettings, m *MailSettingsResourceModel) {
	m.ID = types.StringValue(mailSettingsID)
	m.MaxMessageSizeMB = types.Int64Value(st.MaxMessageSizeMB)
	m.DefaultMaxMessageSizeMB = types.Int64Value(st.DefaultMaxMessageSizeMB)
}

// set PUTs the planned settings and copies apicp's answer into plan.
func (r *MailSettingsResource) set(plan *MailSettingsResourceModel) error {
	st, err := r.client.SetMailSettings(plan.MaxMessageSizeMB.ValueInt64())
	if err != nil {
		return err
	}
	r.apply(st, plan)
	return nil
}

func (r *MailSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan MailSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.set(&plan); err != nil {
		resp.Diagnostics.AddError("Error setting mail settings", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read never drops the resource from state: the settings always exist on a
// server that has the endpoint, and a 404 means an apicpd too old to have
// them - an error to fix, not something to re-create.
func (r *MailSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state MailSettingsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	st, err := r.client.GetMailSettings()
	if err != nil {
		resp.Diagnostics.AddError("Error reading mail settings", err.Error())
		return
	}
	r.apply(st, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *MailSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan MailSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.set(&plan); err != nil {
		resp.Diagnostics.AddError("Error updating mail settings", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete puts apicp's default back - read from the server, so the
// provider never carries its own copy of what the default is.
func (r *MailSettingsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	st, err := r.client.GetMailSettings()
	if err != nil {
		resp.Diagnostics.AddError("Error resetting mail settings", err.Error())
		return
	}
	if _, err := r.client.SetMailSettings(st.DefaultMaxMessageSizeMB); err != nil {
		resp.Diagnostics.AddError("Error resetting mail settings", err.Error())
	}
}

// ImportState accepts any id: there is one set of mail settings per server.
func (r *MailSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), mailSettingsID)...)
}
