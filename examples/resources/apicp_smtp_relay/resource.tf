# The node is the server apicp provisions mail on. On a single-server install (no agent) that is
# the server itself: its node id is APICP_NODE_ID in apicpd's environment, "node-1" by default (the
# `target_node` in apicpd's start-up log). With agents, find a node's id with `GET /v1/nodes`.
variable "mail_node_id" {
  type    = string
  default = "node-1"
}

# A smart host that needs a login: username and password together. apicp then requires TLS to the
# relay (STARTTLS on 587, TLS from the first byte on 465), because the password is sent on every
# connection.
resource "apicp_smtp_relay" "provider" {
  node_id  = var.mail_node_id
  host     = "smtp.example-relay.com"
  port     = 587
  username = "relay-user"
  password = var.smtp_relay_password
}

variable "smtp_relay_password" {
  type      = string
  sensitive = true
}

# Or a relay that accepts mail by source address (an internal relay, an ISP smart host): omit both
# username and password. There is no SASL; TLS is used when the relay offers it. One relay per node,
# so use one of the two resources, not both.
#
# resource "apicp_smtp_relay" "internal" {
#   node_id = var.mail_node_id
#   host    = "relay.internal.example"
#   port    = 25
# }
