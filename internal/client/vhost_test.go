package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// capture serves one canned vhost and remembers the last request body.
func capture(t *testing.T, reply string) (*Client, *string) {
	t.Helper()

	var last string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		last = string(b)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)

	return New(srv.URL, "token"), &last
}

func TestCreateVhostSendsAliases(t *testing.T) {
	c, last := capture(t, `{"id":"vh_1","domain":"example.test","aliases":["www.example.test"]}`)

	v, err := c.CreateVhost("example.test", []string{"www.example.test"}, VhostRuntime{})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(*last, `"aliases":["www.example.test"]`) {
		t.Errorf("request body %s lacks the aliases", *last)
	}

	if len(v.Aliases) != 1 || v.Aliases[0] != "www.example.test" {
		t.Errorf("aliases read back as %v", v.Aliases)
	}

	// none given: the field is left out entirely
	if _, err := c.CreateVhost("example.test", nil, VhostRuntime{}); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(*last, "aliases") {
		t.Errorf("request body %s should not mention aliases", *last)
	}
}

// The API replaces the whole list, so a PATCH must always carry it, and an empty list as []: that is how
// the last alias is removed. (Leaving it out would leave the aliases as they were.)
func TestPatchVhostAlwaysSendsTheAliasList(t *testing.T) {
	c, last := capture(t, `{"id":"vh_1","domain":"example.test"}`)

	for name, aliases := range map[string][]string{"nil": nil, "empty": {}, "some": {"a.example.test", "b.example.test"}} {
		if _, err := c.PatchVhost("vh_1", "example.test", aliases, VhostRuntime{}); err != nil {
			t.Fatal(err)
		}

		var body struct {
			Aliases *[]string `json:"aliases"`
		}

		if err := json.Unmarshal([]byte(*last), &body); err != nil {
			t.Fatalf("%s: %v (%s)", name, err, *last)
		}

		if body.Aliases == nil {
			t.Errorf("%s: the PATCH body %s has no aliases list", name, *last)
		} else if len(*body.Aliases) != len(aliases) {
			t.Errorf("%s: sent %v, want %v", name, *body.Aliases, aliases)
		}
	}
}

func TestCertificateNamesAreRead(t *testing.T) {
	c, _ := capture(t, `{"id":"c","vhost_id":"vh_1","domain":"example.test","names":["example.test","www.example.test"]}`)

	cert, err := c.GetCertificate("vh_1")
	if err != nil {
		t.Fatal(err)
	}

	if len(cert.Names) != 2 || cert.Names[1] != "www.example.test" {
		t.Errorf("names = %v", cert.Names)
	}
}

func TestCreateVhostSendsTheNodeRuntimeOnly(t *testing.T) {
	c, last := capture(t, `{"id":"vh_1","domain":"example.test","node_version":"24","app_port":20001}`)

	v, err := c.CreateVhost("example.test", nil, VhostRuntime{NodeVersion: "24", AppEntry: "server.js"})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(*last, `"node_version":"24"`) || !strings.Contains(*last, `"app_entry":"server.js"`) || strings.Contains(*last, "php_version") {
		t.Errorf("request body %s", *last)
	}

	if v.NodeVersion != "24" || v.AppPort != 20001 {
		t.Errorf("read back %+v", v)
	}
}

// Moving from PHP to Node (or back) is one PATCH that must clear the other runtime explicitly.
func TestPatchVhostAlwaysSendsEveryRuntimeField(t *testing.T) {
	c, last := capture(t, `{"id":"vh_1","domain":"example.test"}`)

	if _, err := c.PatchVhost("vh_1", "example.test", nil, VhostRuntime{NodeVersion: "24"}); err != nil {
		t.Fatal(err)
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(*last), &body); err != nil {
		t.Fatal(err)
	}

	for k, want := range map[string]string{"php_version": "", "node_version": "24", "app_entry": ""} {
		if got, ok := body[k]; !ok || got != want {
			t.Errorf("%s = %v (present %v), want %q in %s", k, got, ok, want, *last)
		}
	}
}
