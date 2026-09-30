data "apicp_nodes" "all" {}

# First connected node that hosts mail.
locals {
  mail_node = [for n in data.apicp_nodes.all.nodes : n.id if contains(n.roles, "mail") && n.connected][0]
}
