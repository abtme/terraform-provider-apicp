package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

var _ provider.Provider = &ApicpProvider{}

type ApicpProvider struct {
	version string
}

type ApicpProviderModel struct {
	URL   types.String `tfsdk:"url"`
	Token types.String `tfsdk:"token"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &ApicpProvider{version: version}
	}
}

func (p *ApicpProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "apicp"
	resp.Version = p.version
}

func (p *ApicpProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Terraform provider for apicp — manage web vhosts, TLS certificates, DNS zones/records, databases, and mail domains/mailboxes via apicp's own control-plane API.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				MarkdownDescription: "Base URL of the apicp control plane, e.g. `https://apicp.example.com`. May also be set via the `APICP_URL` environment variable.",
				Optional:            true,
			},
			"token": schema.StringAttribute{
				MarkdownDescription: "apicp bearer token. May also be set via the `APICP_TOKEN` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
		},
	}
}

func (p *ApicpProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config ApicpProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	url := os.Getenv("APICP_URL")
	if !config.URL.IsNull() && !config.URL.IsUnknown() {
		url = config.URL.ValueString()
	}
	if url == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("url"),
			"Missing apicp URL",
			"Set the url provider attribute or the APICP_URL environment variable.",
		)
	}

	token := os.Getenv("APICP_TOKEN")
	if !config.Token.IsNull() && !config.Token.IsUnknown() {
		token = config.Token.ValueString()
	}
	if token == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("token"),
			"Missing apicp token",
			"Set the token provider attribute or the APICP_TOKEN environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	c := client.New(url, token)
	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *ApicpProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewWebDomainResource,
		NewTLSCertificateResource,
		NewDNSZoneResource,
		NewDNSRecordResource,
		NewDatabaseResource,
		NewMailDomainResource,
		NewMailboxResource,
	}
}

func (p *ApicpProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}
