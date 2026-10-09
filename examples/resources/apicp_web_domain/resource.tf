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
