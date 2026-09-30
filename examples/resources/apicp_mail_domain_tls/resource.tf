resource "apicp_mail_tls" "this" {}

resource "apicp_mail_domain" "example" {
  name = "example.com"
}

# mail.example.com must already resolve to the node (A record) and port 80
# reach it; the certificate is a real ACME order.
resource "apicp_mail_domain_tls" "example" {
  mail_domain_id = apicp_mail_domain.example.id

  # apicp refuses per-domain certificates until node mail TLS is on.
  depends_on = [apicp_mail_tls.this]
}

# Client config after the certificate, so the records name mail.example.com.
resource "apicp_mail_client_config" "example" {
  mail_domain_id = apicp_mail_domain.example.id
  depends_on     = [apicp_mail_domain_tls.example]
}
