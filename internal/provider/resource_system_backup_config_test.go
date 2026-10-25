package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/abtme/terraform-provider-apicp/internal/client"
)

// fakeBackupConfig is apicpd's /v1/system/backup-config. It records every
// write body as sent (so a key's presence and null can be checked). old = an
// apicpd from before PATCH (405, and no backup_unit/verify/warnings).
type fakeBackupConfig struct {
	old     bool
	get     map[string]any // the GET (and write) answer
	patches []map[string]any
	puts    []map[string]any
	gets    int
	refuse  string // a field this apicpd does not know: a write sending it gets apicp v0.7.0's 422
}

func (f *fakeBackupConfig) client(t *testing.T) *client.Client {
	t.Helper()
	if f.get == nil {
		f.get = map[string]any{"enabled": true, "destination": "local", "retention": 3, "format": "targz",
			"compress": true, "encrypt": false, "encryption_available": true, "updated_at": "2026-10-25T10:00:00Z"}
		if !f.old {
			f.get["backup_unit"] = "domain"
			f.get["verify"] = true
			f.get["encryption_warning"] = "backups are written unencrypted although this server has an encryption key"
			f.get["last_run_warnings"] = []string{"2026-10-25/root._._.example.org.mail.tar.gz: mailbox a@example.org: password not readable"}
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/system/backup-config" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var body map[string]any
		switch r.Method {
		case http.MethodGet:
			f.gets++
		case http.MethodPatch:
			if f.old {
				w.WriteHeader(http.StatusMethodNotAllowed)
				_, _ = w.Write([]byte("Method Not Allowed\n"))
				return
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.patches = append(f.patches, body)
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.puts = append(f.puts, body)
		default:
			t.Errorf("unexpected %s", r.Method)
		}
		if _, has := body[f.refuse]; has && f.refuse != "" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":"unknown field \"` + f.refuse + `\": apicpd 0.7.0 does not know it (is the client newer than this server?); nothing was changed"}`))
			return
		}
		for k, v := range body {
			if v != nil {
				f.get[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f.get)
	}))
	t.Cleanup(srv.Close)
	return client.New(srv.URL, "token")
}

func backupConfigCreate(t *testing.T, f *fakeBackupConfig, config map[string]any) (resource.CreateResponse, SystemBackupConfigResourceModel) {
	t.Helper()
	r := &SystemBackupConfigResource{client: f.client(t)}
	sch := resourceSchema(t, r)
	planVals := map[string]any{"id": unknownValue{}, "next_run_at": unknownValue{}, "last_run_at": unknownValue{},
		"last_run_error": unknownValue{}, "encryption_warning": unknownValue{}, "last_run_warnings": unknownValue{}, "retention": unknownValue{}}
	for k, v := range config {
		planVals[k] = v
	}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(context.Background()), nil)}}
	r.Create(context.Background(), resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: sch, Raw: objectValue(t, sch, planVals)},
		Config: tfsdk.Config{Schema: sch, Raw: objectValue(t, sch, config)},
	}, &resp)
	var got SystemBackupConfigResourceModel
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Get(context.Background(), &got)...)
	}
	return resp, got
}

func keys(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Only what the configuration sets is sent: no encrypt (or anything else)
// read from the server is written back.
func TestSystemBackupConfigCreatePatchesOnlyWhatIsSet(t *testing.T) {
	f := &fakeBackupConfig{}
	resp, got := backupConfigCreate(t, f, map[string]any{"enabled": true, "destination": "sftp", "backup_unit": "domain", "verify": false})
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if len(f.puts) != 0 || len(f.patches) != 1 {
		t.Fatalf("PATCHes %v, PUTs %v: want one PATCH", f.patches, f.puts)
	}
	p := f.patches[0]
	if len(p) != 4 || p["enabled"] != true || p["destination"] != "sftp" || p["backup_unit"] != "domain" || p["verify"] != false {
		t.Errorf("PATCH body = %v, want exactly enabled, destination, backup_unit and verify", p)
	}
	if !got.Encrypt.IsNull() || !got.Compress.IsNull() || !got.Format.IsNull() {
		t.Errorf("unset settings must stay null in state, got encrypt=%v compress=%v format=%v", got.Encrypt, got.Compress, got.Format)
	}
	if got.Retention.ValueInt64() != 3 || !strings.Contains(got.EncryptionWarning.ValueString(), "unencrypted") || len(got.LastRunWarnings.Elements()) != 1 {
		t.Errorf("state = %+v", got)
	}
	// the server has a key and writes plain backups: say so
	if resp.Diagnostics.WarningsCount() != 1 || !strings.Contains(resp.Diagnostics.Warnings()[0].Detail(), "encrypt = true") {
		t.Errorf("want one encryption warning, got %v", resp.Diagnostics)
	}
}

// encrypt = false set on purpose: no warning about it.
func TestSystemBackupConfigNoWarningWhenEncryptFalseIsChosen(t *testing.T) {
	resp, _ := backupConfigCreate(t, &fakeBackupConfig{}, map[string]any{"enabled": true, "destination": "local", "encrypt": false})
	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() != 0 {
		t.Errorf("diagnostics: %v", resp.Diagnostics)
	}
}

// A setting removed from the configuration goes back to the server's default
// (null for the booleans, empty for the rest); one never managed is not sent.
func TestSystemBackupConfigUpdateResetsRemovedSettings(t *testing.T) {
	f := &fakeBackupConfig{}
	r := &SystemBackupConfigResource{client: f.client(t)}
	sch := resourceSchema(t, r)

	prior := map[string]any{"id": "system_backup_config", "enabled": true, "destination": "local", "retention": int64(3),
		"encrypt": false, "schedule_time": "03:30", "schedule_days": []string{"mon"}, "backup_unit": "account",
		"next_run_at": "", "last_run_at": "", "last_run_error": "", "encryption_warning": "", "last_run_warnings": []string{}}
	config := map[string]any{"enabled": true, "destination": "local", "retention": int64(5)}
	plan := map[string]any{"id": "system_backup_config", "enabled": true, "destination": "local", "retention": int64(5),
		"next_run_at": unknownValue{}, "last_run_at": unknownValue{}, "last_run_error": unknownValue{},
		"encryption_warning": unknownValue{}, "last_run_warnings": unknownValue{}}

	resp := resource.UpdateResponse{State: tfsdk.State{Schema: sch, Raw: objectValue(t, sch, prior)}}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: sch, Raw: objectValue(t, sch, plan)},
		Config: tfsdk.Config{Schema: sch, Raw: objectValue(t, sch, config)},
		State:  tfsdk.State{Schema: sch, Raw: objectValue(t, sch, prior)},
	}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if len(f.patches) != 1 {
		t.Fatalf("PATCHes = %v", f.patches)
	}
	p := f.patches[0]
	want := map[string]any{"enabled": true, "destination": "local", "retention": float64(5), "encrypt": nil,
		"schedule_time": "", "schedule_days": []any{}, "backup_unit": ""}
	if len(p) != len(want) {
		t.Errorf("PATCH keys = %v, want %v", keys(p), keys(want))
	}
	for k, v := range want {
		got, ok := p[k]
		if !ok {
			t.Errorf("PATCH lacks %s: %v", k, p)
			continue
		}
		if gs, isList := got.([]any); isList {
			if len(gs) != 0 {
				t.Errorf("PATCH %s = %v, want []", k, got)
			}
			continue
		}
		if got != v {
			t.Errorf("PATCH %s = %v, want %v", k, got, v)
		}
	}
}

// An apicpd without PATCH: PUT as before, keeping the server's retention
// when the configuration leaves it out (PUT would set 0).
func TestSystemBackupConfigOldApicpdFallsBackToPut(t *testing.T) {
	f := &fakeBackupConfig{old: true}
	resp, got := backupConfigCreate(t, f, map[string]any{"enabled": true, "destination": "local", "compress": false})
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if len(f.puts) != 1 {
		t.Fatalf("PUTs = %v", f.puts)
	}
	p := f.puts[0]
	if p["retention"] != float64(3) || p["compress"] != false || p["enabled"] != true {
		t.Errorf("PUT body = %v, want retention 3 (the server's), compress false", p)
	}
	if _, has := p["encrypt"]; has {
		t.Errorf("encrypt is not set, so the PUT must leave it out: %v", p)
	}
	if got.EncryptionWarning.ValueString() != "" || len(got.LastRunWarnings.Elements()) != 0 || got.LastRunWarnings.IsNull() {
		t.Errorf("state = %+v", got)
	}
}

func TestSystemBackupConfigOldApicpdRefusesNewSettings(t *testing.T) {
	f := &fakeBackupConfig{old: true}
	resp, _ := backupConfigCreate(t, f, map[string]any{"enabled": true, "destination": "local", "verify": true})
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "verify") ||
		!strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "v0.7.0") {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if len(f.puts) != 0 {
		t.Errorf("nothing may be written: PUTs %v", f.puts)
	}
}

// Read leaves unmanaged settings null (the server's effective encrypt=false
// is not taken in) and shows drift on managed ones.
func TestSystemBackupConfigReadOnlyRefreshesManagedSettings(t *testing.T) {
	f := &fakeBackupConfig{}
	r := &SystemBackupConfigResource{client: f.client(t)}
	sch := resourceSchema(t, r)
	f.get["compress"] = false // changed outside Terraform

	state := tfsdk.State{Schema: sch, Raw: objectValue(t, sch, map[string]any{"id": "system_backup_config", "enabled": true,
		"destination": "local", "retention": int64(3), "compress": true})}
	resp := resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	var got SystemBackupConfigResourceModel
	resp.State.Get(context.Background(), &got)
	if !got.Encrypt.IsNull() || !got.Verify.IsNull() || !got.BackupUnit.IsNull() {
		t.Errorf("unmanaged settings must stay null: encrypt=%v verify=%v backup_unit=%v", got.Encrypt, got.Verify, got.BackupUnit)
	}
	if got.Compress.ValueBool() {
		t.Errorf("compress = %v, want the server's false (drift)", got.Compress)
	}
	if got.EncryptionWarning.ValueString() == "" || len(got.LastRunWarnings.Elements()) != 1 {
		t.Errorf("state = %+v", got)
	}
}

// Destroy turns the scheduler off and changes nothing else.
func TestSystemBackupConfigDeleteOnlyDisables(t *testing.T) {
	for _, old := range []bool{false, true} {
		f := &fakeBackupConfig{old: old}
		r := &SystemBackupConfigResource{client: f.client(t)}
		sch := resourceSchema(t, r)
		state := tfsdk.State{Schema: sch, Raw: objectValue(t, sch, map[string]any{"id": "system_backup_config", "enabled": true,
			"destination": "sftp", "retention": int64(7)})}
		var resp resource.DeleteResponse
		r.Delete(context.Background(), resource.DeleteRequest{State: state}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("old=%v: %v", old, resp.Diagnostics)
		}
		if !old && (len(f.patches) != 1 || len(f.patches[0]) != 1 || f.patches[0]["enabled"] != false || len(f.puts) != 0) {
			t.Errorf("PATCHes %v PUTs %v, want one PATCH {enabled: false}", f.patches, f.puts)
		}
		if old && (len(f.puts) != 1 || f.puts[0]["enabled"] != false || f.puts[0]["destination"] != "sftp" || f.puts[0]["retention"] != float64(7)) {
			t.Errorf("old apicpd: PUTs %v, want the state's settings with enabled false", f.puts)
		}
	}
}

// apicp v0.7.0 refuses a field it does not know with 422: a clear error
// naming the argument, and no PUT fallback.
func TestSystemBackupConfigUnknownFieldIsAClearError(t *testing.T) {
	f := &fakeBackupConfig{refuse: "verify"}
	resp, _ := backupConfigCreate(t, f, map[string]any{"enabled": true, "destination": "local", "verify": true})
	if !resp.Diagnostics.HasError() {
		t.Fatal("want an error")
	}
	d := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(d, `does not support "verify"`) || !strings.Contains(d, "remove the verify argument") || !strings.Contains(d, "Upgrade apicpd") {
		t.Errorf("detail = %s", d)
	}
	if len(f.puts) != 0 {
		t.Errorf("a 422 must not fall back to PUT: %v", f.puts)
	}
}

// The PUT fallback sends exactly the settings PUT knows and the
// configuration sets: nothing GET computes, never backup_unit or verify.
func TestSystemBackupConfigPutBodyIsExact(t *testing.T) {
	f := &fakeBackupConfig{old: true}
	resp, _ := backupConfigCreate(t, f, map[string]any{"enabled": true, "destination": "local", "retention": int64(4),
		"format": "zip", "compress": true, "encrypt": false, "schedule_time": "03:30", "schedule_days": []string{"mon"}, "timezone": "Europe/London"})
	if resp.Diagnostics.HasError() || len(f.puts) != 1 {
		t.Fatalf("diagnostics %v, PUTs %v", resp.Diagnostics, f.puts)
	}
	got, _ := json.Marshal(f.puts[0])
	want := `{"compress":true,"destination":"local","enabled":true,"encrypt":false,"format":"zip","retention":4,"schedule_days":["mon"],"schedule_time":"03:30","timezone":"Europe/London"}`
	if string(got) != want {
		t.Errorf("PUT body = %s\nwant       %s", got, want)
	}

	// nothing optional set: enabled, destination and the server's retention only
	f = &fakeBackupConfig{old: true}
	if resp, _ = backupConfigCreate(t, f, map[string]any{"enabled": false, "destination": "local"}); resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	got, _ = json.Marshal(f.puts[0])
	if string(got) != `{"destination":"local","enabled":false,"retention":3}` {
		t.Errorf("PUT body = %s", got)
	}
}

// Every key the provider can PATCH is one apicp knows.
func TestSystemBackupConfigPatchBodyHasOnlyKnownFields(t *testing.T) {
	known := map[string]bool{"enabled": true, "destination": true, "retention": true, "format": true, "compress": true, "encrypt": true,
		"backup_unit": true, "verify": true, "schedule_time": true, "schedule_days": true, "timezone": true}
	f := &fakeBackupConfig{}
	resp, _ := backupConfigCreate(t, f, map[string]any{"enabled": true, "destination": "local", "retention": int64(4),
		"format": "zip", "compress": true, "encrypt": true, "schedule_time": "03:30", "schedule_days": []string{"mon"},
		"timezone": "Europe/London", "backup_unit": "account", "verify": true})
	if resp.Diagnostics.HasError() || len(f.patches) != 1 {
		t.Fatalf("diagnostics %v, PATCHes %v", resp.Diagnostics, f.patches)
	}
	if len(f.patches[0]) != len(known) {
		t.Errorf("PATCH keys = %v, want all %d known settings", keys(f.patches[0]), len(known))
	}
	for k := range f.patches[0] {
		if !known[k] {
			t.Errorf("PATCH sends %q, which apicp does not know", k)
		}
	}
}
