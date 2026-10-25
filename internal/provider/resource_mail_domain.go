package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

var _ resource.Resource = &MailDomainResource{}
var _ resource.ResourceWithImportState = &MailDomainResource{}
var _ resource.ResourceWithValidateConfig = &MailDomainResource{}

// compressionLevels are the levels apicp accepts per compression method
// (0 = the method's default; off and lz4 take none).
var compressionLevels = map[string][2]int64{"off": {0, 0}, "zstd": {1, 22}, "lz4": {0, 0}, "gz": {1, 9}, "bz2": {1, 9}}

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
	Compression      types.String `tfsdk:"compression"`
	CompressionLevel types.Int64  `tfsdk:"compression_level"`
	CompressedMail   types.Bool   `tfsdk:"compressed_mail"`
	Status           types.String `tfsdk:"status"`
}

func (m *MailDomainResourceModel) protocols() client.Protocols {
	return client.Protocols{DisableIMAP: m.DisableIMAP.ValueBool(), DisablePOP3: m.DisablePOP3.ValueBool()}
}

func (m *MailDomainResourceModel) compression() *client.Compression {
	return &client.Compression{Method: m.Compression.ValueString(), Level: int(m.CompressionLevel.ValueInt64())}
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
				MarkdownDescription: "ID of the relay (an `apicp_mail_relay`, or the default relay's ID) this domain's outgoing mail goes through: mail whose envelope sender is in this domain. `\"\"` = the server's default relay (bounces and every other sender always use that one). The relay must have no login and not be on port 465 (all relays share Postfix's one TLS setting), unless it is the default relay itself. Leave it out to leave the relay as it is on the server (the panel and `apicp maildomain relay` set it too) - it then shows the server's value. Only an admin account's token may set it. Needs apicp v0.8.0 or later.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"compression": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("off"),
				MarkdownDescription: "How new mail of this domain is stored: `off` (default), `zstd`, `lz4`, `gz` or `bz2` (Dovecot's mail_compress plugin; mail programs notice nothing). " +
					"`zstd` (level 3) is the right choice almost always: as small as gzip, about ten times faster. `lz4` uses the least CPU and saves less; `gz` for compatibility; high zstd levels only for archive mailboxes; `bz2` has no advantage. " +
					"Only methods the server's Dovecot can write are accepted (`apicp_mail_settings.compression_methods`). Mail already stored is not changed: compress or decompress it with `apicp mail compress-existing` / `decompress-existing` (a one-off job, deliberately not a Terraform resource); turning compression off keeps compressed mail readable. " +
					"Cannot change while such a job of the domain runs. Needs apicp v0.8.0.",
			},
			"compression_level": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				MarkdownDescription: "The method's level: `zstd` 1-22, `gz` 1-9, `bz2` 1-9 (block size); `lz4` and `off` take none. `0` (default) = the method's own default (zstd 3, gz 6, bz2 9).",
			},
			"compressed_mail": schema.BoolAttribute{
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
				MarkdownDescription: "Some of the domain's stored mail may be compressed (compression was on, or compress-existing ran, since the last whole-domain decompress-existing that left nothing compressed). apicp backups store such a domain's mail decompressed.",
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

// ValidateConfig checks compression and compression_level against what apicp
// accepts, before anything is sent.
func (r *MailDomainResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg MailDomainResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || cfg.Compression.IsUnknown() || cfg.CompressionLevel.IsUnknown() {
		return
	}
	method := "off"
	if !cfg.Compression.IsNull() {
		method = cfg.Compression.ValueString()
	}
	rng, ok := compressionLevels[method]
	if !ok {
		resp.Diagnostics.AddAttributeError(path.Root("compression"), "Unknown compression",
			fmt.Sprintf("%q: want off, zstd, lz4, gz or bz2 (xz is not offered: Dovecot 2.4 can no longer write it)", method))
		return
	}
	if cfg.CompressionLevel.IsNull() || cfg.CompressionLevel.ValueInt64() == 0 {
		return
	}
	level := cfg.CompressionLevel.ValueInt64()
	if rng[1] == 0 {
		resp.Diagnostics.AddAttributeError(path.Root("compression_level"), "No level for this method", fmt.Sprintf("%s takes no level: leave compression_level at 0", method))
	} else if level < rng[0] || level > rng[1] {
		resp.Diagnostics.AddAttributeError(path.Root("compression_level"), "Level out of range", fmt.Sprintf("%s takes %d to %d (0 = its default)", method, rng[0], rng[1]))
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
	m.MaxEmailsPerHour = types.Int64Value(int64(d.MaxEmailsPerHour))
	m.RelayID = types.StringValue(d.RelayID)
	m.Compression = types.StringValue(d.CompressionMethod())
	m.CompressionLevel = types.Int64Value(int64(d.CompressionLevel))
	m.CompressedMail = types.BoolValue(d.CompressedMail)
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
	d, err := r.client.CreateMailDomain(plan.Name.ValueString(), plan.protocols(), int(plan.MaxEmailsPerHour.ValueInt64()), relay, plan.compression())
	if errors.Is(err, client.ErrCompressionUnsupported) {
		// Created without it: kept in state (as it is) so it is not created twice.
		r.applyDomain(d, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		resp.Diagnostics.AddAttributeError(path.Root("compression"), "Compression not supported", err.Error())
		if detail, old := relayTooOld(relay, d); old {
			resp.Diagnostics.AddAttributeError(path.Root("relay_id"), "relay_id not supported", detail)
		}
		return
	}
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
// sending limit, the relay and the compression change here. Compression is
// sent only when it changes: apicp refuses a compression change (409) while a
// compress- or decompress-existing job of the domain runs, and other changes
// must still go through then.
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
	var comp *client.Compression
	if !plan.Compression.Equal(state.Compression) || !plan.CompressionLevel.Equal(state.CompressionLevel) {
		comp = plan.compression()
	}
	d, err := r.client.UpdateMailDomain(state.ID.ValueString(), plan.protocols(), int(plan.MaxEmailsPerHour.ValueInt64()), relay, comp)
	if errors.Is(err, client.ErrCompressionUnsupported) {
		resp.Diagnostics.AddAttributeError(path.Root("compression"), "Compression not supported", err.Error())
		return
	}
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
