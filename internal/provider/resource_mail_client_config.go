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

// MailClientConfigResource is apicp_mail_client_config - per
// apicp_mail_domain, same shape as apicp_dkim.
type MailClientConfigResource struct{ client *client.Client }

var _ resource.Resource = &MailClientConfigResource{}
var _ resource.ResourceWithImportState = &MailClientConfigResource{}

func NewMailClientConfigResource() resource.Resource { return &MailClientConfigResource{} }

type mailClientConfigRecordModel struct {
	Name    types.String `tfsdk:"name"`
	Type    types.String `tfsdk:"type"`
	Content types.String `tfsdk:"content"`
	Action  types.String `tfsdk:"action"`
}

type MailClientConfigResourceModel struct {
	ID            types.String                  `tfsdk:"id"`
	MailDomainID  types.String                  `tfsdk:"mail_domain_id"`
	Host          types.String                  `tfsdk:"host"`
	AutoconfigURL types.String                  `tfsdk:"autoconfig_url"`
	Records       []mailClientConfigRecordModel `tfsdk:"records"`
}

func (r *MailClientConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mail_client_config"
}

func (r *MailClientConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Publishes mail-client autoconfiguration for an `apicp_mail_domain`, so Thunderbird, Outlook and other clients find the right server settings from just an address: SRV, MX, SPF and DMARC records (created in the account's own `apicp_dns_zone` for the domain when apicp hosts one) and the Thunderbird and Outlook autoconfig documents on the node. Requires `apicp_mail_tls` to be enabled first (apicp refuses otherwise, since the advertised ports only exist with TLS) - use `depends_on`. Records that already exist are left alone, and destroying this resource removes only the records apicp created. Where there is no matching zone, add the `records` marked `manual` with your own DNS provider.",
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
				MarkdownDescription: "Mail server hostname the records point clients at.",
			},
			"autoconfig_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Where clients fetch the Thunderbird autoconfig document.",
			},
			"records": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The discovery records and what apicp did with each: `created` (apicp published it), `exists` (something was already there, left alone) or `manual` (apicp hosts no matching zone - add it yourself).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":    schema.StringAttribute{Computed: true, MarkdownDescription: "Record name relative to the domain (`@` = apex)."},
						"type":    schema.StringAttribute{Computed: true},
						"content": schema.StringAttribute{Computed: true},
						"action":  schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (r *MailClientConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *MailClientConfigResource) apply(cc *client.ClientConfig, m *MailClientConfigResourceModel) {
	m.ID = types.StringValue(cc.MailDomainID)
	m.MailDomainID = types.StringValue(cc.MailDomainID)
	m.Host = types.StringValue(cc.Host)
	m.AutoconfigURL = types.StringValue(cc.AutoconfigURL)
	m.Records = make([]mailClientConfigRecordModel, 0, len(cc.Records))
	for _, rec := range cc.Records {
		m.Records = append(m.Records, mailClientConfigRecordModel{
			Name:    types.StringValue(rec.Name),
			Type:    types.StringValue(rec.Type),
			Content: types.StringValue(rec.Content),
			Action:  types.StringValue(rec.Action),
		})
	}
}

func (r *MailClientConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan MailClientConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cc, err := r.client.EnableClientConfig(plan.MailDomainID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error enabling mail client config", err.Error())
		return
	}
	r.apply(cc, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MailClientConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state MailClientConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cc, err := r.client.GetClientConfig(state.MailDomainID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading mail client config", err.Error())
		return
	}
	r.apply(cc, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update - everything but mail_domain_id is Computed (and that replaces);
// implemented to satisfy the interface.
func (r *MailClientConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan MailClientConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *MailClientConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state MailClientConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DisableClientConfig(state.MailDomainID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Error disabling mail client config", err.Error())
	}
}

// ImportState accepts the mail_domain_id.
func (r *MailClientConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mail_domain_id"), req.ID)...)
}
