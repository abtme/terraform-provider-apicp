resource "apicp_mail_domain" "example" {
  name = "example.com"
}

# A domain whose outgoing mail goes through a relay of its own (apicp v0.8.0 or later; see
# apicp_mail_relay). relay_id = "" puts it back on the server's default relay.
resource "apicp_mail_domain" "shop" {
  name     = "shop.example"
  relay_id = apicp_mail_relay.shop.id
}

# New mail stored compressed (zstd at its default level 3; needs apicp v0.8.0).
# Mail already stored is compressed with `apicp mail compress-existing --domain archive.example.com`.
resource "apicp_mail_domain" "archive" {
  name        = "archive.example.com"
  compression = "zstd"
}
