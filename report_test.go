package main

import (
	"strings"
	"testing"
	"time"
)

func TestSettingsReportHidesSecrets(t *testing.T) {
	const (
		smsToken = "smsSECRETtoken1234567890abcdef12"
		waToken  = "waSECRETtoken1234567890abcdef123"
		mailPass = "mailSECRETpassword"
		sid      = "ACfakefakefakefakefakefakefake1234" // made up; never use a real one here
	)
	cfg := &Config{
		CheckEveryMinutes: 30,
		Pages:             []Page{{URL: "https://example.com/events", Words: []string{"tickets", "מבצע"}}},
		Alerts: Alerts{
			Ntfy:     &NtfyConfig{Topic: "pagewatch-abc123"},
			SMS:      &TwilioConfig{sid, smsToken, "+15005550006", "+61412345678"},
			WhatsApp: &WhatsAppConfig{sid, waToken, "+15005550006", "+61412345678", "HXfakefakefakefakefakefakefake1234"},
			Email:    &EmailConfig{"smtp.gmail.com", 587, "me@gmail.com", mailPass, "dad@example.com"},
		},
	}
	st := &State{LastCheck: time.Date(2026, 11, 24, 18, 0, 0, 0, time.Local), LastResult: "https://example.com/events: none of the words are there"}
	r := settingsReport(cfg, st, scheduleInfo{Installed: true, Minutes: 30})

	for _, secret := range []string{smsToken, waToken, mailPass, sid} {
		if strings.Contains(r, secret) {
			t.Errorf("report shows a secret: %q", secret)
		}
	}
	for _, want := range []string{
		"https://example.com/events", `"tickets", "מבצע"`, "Every 30 minutes",
		"set up and running", "Tue 24 Nov 18:00", "none of the words are there",
		"pagewatch-abc123", "+61412345678", "ACfa…1234", "Auth Token:  set (hidden)",
		"HXfakefakefakefakefakefakefake1234", "dad@example.com", "Password:    set (hidden)",
		"smtp.gmail.com, port 587",
	} {
		if !strings.Contains(r, want) {
			t.Errorf("report is missing %q", want)
		}
	}
}

func TestSettingsReportScheduleProblems(t *testing.T) {
	cfg := baseConfig(Alerts{Ntfy: ntfyA})
	if r := settingsReport(cfg, &State{}, scheduleInfo{}); !strings.Contains(r, "NOT set up") ||
		!strings.Contains(r, "Last check: none yet") {
		t.Errorf("missing schedule should be reported:\n%s", r)
	}
	if r := settingsReport(cfg, nil, scheduleInfo{Installed: true, Minutes: 10}); !strings.Contains(r, "runs every 10 minutes") {
		t.Errorf("a schedule that doesn't match the setting should be reported:\n%s", r)
	}
}

func TestSettingsReportMissingSecret(t *testing.T) {
	cfg := baseConfig(Alerts{Email: &EmailConfig{"smtp.gmail.com", 587, "me@gmail.com", "", "dad@example.com"}})
	if r := settingsReport(cfg, nil, scheduleInfo{Installed: true}); !strings.Contains(r, "Password:    missing") {
		t.Errorf("an empty password should show as missing:\n%s", r)
	}
}
