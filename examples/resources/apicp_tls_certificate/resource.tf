resource "apicp_web_domain" "example" {
  domain = "example.com"
}

resource "apicp_tls_certificate" "example" {
  vhost_id = apicp_web_domain.example.id
}
