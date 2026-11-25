package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestEmbeddedURLs(t *testing.T) {
	base, _ := url.Parse("https://example.com/events/list")
	body := `
<script>document.write('<iframe src="/from-script"></iframe>')</script>
<!-- <iframe src="/commented-out"></iframe> -->
<iframe src="/listings?a=1&amp;b=2" width="100%"></iframe>
<IFRAME SRC='https://other.example.org/table'></IFRAME>
<iframe src="about:blank" data-src="lazy.html"></iframe>
<iframe src=/listings?a=1&b=2#top></iframe>
<iframe src="https://example.com/events/list"></iframe>
<iframe src="https://www.youtube.com/embed/abc"></iframe>
<iframe src="https://www.google.com/maps/embed?pb=1"></iframe>
<iframe src="https://docs.google.com/spreadsheets/d/e/x/pubhtml"></iframe>
<iframe src="javascript:void(0)"></iframe>
<iframe></iframe>
<frame src="/frame.html">
`
	got := embeddedURLs(body, base)
	want := []string{
		"https://example.com/listings?a=1&b=2",
		"https://other.example.org/table",
		"https://example.com/events/lazy.html",
		"https://docs.google.com/spreadsheets/d/e/x/pubhtml",
		"https://example.com/frame.html",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("embeddedURLs =\n  %q\nwant\n  %q", got, want)
	}

	many := strings.Repeat(`<iframe src="/a"></iframe>`, 3)
	for i := 0; i < 10; i++ {
		many += fmt.Sprintf(`<iframe src="/p%d"></iframe>`, i)
	}
	if n := len(embeddedURLs(many, base)); n != maxEmbedded {
		t.Errorf("expected at most %d embedded pages, got %d", maxEmbedded, n)
	}
}

// embedSite serves an outer page that shows /inner inside it, like the
// cruise listings page that started this.
func embedSite(t *testing.T, inner *string, innerStatus *int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/outer", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<h1>Cruise listings</h1><p>See the listings below.</p><iframe src="/inner"></iframe>
<iframe src="/brochure.pdf"></iframe>`)
	})
	mux.HandleFunc("/inner", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(*innerStatus)
		fmt.Fprint(w, *inner)
	})
	mux.HandleFunc("/brochure.pdf", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		fmt.Fprint(w, "%PDF-1.4 bridge")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRunCheckFindsWordsInEmbeddedPage(t *testing.T) {
	inner, status := "<table><tr><td>Harbour cruise</td></tr></table>", 200
	srv := embedSite(t, &inner, &status)
	cfg := &Config{Pages: []Page{{URL: srv.URL + "/outer", Words: []string{"bridge"}}}}
	st := &State{Found: map[string]bool{}, Failures: map[string]int{}}
	var alerts []string
	alert := func(title, msg, link string) (int, error) { alerts = append(alerts, msg); return 1, nil }

	report := runCheck(cfg, st, alert)
	if len(alerts) != 0 {
		t.Fatalf("unexpected alert (a PDF must not count): %v", alerts)
	}
	if !strings.Contains(strings.Join(report, "\n"), "also checked 1 page shown inside it") {
		t.Errorf("report should mention the embedded page: %q", report)
	}

	inner = "<table><tr><td>Sydney Harbour Bridge cruise</td></tr></table>"
	runCheck(cfg, st, alert)
	if len(alerts) != 1 || !strings.Contains(alerts[0], `"bridge"`) || !strings.Contains(alerts[0], srv.URL+"/outer") {
		t.Fatalf("expected one alert naming the watched page, got %v", alerts)
	}
}

func TestRunCheckEmbeddedPageFailing(t *testing.T) {
	inner, status := "", 503
	srv := embedSite(t, &inner, &status)
	cfg := &Config{Pages: []Page{{URL: srv.URL + "/outer", Words: []string{"listings"}}}}
	st := &State{Found: map[string]bool{}, Failures: map[string]int{}}
	var titles []string
	alert := func(title, msg, link string) (int, error) { titles = append(titles, title); return 1, nil }

	var report []string
	for i := 0; i < failuresBeforeAlert+2; i++ {
		report = runCheck(cfg, st, alert)
	}
	// The outer page still works: its own words are found (one alert), and
	// the broken embedded page causes exactly one "not reachable" alert.
	if len(titles) != 2 || titles[1] != "pagewatch: page not reachable" {
		t.Fatalf("expected a word alert and one 'not reachable' alert, got %q", titles)
	}
	if !strings.Contains(strings.Join(report, "\n"), "which is shown inside") {
		t.Errorf("report should explain the embedded page problem: %q", report)
	}

	status, inner = 200, "ok"
	runCheck(cfg, st, alert)
	if st.Failures[srv.URL+"/inner"] != 0 {
		t.Error("failure count should reset once the embedded page opens again")
	}
}
