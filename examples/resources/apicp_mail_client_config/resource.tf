resource "apicp_mail_tls" "this" {}

resource "apicp_mail_domain" "example" {
  name = "example.com"
}

resource "apicp_mail_client_config" "example" {
  mail_domain_id = apicp_mail_domain.example.id

  # apicp refuses client config until mail TLS is on.
  depends_on = [apicp_mail_tls.this]
}
