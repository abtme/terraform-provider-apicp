# The node's default relay: all outgoing mail, except that of mail domains with a relay of their own.
# (Or manage the default relay with apicp_smtp_relay - one or the other per node, not both.)
resource "apicp_mail_relay" "main" {
  node_id  = "node-1"
  name     = "main"
  host     = "smtp.example-relay.com"
  port     = 587
  username = "relay-user"
  password = var.main_relay_password
  default  = true
}

# Another relay, used only by the mail domains that name it (relay_id). Port 465 is TLS from the
# first byte.
resource "apicp_mail_relay" "shop" {
  node_id  = "node-1"
  name     = "shop"
  host     = "smtps.other-provider.example"
  port     = 465
  username = "shop-sender"
  password = var.shop_relay_password
}

# Mail whose sender is @shop.example goes out through the shop relay; bounces and every other
# domain use the default relay.
resource "apicp_mail_domain" "shop" {
  name     = "shop.example"
  relay_id = apicp_mail_relay.shop.id
}

variable "main_relay_password" {
  type      = string
  sensitive = true
}

variable "shop_relay_password" {
  type      = string
  sensitive = true
}
