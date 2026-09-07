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

// DKIMResource is apicp_dkim (PLAN.md §8 phase 6) - per apicp_mail_domain,
// unlike apicp_antispam/apicp_antivirus's node-wide toggle shape,
// confirmed with the user before this was built: a DKIM keypair and
// selector genuinely belong to one domain. Same "takes an ID, POST/GET/
// DELETE a sub-path" shape as apicp_ssh_access.
type DKIMResource struct{ client *client.Client }

var _ resource.Resource = &DKIMResource{}
var _ resource.ResourceWithImportState = &DKIMResource{}

func NewDKIMResource() resource.Resource { return &DKIMResource{} }

type DKIMResourceModel struct {
	ID             types.String `tfsdk:"id"`
	MailDomainID   types.String `tfsdk:"mail_domain_id"`
	Selector       types.String `tfsdk:"selector"`
	DNSRecordName  types.String `tfsdk:"dns_record_name"`
	DNSRecordValue types.String `tfsdk:"dns_record_value"`
	DNSAutoCreated types.Bool   `tfsdk:"dns_auto_created"`
}

func (r *DKIMResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dkim"
}

func (r *DKIMResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Enables DKIM signing for an `apicp_mail_domain` (PLAN.md §8 phase 6) - apicp generates the keypair, installs it on the mail node, and auto-creates the DNS TXT record when apicp itself manages a matching `apicp_dns_zone` for the same account. If no matching zone exists, add `dns_record_name`/`dns_record_value` as a TXT record with your own DNS provider.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mail_domain_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the `apicp_mail_domain` to enable DKIM for. Changing this replaces the resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"selector": schema.StringAttribute{Computed: true},
			"dns_record_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "TXT record name, relative to the zone apex (e.g. `apicp._domainkey`).",
			},
			"dns_record_value": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "TXT record content (`v=DKIM1; k=rsa; p=...`).",
			},
			"dns_auto_created": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether apicp created the DNS record itself in a matching `apicp_dns_zone`, versus this being informational only.",
			},
		},
	}
}

func (r *DKIMResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DKIMResource) apply(d *client.DKIM, m *DKIMResourceModel) {
	m.ID = types.StringValue(d.MailDomainID)
	m.MailDomainID = types.StringValue(d.MailDomainID)
	m.Selector = types.StringValue(d.Selector)
	m.DNSRecordName = types.StringValue(d.DNSRecordName)
	m.DNSRecordValue = types.StringValue(d.DNSRecordValue)
	m.DNSAutoCreated = types.BoolValue(d.DNSAutoCreated)
}

func (r *DKIMResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DKIMResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.EnableDKIM(plan.MailDomainID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error enabling dkim", err.Error())
		return
	}
	r.apply(d, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DKIMResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DKIMResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.GetDKIM(state.MailDomainID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading dkim", err.Error())
		return
	}
	r.apply(d, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update — every attribute besides mail_domain_id (which forces
// replacement) is Computed; implemented to satisfy resource.Resource.
func (r *DKIMResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DKIMResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DKIMResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DKIMResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DisableDKIM(state.MailDomainID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error disabling dkim", err.Error())
	}
}

// ImportState accepts the mail_domain_id — same rationale as
// apicp_ssh_access's ImportState.
func (r *DKIMResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mail_domain_id"), req.ID)...)
}
