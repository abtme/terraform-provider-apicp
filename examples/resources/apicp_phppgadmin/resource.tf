resource "apicp_phppgadmin" "this" {}

output "phppgadmin_url" {
  value = apicp_phppgadmin.this.url
}
