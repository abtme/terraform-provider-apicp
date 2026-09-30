# Use Let's Encrypt's staging directory while testing.
resource "apicp_system_tls_config" "this" {
  acme_directory_url = "https://acme-staging-v02.api.letsencrypt.org/directory"
}
