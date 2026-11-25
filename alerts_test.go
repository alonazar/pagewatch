package main

import (
	"bufio"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// scripted returns a prompter that answers with the given lines, in order.
func scripted(lines ...string) *prompter {
	return &prompter{bufio.NewReader(strings.NewReader(strings.Join(lines, "\n") + "\n"))}
}

// fakeSender replaces the real alert sender. fail makes every test alert fail.
func fakeSender(t *testing.T, fail *bool) *[]Alerts {
	t.Helper()
	var sent []Alerts
	old := sendAlertFn
	sendAlertFn = func(a Alerts, title, msg, link string) (int, error) {
		sent = append(sent, a)
		if *fail {
			return 0, errors.New("could not send")
		}
		return len(alertEntries(a)), nil
	}
	t.Cleanup(func() { sendAlertFn = old })
	return &sent
}

func tempSettings(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func baseConfig(a Alerts) *Config {
	return &Config{Pages: []Page{{URL: "https://example.com", Words: []string{"x"}}}, Alerts: a}
}

var (
	ntfyA = &NtfyConfig{Topic: "pagewatch-aaaa"}
	email = &EmailConfig{SMTPServer: "smtp.gmail.com", SMTPPort: 587, Username: "me@gmail.com", Password: "pw", To: "dad@example.com"}
	sms   = &TwilioConfig{"AC1", "t", "+12369000098", "+61412345678"}
)

func TestAddAlertMethodKeepsExistingOnes(t *testing.T) {
	tempSettings(t)
	fail := false
	sent := fakeSender(t, &fail)
	cfg := baseConfig(Alerts{Ntfy: ntfyA})

	// Pick email, answer its questions, confirm the test arrived.
	addAlertMethod(scripted("4", "me@gmail.com", "abcd efgh ijkl mnop", "dad@example.com", "y"), cfg)

	if cfg.Alerts.Ntfy != ntfyA || cfg.Alerts.Email == nil || cfg.Alerts.Email.Password != "abcdefghijklmnop" {
		t.Fatalf("expected ntfy kept and email added, got %s", describeAlerts(cfg.Alerts))
	}
	// Only the new method gets the test alert.
	if len(*sent) != 1 || (*sent)[0].Ntfy != nil || (*sent)[0].Email == nil {
		t.Errorf("test alert should go only to the new method, got %v", *sent)
	}
	saved, err := loadConfig()
	if err != nil || saved.Alerts.Ntfy == nil || saved.Alerts.Email == nil {
		t.Fatalf("both methods should be saved: %v / %+v", err, saved)
	}
}

func TestAddAlertMethodFailedTestChangesNothing(t *testing.T) {
	tempSettings(t)
	fail := true
	fakeSender(t, &fail)
	cfg := baseConfig(Alerts{Ntfy: ntfyA})

	// Pick email; the test fails; decline to try again.
	addAlertMethod(scripted("4", "me@gmail.com", "abcd efgh ijkl mnop", "", "n"), cfg)

	if cfg.Alerts.Email != nil || cfg.Alerts.Ntfy != ntfyA {
		t.Fatalf("a method whose test failed must not be added, got %s", describeAlerts(cfg.Alerts))
	}
	if _, err := os.Stat(configPath()); !errors.Is(err, os.ErrNotExist) {
		t.Error("settings should not have been written")
	}
}

func TestAddAlertMethodRetryAfterFailure(t *testing.T) {
	tempSettings(t)
	fail := true
	fakeSender(t, &fail)
	cfg := baseConfig(Alerts{Ntfy: ntfyA})

	// First attempt fails; say "try again"; the fake then succeeds.
	p := scripted("4", "me@gmail.com", "short", "abcd efgh ijkl mnop", "", "y",
		"4", "me@gmail.com", "abcd efgh ijkl mnop", "", "y")
	old := sendAlertFn
	calls := 0
	sendAlertFn = func(a Alerts, title, msg, link string) (int, error) {
		calls++
		if calls == 1 {
			return 0, errors.New("could not send")
		}
		return 1, nil
	}
	defer func() { sendAlertFn = old }()

	addAlertMethod(p, cfg)
	if cfg.Alerts.Email == nil || calls != 2 {
		t.Fatalf("expected email added on the second try (calls=%d), got %s", calls, describeAlerts(cfg.Alerts))
	}
}

func TestAddSameKindReplacesIt(t *testing.T) {
	tempSettings(t)
	fail := false
	fakeSender(t, &fail)
	cfg := baseConfig(Alerts{Ntfy: ntfyA, Email: email})

	addAlertMethod(scripted("1", "", "y"), cfg) // new ntfy topic

	if cfg.Alerts.Ntfy == nil || cfg.Alerts.Ntfy.Topic == ntfyA.Topic || cfg.Alerts.Email != email {
		t.Fatalf("ntfy should be replaced and email kept, got %s", describeAlerts(cfg.Alerts))
	}
}

func TestRemoveAlertMethod(t *testing.T) {
	tempSettings(t)
	cfg := baseConfig(Alerts{Ntfy: ntfyA, SMS: sms, Email: email})

	removeAlertMethod(scripted("2"), cfg) // the second listed: SMS

	if cfg.Alerts.SMS != nil || cfg.Alerts.Ntfy == nil || cfg.Alerts.Email == nil {
		t.Fatalf("only SMS should be removed, got %s", describeAlerts(cfg.Alerts))
	}
	saved, err := loadConfig()
	if err != nil || saved.Alerts.SMS != nil || saved.Alerts.Email == nil {
		t.Fatalf("removal should be saved: %v / %+v", err, saved)
	}
}

func TestRemoveAlertMethodCancel(t *testing.T) {
	tempSettings(t)
	cfg := baseConfig(Alerts{Ntfy: ntfyA, Email: email})

	removeAlertMethod(scripted("3"), cfg) // "Cancel"

	if cfg.Alerts.Ntfy == nil || cfg.Alerts.Email == nil {
		t.Fatalf("cancel must not remove anything, got %s", describeAlerts(cfg.Alerts))
	}
}

func TestCannotRemoveLastAlertMethod(t *testing.T) {
	tempSettings(t)
	cfg := baseConfig(Alerts{Email: email})

	removeAlertMethod(scripted(), cfg) // must not ask anything

	if cfg.Alerts.Email == nil {
		t.Fatal("the only alert method must not be removed")
	}
}

func TestSetupWithTwoAlertMethods(t *testing.T) {
	tempSettings(t)
	fail := false
	fakeSender(t, &fail)
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<p>nothing yet</p>")
	}))
	defer page.Close()

	p := scripted(
		page.URL, "tickets", "n", // page, words, no other page
		"1", "", "y", // ntfy, subscribed, test arrived
		"y",                                                 // add another
		"4", "me@gmail.com", "abcd efgh ijkl mnop", "", "y", // email, test arrived
		"n", // no more
	)
	cfg, _ := runSetup(p, nil)
	if cfg.Alerts.Ntfy == nil || cfg.Alerts.Email == nil || cfg.Alerts.SMS != nil {
		t.Fatalf("expected ntfy and email, got %s", describeAlerts(cfg.Alerts))
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("resulting settings are invalid: %v", err)
	}
}

func TestMergeAndRemoveHelpers(t *testing.T) {
	var a Alerts
	mergeAlerts(&a, Alerts{SMS: sms})
	mergeAlerts(&a, Alerts{Email: email})
	if got := describeAlerts(a); got != "SMS to +61412345678, email to dad@example.com" {
		t.Errorf("describeAlerts = %q", got)
	}
	removeAlert(&a, "sms")
	removeAlert(&a, "email")
	if describeAlerts(a) != "(nothing yet)" {
		t.Errorf("expected nothing left, got %q", describeAlerts(a))
	}
}
