package client

import "testing"

// relay_id goes on create only when set (an apicpd before v0.8.0 does not know it), and on update
// only when it changes ("" = back to the default relay).
func TestMailDomainRelayIsSentOnlyWhenSet(t *testing.T) {
	c, last := capture(t, `{"id":"mdom_1","name":"a.example"}`)
	if _, err := c.CreateMailDomain("a.example", Protocols{}, 0, ""); err != nil {
		t.Fatal(err)
	}
	if *last != `{"name":"a.example","disable_imap":false,"disable_pop3":false,"max_emails_per_hour":0}` {
		t.Errorf("create without a relay: %s", *last)
	}
	if _, err := c.CreateMailDomain("a.example", Protocols{}, 0, "relay_1"); err != nil {
		t.Fatal(err)
	}
	if *last != `{"name":"a.example","disable_imap":false,"disable_pop3":false,"max_emails_per_hour":0,"relay_id":"relay_1"}` {
		t.Errorf("create with a relay: %s", *last)
	}
	if _, err := c.UpdateMailDomain("mdom_1", Protocols{}, 5, nil); err != nil {
		t.Fatal(err)
	}
	if *last != `{"disable_imap":false,"disable_pop3":false,"max_emails_per_hour":5}` {
		t.Errorf("update without a relay change: %s", *last)
	}
	empty := ""
	if _, err := c.UpdateMailDomain("mdom_1", Protocols{}, 5, &empty); err != nil {
		t.Fatal(err)
	}
	if *last != `{"disable_imap":false,"disable_pop3":false,"max_emails_per_hour":5,"relay_id":""}` {
		t.Errorf("back to the default relay: %s", *last)
	}
}

func TestCreateRelayBody(t *testing.T) {
	c, last := capture(t, `{"id":"relay_1"}`)
	if _, err := c.CreateRelay("node-1", SMTPRelayCreate{Name: "lan", Host: "relay.lan", Port: 25}); err != nil {
		t.Fatal(err)
	}
	if *last != `{"name":"lan","host":"relay.lan","port":25}` {
		t.Errorf("no login, not the default: %s", *last)
	}
	if _, err := c.CreateRelay("node-1", SMTPRelayCreate{Name: "shop", Host: "smtp.x", Port: 465, Username: "u", Password: "p", Default: true}); err != nil {
		t.Fatal(err)
	}
	if *last != `{"name":"shop","host":"smtp.x","port":465,"username":"u","password":"p","default":true}` {
		t.Errorf("%s", *last)
	}
}
