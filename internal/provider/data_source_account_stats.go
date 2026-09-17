package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

// AccountStatsDataSource is apicp_account_stats - the one apicp
// resource-group in this provider's own gap audit that's read-only in
// the API itself (GET /v1/accounts/{id}/stats, no create/update/delete
// exists at all), so it's a data source rather than a resource - this
// provider's first one.
type AccountStatsDataSource struct{ client *client.Client }

var _ datasource.DataSource = &AccountStatsDataSource{}

func NewAccountStatsDataSource() datasource.DataSource { return &AccountStatsDataSource{} }

type AccountStatsDataSourceModel struct {
	AccountID   types.String `tfsdk:"account_id"`
	WebDomains  types.Int64  `tfsdk:"web_domains"`
	Databases   types.Int64  `tfsdk:"databases"`
	MailDomains types.Int64  `tfsdk:"mail_domains"`
	Mailboxes   types.Int64  `tfsdk:"mailboxes"`
	CronJobs    types.Int64  `tfsdk:"cron_jobs"`
	SubAccounts types.Int64  `tfsdk:"sub_accounts"`
}

func (d *AccountStatsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_stats"
}

func (d *AccountStatsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads resource counts for an `apicp_account` (web domains, databases, mail domains/mailboxes, cron jobs, sub-accounts) - a plain tally, nothing configurable here, hence a data source rather than a resource.",
		Attributes: map[string]schema.Attribute{
			"account_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the `apicp_account` to read stats for.",
			},
			"web_domains":  schema.Int64Attribute{Computed: true},
			"databases":    schema.Int64Attribute{Computed: true},
			"mail_domains": schema.Int64Attribute{Computed: true},
			"mailboxes":    schema.Int64Attribute{Computed: true},
			"cron_jobs":    schema.Int64Attribute{Computed: true},
			"sub_accounts": schema.Int64Attribute{Computed: true},
		},
	}
}

func (d *AccountStatsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *AccountStatsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m AccountStatsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}

	s, err := d.client.GetAccountStats(m.AccountID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading account stats", err.Error())
		return
	}
	m.WebDomains = types.Int64Value(int64(s.WebDomains))
	m.Databases = types.Int64Value(int64(s.Databases))
	m.MailDomains = types.Int64Value(int64(s.MailDomains))
	m.Mailboxes = types.Int64Value(int64(s.Mailboxes))
	m.CronJobs = types.Int64Value(int64(s.CronJobs))
	m.SubAccounts = types.Int64Value(int64(s.SubAccounts))
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
