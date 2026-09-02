resource "apicp_dns_zone" "example" {
  name = "example.com"
}

resource "apicp_dns_record" "www" {
  zone_id = apicp_dns_zone.example.id
  name    = "www"
  type    = "A"
  content = "203.0.113.10"
  ttl     = 300
}
