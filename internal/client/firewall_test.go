package client

import "testing"

// A rule's list and a restriction's lists go on the wire only when set, so an
// apicpd before address lists sees exactly the requests it always did.
func TestFirewallListFieldsAreSentOnlyWhenSet(t *testing.T) {
	c, last := capture(t, `{"id":"fw_1","protocol":"all","action":"accept","list":"ovh-monitoring"}`)
	if _, err := c.CreateFirewallRule("", nil, "", "9911", "tcp", "accept", ""); err != nil {
		t.Fatal(err)
	}
	if *last != `{"port":"9911","protocol":"tcp","action":"accept"}` {
		t.Errorf("rule without a list: %s", *last)
	}
	r, err := c.CreateFirewallRule("", nil, "ovh-monitoring", "", "all", "accept", "")
	if err != nil {
		t.Fatal(err)
	}
	if *last != `{"list":"ovh-monitoring","protocol":"all","action":"accept"}` || r.List != "ovh-monitoring" {
		t.Errorf("rule with a list: %s %+v", *last, r)
	}

	if _, err := c.SetFirewallRestriction(22, nil, nil, "", false); err != nil {
		t.Fatal(err)
	}
	if *last != `{"sources":[]}` {
		t.Errorf("restriction without lists: %s", *last)
	}
	if _, err := c.SetFirewallRestriction(22, nil, []string{"office"}, "", false); err != nil {
		t.Fatal(err)
	}
	if *last != `{"sources":[],"lists":["office"]}` {
		t.Errorf("restriction with lists: %s", *last)
	}
}

func TestFirewallListRequests(t *testing.T) {
	c, last := capture(t, `{"id":"fwl_1","name":"office","sources":["192.0.2.0/24"],"addresses":["192.0.2.0/24"],"used_by":[]}`)
	l, err := c.CreateFirewallList(FirewallListInput{Name: "office", Sources: []string{"192.0.2.0/24"}})
	if err != nil {
		t.Fatal(err)
	}
	if *last != `{"name":"office","sources":["192.0.2.0/24"]}` || l.ID != "fwl_1" || len(l.Addresses) != 1 {
		t.Errorf("create: %s %+v", *last, l)
	}
	src := []string{"203.0.113.9"}
	if _, err := c.UpdateFirewallList("fwl_1", FirewallListPatch{Sources: &src}); err != nil {
		t.Fatal(err)
	}
	if *last != `{"sources":["203.0.113.9"]}` {
		t.Errorf("patch: %s", *last)
	}
}
