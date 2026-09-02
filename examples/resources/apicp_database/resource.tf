resource "apicp_database" "example" {
  engine = "postgresql" # or "mysql"
  name   = "example_app"
}

output "example_db_password" {
  value     = apicp_database.example.password
  sensitive = true
}
