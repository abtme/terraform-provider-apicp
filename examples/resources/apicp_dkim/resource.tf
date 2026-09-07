resource "apicp_mail_domain" "example" {
  name = "example.com"
}

resource "apicp_dkim" "example" {
  mail_domain_id = apicp_mail_domain.example.id
}

output "dkim_dns_record" {
  value = {
    name  = apicp_dkim.example.dns_record_name
    value = apicp_dkim.example.dns_record_value
  }
}
