resource "apicp_mail_domain" "example" {
  name = "example.com"
}

resource "apicp_mailbox" "info" {
  mail_domain_id = apicp_mail_domain.example.id
  local_part     = "info"
}

output "info_mailbox_password" {
  value     = apicp_mailbox.info.password
  sensitive = true
}
