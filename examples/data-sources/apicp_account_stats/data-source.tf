data "apicp_account_stats" "example" {
  account_id = apicp_account.example.id
}

output "example_web_domains" {
  value = data.apicp_account_stats.example.web_domains
}
