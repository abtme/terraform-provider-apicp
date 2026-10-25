package client

import "testing"

// POST /v1/databases takes engine, name and vhost_id only (apicp v0.7.0
// refuses anything else); vhost_id is left out when there is no link.
func TestCreateDatabaseSendsOnlyKnownFields(t *testing.T) {
	c, last := capture(t, `{"id":"db_1","engine":"mysql","name":"shop","password":"p"}`)
	if _, err := c.CreateDatabase("mysql", "shop", "vh_1"); err != nil {
		t.Fatal(err)
	}
	if *last != `{"engine":"mysql","name":"shop","vhost_id":"vh_1"}` {
		t.Errorf("body = %s", *last)
	}
	if _, err := c.CreateDatabase("postgres", "shop", ""); err != nil {
		t.Fatal(err)
	}
	if *last != `{"engine":"postgres","name":"shop"}` {
		t.Errorf("body = %s", *last)
	}
}

// PATCH /v1/databases/{id} takes vhost_id only; "" unlinks, so it is sent.
func TestSetDatabaseVhostSendsOnlyVhostID(t *testing.T) {
	c, last := capture(t, `{"id":"db_1"}`)
	for vhost, want := range map[string]string{"vh_2": `{"vhost_id":"vh_2"}`, "": `{"vhost_id":""}`} {
		if _, err := c.SetDatabaseVhost("db_1", vhost); err != nil {
			t.Fatal(err)
		}
		if *last != want {
			t.Errorf("vhost %q: body = %s, want %s", vhost, *last, want)
		}
	}
}
