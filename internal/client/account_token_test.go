package client

import "testing"

// mailbox_id is sent only for a mailbox token, so an apicp before 0.8.5 sees the same request as before.
func TestAccountTokenMailboxIsSentOnlyWhenSet(t *testing.T) {
	c, last := capture(t, `{"token":"apicp_x"}`)
	if _, err := c.CreateAccountToken("acct_1", "panel", []string{"mail:read"}, "", ""); err != nil {
		t.Fatal(err)
	}
	if *last != `{"label":"panel","scopes":["mail:read"]}` {
		t.Errorf("an account token: %s", *last)
	}
	if _, err := c.CreateAccountToken("acct_1", "jo", []string{"mailbox:read", "mailbox:write"}, "", "mbx_7"); err != nil {
		t.Fatal(err)
	}
	if *last != `{"label":"jo","scopes":["mailbox:read","mailbox:write"],"mailbox_id":"mbx_7"}` {
		t.Errorf("a mailbox token: %s", *last)
	}
}
