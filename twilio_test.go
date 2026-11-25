package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestSMSCutsOnLetterBoundary checks that long messages are shortened
// without breaking a non-English letter in half. Runs offline.
func TestSMSCutsOnLetterBoundary(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got = r.PostForm.Get("Body")
		w.WriteHeader(201)
	}))
	defer srv.Close()
	old := twilioAPI
	twilioAPI = srv.URL
	defer func() { twilioAPI = old }()

	if err := sendSMS(&TwilioConfig{"AC1", "t", "+15005550006", "+14155550100"}, strings.Repeat("x", 999)+"מבצע"); err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(got) || got != strings.Repeat("x", 999) {
		t.Fatalf("message was cut badly: valid=%v len=%d", utf8.ValidString(got), len(got))
	}
}

// TestSMSTrialErrorsExplained checks that the errors free trial accounts
// get are shown in plain words. Runs offline.
func TestSMSTrialErrorsExplained(t *testing.T) {
	code := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprintf(w, `{"code": %d, "message": "something technical", "status": 400}`, code)
	}))
	defer srv.Close()
	old := twilioAPI
	twilioAPI = srv.URL
	defer func() { twilioAPI = old }()

	cfg := &TwilioConfig{"AC1", "t", "+15005550006", "+14155550100"}
	for c, want := range map[int]string{
		572006: "upgrade your Twilio account",
		572002: "Verified Caller IDs",
		21608:  "Verified Caller IDs",
		21211:  "Twilio said: something technical (code 21211)",
	} {
		code = c
		err := sendSMS(cfg, "hi")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("code %d: got %v, want it to mention %q", c, err, want)
		}
	}
}

// TestTwilioRealSMS sends ONE real text with your live Twilio credentials.
// On a trial account, the "to" number must be verified in the Twilio Console.
// Runs only when PAGEWATCH_TWILIO_SID, _TOKEN, _FROM and _TO are all set.
func TestTwilioRealSMS(t *testing.T) {
	c := TwilioConfig{
		AccountSID: os.Getenv("PAGEWATCH_TWILIO_SID"),
		AuthToken:  os.Getenv("PAGEWATCH_TWILIO_TOKEN"),
		FromNumber: os.Getenv("PAGEWATCH_TWILIO_FROM"),
		ToNumber:   os.Getenv("PAGEWATCH_TWILIO_TO"),
	}
	if c.AccountSID == "" || c.AuthToken == "" || c.FromNumber == "" || c.ToNumber == "" {
		t.Skip("set PAGEWATCH_TWILIO_SID, _TOKEN, _FROM and _TO to send one real text")
	}
	var err error
	if c.FromNumber, err = phoneNumber(c.FromNumber); err != nil {
		t.Fatal("from number:", err)
	}
	if c.ToNumber, err = phoneNumber(c.ToNumber); err != nil {
		t.Fatal("to number:", err)
	}
	if err := sendSMS(&c, `pagewatch test: found "tickets" on https://example.com`); err != nil {
		t.Fatalf("pagewatch would show: %v", err)
	}
	t.Log("Twilio accepted the message. Check your phone.")
}

// TestTwilioLive sends real requests to Twilio using your Twilio *test*
// credentials (Console > Account > API keys & tokens > Test credentials).
// Test credentials never send a real text and cost nothing. Twilio's
// "magic" phone numbers make it succeed or fail on purpose.
//
// Runs only when PAGEWATCH_TWILIO_SID and PAGEWATCH_TWILIO_TOKEN are set.
func TestTwilioLive(t *testing.T) {
	sid, token := os.Getenv("PAGEWATCH_TWILIO_SID"), os.Getenv("PAGEWATCH_TWILIO_TOKEN")
	if sid == "" || token == "" {
		t.Skip("set PAGEWATCH_TWILIO_SID and PAGEWATCH_TWILIO_TOKEN (test credentials) to run")
	}
	const (
		goodFrom  = "+15005550006" // magic: a valid sending number
		badFrom   = "+15005550001" // magic: invalid sending number
		goodTo    = "+14155550100" // any normal number works with test credentials
		invalidTo = "+15005550001" // magic: invalid destination
		notMobile = "+15005550009" // magic: can't receive texts
	)

	cases := []struct {
		name    string
		cfg     TwilioConfig
		wantErr string // "" means success
	}{
		{"message accepted", TwilioConfig{sid, token, goodFrom, goodTo}, ""},
		{"Hebrew and long text", TwilioConfig{sid, token, goodFrom, goodTo}, ""},
		{"invalid destination", TwilioConfig{sid, token, goodFrom, invalidTo}, "21211"},
		{"number can't receive texts", TwilioConfig{sid, token, goodFrom, notMobile}, "21614"},
		{"invalid sending number", TwilioConfig{sid, token, badFrom, goodTo}, "21212"},
		{"wrong auth token", TwilioConfig{sid, "0000000000000000000000000000000x", goodFrom, goodTo}, "did not accept"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg := `Found "tickets" on https://example.com`
			if strings.Contains(c.name, "Hebrew") {
				// The 1000-byte cut lands in the middle of a Hebrew letter.
				msg = strings.Repeat("x", 999) + "מבצע חדש"
			}
			err := sendSMS(&c.cfg, msg)
			switch {
			case c.wantErr == "" && err != nil:
				t.Fatalf("expected success, got: %v", err)
			case c.wantErr != "" && err == nil:
				t.Fatalf("expected an error containing %q, got success", c.wantErr)
			case c.wantErr != "" && !strings.Contains(err.Error(), c.wantErr):
				t.Fatalf("expected an error containing %q, got: %v", c.wantErr, err)
			}
			if err != nil {
				t.Logf("pagewatch would show: %v", err)
			}
		})
	}
}
