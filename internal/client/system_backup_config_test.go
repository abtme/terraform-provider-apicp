package client

import (
	"encoding/json"
	"testing"
)

func TestSetSystemBackupConfigSendsEverySettingAndOmitsDefaults(t *testing.T) {
	c, last := capture(t, `{"enabled":true,"destination":"sftp","retention":14,"compress":true,"encrypt":true,"schedule_time":"03:30","schedule_days":["mon","fri"],"timezone":"Europe/London","next_run_at":"2026-10-16T02:30:00Z"}`)

	no := false
	cfg, err := c.SetSystemBackupConfig(SystemBackupConfigInput{
		Enabled: true, Destination: "sftp", Retention: 14, Format: "zip", Compress: &no,
		ScheduleTime: "03:30", ScheduleDays: []string{"mon", "fri"}, Timezone: "Europe/London",
	})
	if err != nil {
		t.Fatal(err)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(*last), &sent); err != nil {
		t.Fatal(err)
	}

	if sent["destination"] != "sftp" || sent["retention"] != float64(14) || sent["format"] != "zip" || sent["compress"] != false ||
		sent["schedule_time"] != "03:30" || sent["timezone"] != "Europe/London" {
		t.Errorf("body = %s", *last)
	}

	if _, has := sent["encrypt"]; has {
		t.Errorf("encrypt was not set, so it must be left out (the server then uses its default): %s", *last)
	}

	if cfg.NextRunAt == "" || len(cfg.ScheduleDays) != 2 || !cfg.Encrypt {
		t.Errorf("response = %+v", cfg)
	}

	// nothing optional: only the three required settings go
	_, _ = c.SetSystemBackupConfig(SystemBackupConfigInput{Destination: "local"})
	var minimal map[string]any
	_ = json.Unmarshal([]byte(*last), &minimal)

	for k := range minimal {
		if k != "enabled" && k != "destination" && k != "retention" {
			t.Errorf("unexpected field %q in %s", k, *last)
		}
	}
}

// The PUT body (apicpd without PATCH, and apicp v0.7.0 refuses unknown
// fields on PUT too) holds only the settings PUT knows, never a value GET
// computes (encryption_available, next_run_at, backup_unit, verify, ...).
func TestSetSystemBackupConfigSendsOnlyKnownFields(t *testing.T) {
	c, last := capture(t, `{"enabled":true,"destination":"local"}`)
	yes, no := true, false
	if _, err := c.SetSystemBackupConfig(SystemBackupConfigInput{
		Enabled: true, Destination: "local", Retention: 3, Format: "targz", Compress: &yes, Encrypt: &no,
		ScheduleTime: "03:30", ScheduleDays: []string{"mon"}, Timezone: "UTC",
	}); err != nil {
		t.Fatal(err)
	}
	want := `{"enabled":true,"destination":"local","retention":3,"format":"targz","compress":true,"encrypt":false,"schedule_time":"03:30","schedule_days":["mon"],"timezone":"UTC"}`
	if *last != want {
		t.Errorf("body = %s\nwant   %s", *last, want)
	}
}
