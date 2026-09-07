resource "apicp_system_backup_config" "this" {
  enabled     = true
  destination = "local"
}

output "system_backup_last_run_at" {
  value = apicp_system_backup_config.this.last_run_at
}
