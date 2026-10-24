# The mail server's server-wide settings. One per server.
resource "apicp_mail_settings" "this" {
  # The largest message accepted, incoming and outgoing (0 = unlimited). 50 MB lets through
  # attachments up to roughly 36 MB (encoding adds about a third).
  max_message_size_mb = 50

  # Only while mailboxes moved from an older server still have its MD5-crypt (or similar) password
  # hashes: Dovecot 2.4 refuses those logins otherwise. Back to false (the default) once they have
  # new passwords.
  # allow_weak_password_schemes = true
}
