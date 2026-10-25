resource "apicp_account_backup" "nightly" {
  account_id = apicp_account.example.id
}

# With per-domain backups one backup is several archives: the account's
# settings, then a web and a mail archive per domain.
output "backup_archives" {
  value = {
    for b in apicp_account_backup.nightly.backups : b.id => "${b.scope} ${b.domain} (${b.size_bytes} bytes)"
  }
}
