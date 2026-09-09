resource "apicp_firewall" "this" {}

# Allow one specific admin IP full SSH access even if a later rule
# below would otherwise restrict it - rules are evaluated in creation
# order, before the fixed baseline allow-list.
resource "apicp_firewall_rule" "admin_ssh" {
  depends_on = [apicp_firewall.this]
  source     = "203.0.113.50/32"
  port       = "22"
  protocol   = "tcp"
  action     = "accept"
  comment    = "office IP"
}

# Block a known-bad IP entirely.
resource "apicp_firewall_rule" "block_attacker" {
  depends_on = [apicp_firewall.this]
  source     = "198.51.100.23/32"
  protocol   = "all"
  action     = "drop"
  comment    = "repeated brute-force attempts"
}
