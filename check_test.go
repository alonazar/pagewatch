package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestHTMLToText(t *testing.T) {
	in := `<html><head><style>.sale{}</style><script>var x="sale"</script></head>
<body><!-- sale --><h1>Big&nbsp;News</h1><p>Tickets &amp; more</p></body></html>`
	got := htmlToText(in)
	want := "Big News Tickets & more"
	if got != want {
		t.Fatalf("htmlToText = %q, want %q", got, want)
	}
}

func TestFindWords(t *testing.T) {
	text := "Wholesale prices! Tickets AVAILABLE now. מבצע חדש. Price: $5."
	cases := []struct {
		words []string
		whole bool
		want  []string
	}{
		{[]string{"sale"}, true, nil},
		{[]string{"sale"}, false, []string{"sale"}},
		{[]string{"available", "sold out"}, true, []string{"available"}},
		{[]string{"tickets  available"}, true, []string{"tickets  available"}},
		{[]string{"מבצע"}, true, []string{"מבצע"}},
		{[]string{"מבצ"}, true, nil},
		{[]string{"$5"}, true, []string{"$5"}},
		{[]string{"  "}, true, nil},
	}
	for _, c := range cases {
		got := findWords(text, c.words, c.whole)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("findWords(%q, whole=%v) = %q, want %q", c.words, c.whole, got, c.want)
		}
	}
}

func TestRunCheckAlertsOnlyOnChange(t *testing.T) {
	body := "<p>nothing yet</p>"
	status := 200
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	cfg := &Config{Pages: []Page{{URL: srv.URL, Words: []string{"tickets"}}}}
	st := &State{Found: map[string]bool{}, Failures: map[string]int{}}
	var alerts []string
	failSend := false
	alert := func(title, msg, link string) (int, error) {
		if failSend {
			return 0, errors.New("offline")
		}
		alerts = append(alerts, msg)
		return 1, nil
	}

	runCheck(cfg, st, alert)
	if len(alerts) != 0 {
		t.Fatalf("unexpected alert: %v", alerts)
	}

	body = "<p>Tickets on sale</p>"
	runCheck(cfg, st, alert)
	if len(alerts) != 1 || !strings.Contains(alerts[0], `"tickets"`) {
		t.Fatalf("expected one alert, got %v", alerts)
	}

	runCheck(cfg, st, alert) // still there: no new alert
	if len(alerts) != 1 {
		t.Fatalf("repeated alert: %v", alerts)
	}

	// A failed fetch keeps the memory, so recovery doesn't re-alert.
	status = 500
	runCheck(cfg, st, alert)
	status = 200
	runCheck(cfg, st, alert)
	if len(alerts) != 1 {
		t.Fatalf("re-alerted after an error: %v", alerts)
	}

	// Disappears, then comes back while alerts fail: retried next time.
	body = "<p>gone</p>"
	runCheck(cfg, st, alert)
	body = "<p>tickets!</p>"
	failSend = true
	runCheck(cfg, st, alert)
	failSend = false
	runCheck(cfg, st, alert)
	if len(alerts) != 2 {
		t.Fatalf("expected retry alert, got %v", alerts)
	}
}

func TestRunCheckAlertsAfterRepeatedFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()
	cfg := &Config{Pages: []Page{{URL: srv.URL, Words: []string{"x"}}}}
	st := &State{Found: map[string]bool{}, Failures: map[string]int{}}
	n := 0
	alert := func(title, msg, link string) (int, error) { n++; return 1, nil }
	for i := 0; i < failuresBeforeAlert+2; i++ {
		runCheck(cfg, st, alert)
	}
	if n != 1 {
		t.Fatalf("expected exactly one 'not reachable' alert, got %d", n)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())         // Windows
	t.Setenv("HOME", t.TempDir())            // Mac
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // Linux
	f := false
	in := &Config{
		CheckEveryMinutes: 30,
		WholeWords:        &f,
		Pages:             []Page{{URL: "https://example.com/a?b=\"c\"", Words: []string{"sold out", "מבצע", "a: b"}}},
		Alerts: Alerts{
			Ntfy: &NtfyConfig{Topic: "pagewatch-abc"},
			SMS:  &TwilioConfig{AccountSID: "AC1", AuthToken: "t", FromNumber: "+1555", ToNumber: "+1666"},
		},
	}
	if err := saveConfig(in); err != nil {
		t.Fatal(err)
	}
	out, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip mismatch:\n in: %+v\nout: %+v", in, out)
	}
}
