resource "apicp_firewall" "this" {}

# The office is on an ISP that renumbers it; b.example.org follows it (dynamic DNS).
resource "apicp_firewall_site" "office" {
  name = "Office"
  host = "b.example.org"
}

# Home reaches SSH and the API only.
resource "apicp_firewall_site" "home" {
  name  = "Home"
  host  = "v.example.org"
  ports = [22, 8080]
}

# SSH from the sites only: no address of its own. Until a site's name has
# resolved, the port stays closed rather than open.
resource "apicp_firewall_restriction" "ssh" {
  depends_on = [apicp_firewall.this, apicp_firewall_site.office, apicp_firewall_site.home]
  port       = 22
  sources    = []
}
