resource "apicp_package" "reseller_starter" {
  name             = "reseller-starter"
  for_tier         = "reseller"
  max_sub_accounts = 10
}

resource "apicp_account" "agency" {
  name       = "Acme Hosting"
  tier       = "reseller"
  package_id = apicp_package.reseller_starter.id
}

resource "apicp_package" "user_starter" {
  name            = "user-starter"
  for_tier        = "user"
  max_web_domains = 3
}

# Created with the admin provider's token; in practice a reseller mints
# its own users' accounts with its own token instead (see
# apicp_account's schema description).
resource "apicp_account" "client" {
  name              = "Bob"
  tier              = "user"
  parent_account_id = apicp_account.agency.id
  package_id        = apicp_package.user_starter.id
}
