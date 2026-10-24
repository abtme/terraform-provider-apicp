# The largest message the mail server accepts, incoming and outgoing. One per server.
# 50 MB lets through attachments up to roughly 36 MB (encoding adds about a third).
resource "apicp_mail_settings" "this" {
  max_message_size_mb = 50
}

# Or no limit at all:
#
# resource "apicp_mail_settings" "this" {
#   max_message_size_mb = 0
# }
