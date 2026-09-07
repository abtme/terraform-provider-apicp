resource "apicp_package" "starter" {
  name     = "starter"
  for_tier = "user"

  max_web_domains  = 3
  max_databases    = 2
  max_mail_domains = 1
  max_mailboxes    = 5
  max_cron_jobs    = 2
}
