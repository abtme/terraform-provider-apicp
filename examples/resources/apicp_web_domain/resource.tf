resource "apicp_web_domain" "example" {
  domain = "example.com"
}

# One vhost can answer to several names. Each alias serves the same files as the domain and is
# covered by the same certificate; every name must resolve to the node.
resource "apicp_web_domain" "with_www" {
  domain  = "example.org"
  aliases = ["www.example.org"]
}

resource "apicp_tls_certificate" "with_www" {
  vhost_id = apicp_web_domain.with_www.id
}

# A Node.js app instead of static files or PHP: deploy it into /home/<unix_user>/app over SSH
# (see apicp_ssh_access), build it, then touch ~/app/restart.txt. EmDash and other Astro sites
# work with the default start file, dist/server/entry.mjs.
resource "apicp_web_domain" "node_app" {
  domain       = "app.example.net"
  node_version = "24"
}
