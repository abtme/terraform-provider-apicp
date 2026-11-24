resource "apicp_firewall" "this" {}

# One set of addresses, kept once and used by name.
resource "apicp_firewall_list" "office" {
  name    = "office"
  sources = ["203.0.113.7", "198.51.100.0/24", "2001:db8:5::/48"]
  comment = "HQ and VPN"
}

# An accept rule for the list: only its addresses reach port 9090.
resource "apicp_firewall_rule" "office_app" {
  depends_on = [apicp_firewall.this]
  list       = apicp_firewall_list.office.name
  port       = "9090"
  protocol   = "tcp"
  action     = "accept"
}

# IMAPS from the list alone: no address of its own.
resource "apicp_firewall_restriction" "imaps" {
  depends_on = [apicp_firewall.this]
  port       = 993
  sources    = []
  lists      = [apicp_firewall_list.office.name]
}

# A built-in template needs no resource: OVHcloud's dedicated-server
# monitoring (ping and the SSH check, including from the server's own
# .250/.251), one rule for all of it.
resource "apicp_firewall_rule" "ovh_monitoring" {
  depends_on = [apicp_firewall.this]
  list       = "ovh-monitoring"
  protocol   = "all"
  action     = "accept"
  comment    = "OVHcloud monitoring"
}
