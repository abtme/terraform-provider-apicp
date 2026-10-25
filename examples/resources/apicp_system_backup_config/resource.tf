resource "apicp_system_backup_config" "this" {
  enabled     = true
  destination = "local"

  # Optional: only the settings set here are managed; the rest keep what
  # the server has saved.
  encrypt     = true     # needs an encryption key on the server
  backup_unit = "domain" # or "account" for one archive per account
  verify      = true     # read every run back after it is written
}

output "system_backup_last_run_at" {
  value = apicp_system_backup_config.this.last_run_at
}

output "system_backup_warnings" {
  value = concat(
    compact([apicp_system_backup_config.this.encryption_warning]),
    apicp_system_backup_config.this.last_run_warnings,
  )
}
