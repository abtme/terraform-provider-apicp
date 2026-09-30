resource "apicp_mail_domain" "example" {
  name = "example.com"
}

resource "apicp_mailbox" "info" {
  mail_domain_id = apicp_mail_domain.example.id
  local_part     = "info"
}

# sales@example.com goes to two outside addresses.
resource "apicp_mail_forwarder" "sales" {
  mail_domain_id = apicp_mail_domain.example.id
  local_part     = "sales"
  destinations   = ["jane@example.org", "bob@example.net"]
}

# Everything else in the domain without a mailbox or forwarder lands in info@.
# (A same-domain catch-all destination must be an existing mailbox.)
resource "apicp_mail_forwarder" "catch_all" {
  mail_domain_id = apicp_mail_domain.example.id
  local_part     = "*"
  destinations   = [apicp_mailbox.info.email]
}
