resource "apicp_firewall" "this" {}

# Only the office and the VPN can reach the apicp API. The address Terraform
# itself connects from must be included, or apicp refuses (it would lock you
# out); IPv4 and IPv6 can be mixed.
resource "apicp_firewall_restriction" "api" {
  depends_on = [apicp_firewall.this]
  port       = 8080
  sources    = ["203.0.113.0/24", "2001:db8:1::/48"]
  comment    = "office + VPN"
}

# SSH from the office only. If the machine running Terraform is not the one
# that uses SSH, force acknowledges the restriction excludes it.
resource "apicp_firewall_restriction" "ssh" {
  depends_on = [apicp_firewall.this]
  port       = 22
  sources    = ["203.0.113.0/24"]
  force      = true
}

# Plain POP3 only from a trusted network.
resource "apicp_firewall_restriction" "pop3" {
  depends_on = [apicp_firewall.this]
  port       = 110
  sources    = ["198.51.100.0/24"]
}
