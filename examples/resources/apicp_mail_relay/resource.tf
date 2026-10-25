# The node's default relay: all outgoing mail, except that of mail domains with a relay of their own.
# The node's first relay is always its default. (Or manage the default relay with apicp_smtp_relay -
# one or the other per node, not both.)
resource "apicp_mail_relay" "main" {
  node_id = "node-1"
  name    = "main"
  host    = "relay.example.net"
  port    = 2500
  default = true
}

# Another relay, used only by the mail domains that name it (relay_id). It accepts the server by its
# address: a relay a mail domain uses has no login and is not on port 465.
resource "apicp_mail_relay" "shop" {
  node_id = "node-1"
  name    = "shop"
  host    = "relay.shop-provider.example"
  port    = 2525
}

# Mail whose sender is @shop.example goes out through the shop relay; bounces and every other
# domain use the default relay.
resource "apicp_mail_domain" "shop" {
  name     = "shop.example"
  relay_id = apicp_mail_relay.shop.id
}
