resource "apicp_web_domain" "example" {
  domain = "example.com"
}

resource "apicp_ssh_access" "example" {
  vhost_id   = apicp_web_domain.example.id
  public_key = "ssh-ed25519 AAAA... you@example"
}

# Several apicp_ssh_access resources can target the same vhost_id with
# different keys - each one grants/revokes only its own key, so both
# stay active on the vhost at once.
resource "apicp_ssh_access" "teammate" {
  vhost_id   = apicp_web_domain.example.id
  public_key = "ssh-ed25519 AAAA... teammate@example"
}
