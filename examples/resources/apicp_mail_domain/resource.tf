resource "apicp_mail_domain" "example" {
  name = "example.com"
}

# New mail stored compressed (zstd at its default level 3; needs apicp v0.8.0).
# Mail already stored is compressed with `apicp mail compress-existing --domain archive.example.com`.
resource "apicp_mail_domain" "archive" {
  name        = "archive.example.com"
  compression = "zstd"
}
