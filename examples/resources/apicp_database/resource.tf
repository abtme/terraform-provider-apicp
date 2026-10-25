resource "apicp_database" "example" {
  engine = "postgresql" # or "mysql"
  name   = "example_app"
}

output "example_db_password" {
  value     = apicp_database.example.password
  sensitive = true
}

# A database linked to the website that uses it is backed up in that
# website's archive, so the site and its data are restored together.
resource "apicp_web_domain" "shop" {
  domain = "shop.example.com"
}

resource "apicp_database" "shop" {
  engine   = "mysql"
  name     = "shop_wp"
  vhost_id = apicp_web_domain.shop.id
}
