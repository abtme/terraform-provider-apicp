package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

// NodesDataSource is apicp_nodes: the enrolled nodes. Nodes are enrolled by
// the installer on the node itself (with a one-time token), never created
// through the API, so this is a data source rather than a resource.
type NodesDataSource struct{ client *client.Client }

var _ datasource.DataSource = &NodesDataSource{}

func NewNodesDataSource() datasource.DataSource { return &NodesDataSource{} }

type nodeModel struct {
	ID           types.String `tfsdk:"id"`
	Hostname     types.String `tfsdk:"hostname"`
	PublicHost   types.String `tfsdk:"public_host"`
	Roles        types.Set    `tfsdk:"roles"`
	Status       types.String `tfsdk:"status"`
	Connected    types.Bool   `tfsdk:"connected"`
	AgentVersion types.String `tfsdk:"agent_version"`
	LastSeenAt   types.String `tfsdk:"last_seen_at"`
	EnrolledAt   types.String `tfsdk:"enrolled_at"`
}

type NodesDataSourceModel struct {
	ID    types.String `tfsdk:"id"`
	Nodes []nodeModel  `tfsdk:"nodes"`
}

func (d *NodesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nodes"
}

func (d *NodesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	str := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, MarkdownDescription: desc}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the nodes enrolled with apicp. Nodes are enrolled by the installer on the node itself, not created through the API, so this is read-only. Use it to pick a `node_id` for `apicp_web_domain` / `apicp_mail_domain`, e.g. `[for n in data.apicp_nodes.all.nodes : n.id if contains(n.roles, \"web\") && n.connected][0]`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, MarkdownDescription: "Always `nodes`."},
			"nodes": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":            str("Node ID."),
						"hostname":      str("Node hostname."),
						"public_host":   str("Public hostname, if set at enrolment."),
						"roles":         schema.SetAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "What the node hosts (e.g. `web`, `mail`, `dns`, `db`)."},
						"status":        str("Enrolment status."),
						"connected":     schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the node's agent is connected right now."},
						"agent_version": str("Version reported by the node's agent."),
						"last_seen_at":  str("When the node was last seen (RFC3339)."),
						"enrolled_at":   str("When the node enrolled (RFC3339)."),
					},
				},
			},
		},
	}
}

func (d *NodesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *NodesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	nodes, err := d.client.ListNodes()
	if err != nil {
		resp.Diagnostics.AddError("Error listing nodes", err.Error())
		return
	}
	m := NodesDataSourceModel{ID: types.StringValue("nodes"), Nodes: make([]nodeModel, 0, len(nodes))}
	for _, n := range nodes {
		roles, diags := types.SetValueFrom(ctx, types.StringType, n.Roles)
		resp.Diagnostics.Append(diags...)
		m.Nodes = append(m.Nodes, nodeModel{
			ID:           types.StringValue(n.ID),
			Hostname:     types.StringValue(n.Hostname),
			PublicHost:   types.StringValue(n.PublicHost),
			Roles:        roles,
			Status:       types.StringValue(n.Status),
			Connected:    types.BoolValue(n.Connected),
			AgentVersion: types.StringValue(n.AgentVersion),
			LastSeenAt:   types.StringValue(n.LastSeenAt),
			EnrolledAt:   types.StringValue(n.EnrolledAt),
		})
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
