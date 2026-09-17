resource "apicp_account" "example" {
  name = "Example Reseller"
  tier = "reseller"
}

resource "apicp_totp" "example" {
  account_id = apicp_account.example.id
}

output "totp_otpauth_url" {
  value     = apicp_totp.example.otpauth_url
  sensitive = true
}
