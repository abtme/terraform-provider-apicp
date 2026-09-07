resource "apicp_phpmyadmin" "this" {}

output "phpmyadmin_url" {
  value = apicp_phpmyadmin.this.url
}
