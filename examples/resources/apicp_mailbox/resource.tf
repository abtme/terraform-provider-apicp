resource "apicp_mail_domain" "example" {
  name = "example.com"
}

resource "apicp_mailbox" "info" {
  mail_domain_id = apicp_mail_domain.example.id
  local_part     = "info"
}

# A mailbox that may send at most 200 recipients an hour with its login (0 = unlimited, the default).
resource "apicp_mailbox" "newsletter" {
  mail_domain_id      = apicp_mail_domain.example.id
  local_part          = "newsletter"
  max_emails_per_hour = 200
}

output "info_mailbox_password" {
  value     = apicp_mailbox.info.password
  sensitive = true
}
