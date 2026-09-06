resource "apicp_web_domain" "example" {
  domain = "example.com"
}

resource "apicp_cron_job" "backup" {
  vhost_id = apicp_web_domain.example.id
  schedule = "0 3 * * *"
  command  = "tar czf backup.tar.gz public_html"
}
