resource "apicp_webmail" "this" {}

output "webmail_url" {
  value = apicp_webmail.this.url
}
