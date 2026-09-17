# Node IDs come from enrolling an apicp-agent on that node (there's no
# apicp_node resource - nodes are registered out-of-band via the agent's
# own enroll flow, not created by Terraform). Find one with
# `GET /v1/nodes` or the apicp CLI/desktop app.
variable "web_node_id" {
  type = string
}

resource "apicp_smtp_relay" "example" {
  node_id  = var.web_node_id
  host     = "smtp.example-relay.com"
  port     = 587
  username = "relay-user"
  password = var.smtp_relay_password
}

variable "smtp_relay_password" {
  type      = string
  sensitive = true
}
