resource "apicp_web_domain" "example" {
  domain = "example.com"
}

resource "apicp_ssh_access" "example" {
  vhost_id   = apicp_web_domain.example.id
  public_key = "ssh-ed25519 AAAA... you@example"
}
