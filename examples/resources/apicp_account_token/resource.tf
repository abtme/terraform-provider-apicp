resource "apicp_account_token" "mail_only" {
  account_id = apicp_account.example.id
  label      = "mail automation"
  scopes     = ["mail:read", "mail:write"]
}

output "mail_token" {
  value     = apicp_account_token.mail_only.token
  sensitive = true
}
