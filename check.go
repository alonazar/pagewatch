package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// After this many failed checks in a row, send one alert that the page can't be opened.
const failuresBeforeAlert = 3

// State is what pagewatch remembers between checks (state.json).
type State struct {
	// Found records which words were on each page at the last check,
	// keyed by stateKey(url, word). An alert is sent only when a word
	// goes from "not there" to "there".
	Found      map[string]bool `json:"found"`
	Failures   map[string]int  `json:"failures"`
	LastCheck  time.Time       `json:"last_check"`
	LastResult string          `json:"last_result"`
}

func stateKey(url, word string) string { return url + "\n" + strings.ToLower(word) }

func loadState() *State {
	st := &State{}
	if data, err := os.ReadFile(statePath()); err == nil {
		_ = json.Unmarshal(data, st)
	}
	if st.Found == nil {
		st.Found = map[string]bool{}
	}
	if st.Failures == nil {
		st.Failures = map[string]int{}
	}
	return st
}

func saveState(st *State) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(appDir(), 0o700); err != nil {
		return err
	}
	return writeFileAtomic(statePath(), data, 0o600)
}

// alertFunc sends an alert and reports how many alert methods it reached.
type alertFunc func(title, message, link string) (int, error)

// runCheck checks every page once, sends alerts, updates st and returns
// a line per page describing what happened.
func runCheck(cfg *Config, st *State, alert alertFunc) []string {
	var report []string
	found := map[string]bool{}
	failures := map[string]int{}

	// failed counts a failure to open addr (the page itself, or a page
	// embedded in it), and sends one alert when it keeps happening.
	failed := func(addr, page string, err error) {
		failures[addr] = st.Failures[addr] + 1
		if addr == page {
			report = append(report, fmt.Sprintf("Could not open %s: %v", page, err))
		} else {
			report = append(report, fmt.Sprintf("  Could not open %s, which is shown inside %s: %v", addr, page, err))
		}
		if failures[addr] == failuresBeforeAlert {
			what := addr
			if addr != page {
				what = fmt.Sprintf("%s (part of %s)", addr, page)
			}
			msg := fmt.Sprintf("pagewatch has not been able to open %s for the last %d checks: %v. It will keep trying.",
				what, failuresBeforeAlert, err)
			if _, aerr := alert("pagewatch: page not reachable", msg, page); aerr != nil {
				report = append(report, "  Alert problem: "+aerr.Error())
			}
		}
	}

	for _, p := range cfg.Pages {
		words := cleanWords(p.Words)
		pc, err := fetchPage(p.URL)
		if err != nil {
			failed(p.URL, p.URL, err)
			for _, w := range words { // keep what we knew
				if st.Found[stateKey(p.URL, w)] {
					found[stateKey(p.URL, w)] = true
				}
			}
			continue
		}
		for _, prob := range pc.Problems {
			failed(prob.URL, p.URL, prob.Err)
		}
		text := pc.Text
		also := ""
		if n := len(pc.Embedded); n == 1 {
			also = " (also checked 1 page shown inside it)"
		} else if n > 1 {
			also = fmt.Sprintf(" (also checked %d pages shown inside it)", n)
		}

		present := findWords(text, words, cfg.wholeWords())
		var newlyFound []string
		for _, w := range present {
			if !st.Found[stateKey(p.URL, w)] {
				newlyFound = append(newlyFound, w)
			}
		}

		alerted := true
		if len(newlyFound) > 0 {
			msg := fmt.Sprintf("Found %s on %s", quoteList(newlyFound), p.URL)
			sent, aerr := alert("pagewatch found "+quoteList(newlyFound), msg, p.URL)
			if aerr != nil {
				report = append(report, "  Alert problem: "+aerr.Error())
			}
			// If no alert got through, don't remember these words, so we try again next time.
			alerted = sent > 0
		}

		isNew := map[string]bool{}
		for _, w := range newlyFound {
			isNew[w] = true
		}
		for _, w := range present {
			if !isNew[w] || alerted {
				found[stateKey(p.URL, w)] = true
			}
		}

		switch {
		case len(newlyFound) > 0 && alerted:
			report = append(report, fmt.Sprintf("%s: found %s, alert sent", p.URL, quoteList(newlyFound)))
		case len(newlyFound) > 0:
			report = append(report, fmt.Sprintf("%s: found %s, but the alert could not be sent (will retry)", p.URL, quoteList(newlyFound)))
		case len(present) > 0:
			report = append(report, fmt.Sprintf("%s: %s still there (already alerted)", p.URL, quoteList(present)))
		default:
			report = append(report, fmt.Sprintf("%s: none of the words are there%s", p.URL, also))
		}
	}

	st.Found = found
	st.Failures = failures
	st.LastCheck = time.Now()
	st.LastResult = strings.Join(report, "; ")
	return report
}

func quoteList(words []string) string {
	q := make([]string, len(words))
	for i, w := range words {
		q[i] = `"` + w + `"`
	}
	return strings.Join(q, ", ")
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

// pageContent is what a check reads from one page: its own text plus the
// text of the pages it embeds (iframes). A browser shows embedded pages as
// part of the page, so people expect their words to be found there too.
type pageContent struct {
	Text     string
	Embedded []string       // embedded pages that were read
	Problems []embedProblem // embedded pages that could not be read
}

type embedProblem struct {
	URL string
	Err error
}

// At most this many embedded pages are read for each page (one level deep).
const maxEmbedded = 5

// fetchPage downloads a page and the pages it embeds, and returns their text.
// Only the page itself failing is an error; problems with embedded pages are
// returned in Problems.
func fetchPage(pageURL string) (*pageContent, error) {
	body, final, _, err := fetchHTML(pageURL)
	if err != nil {
		return nil, err
	}
	pc := &pageContent{}
	texts := []string{htmlToText(body)}
	for _, u := range embeddedURLs(body, final) {
		eb, _, ctype, err := fetchHTML(u)
		if err != nil {
			pc.Problems = append(pc.Problems, embedProblem{u, err})
			continue
		}
		if ctype != "" && !strings.Contains(ctype, "html") && !strings.HasPrefix(ctype, "text/") {
			continue // an embedded PDF, image or similar: no text to read
		}
		pc.Embedded = append(pc.Embedded, u)
		texts = append(texts, htmlToText(eb))
	}
	pc.Text = strings.Join(texts, " ")
	return pc, nil
}

// fetchHTML downloads a page and returns its body, its final address (after
// redirects) and its content type.
func fetchHTML(pageURL string) (body string, final *url.URL, contentType string, err error) {
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return "", nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; pagewatch/"+version+")")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", nil, "", simplifyNetError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", nil, "", fmt.Errorf("the website answered with error %d (%s)", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return "", nil, "", simplifyNetError(err)
	}
	return string(data), resp.Request.URL, strings.ToLower(resp.Header.Get("Content-Type")), nil
}

var (
	reFrameTag  = regexp.MustCompile(`(?is)<i?frame\b[^>]*>`)
	reFrameAttr = regexp.MustCompile(`(?is)\s(src|data-src)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+))`)
)

// embeddedURLs finds the pages a page embeds with <iframe> or <frame>,
// as full addresses. Videos, maps, ads, social media widgets and the like
// are left out: they are not part of the page's own content.
func embeddedURLs(body string, base *url.URL) []string {
	body = reHidden.ReplaceAllString(body, " ")
	seen := map[string]bool{}
	if base != nil {
		seen[base.String()] = true
	}
	var out []string
	for _, tag := range reFrameTag.FindAllString(body, -1) {
		attrs := map[string]string{}
		for _, m := range reFrameAttr.FindAllStringSubmatch(tag, -1) {
			attrs[strings.ToLower(m[1])] = strings.TrimSpace(html.UnescapeString(m[2] + m[3] + m[4]))
		}
		src := attrs["src"]
		if src == "" || strings.HasPrefix(strings.ToLower(src), "about:") {
			src = attrs["data-src"] // pages that load their frames later
		}
		if src == "" {
			continue
		}
		u, err := url.Parse(src)
		if err != nil {
			continue
		}
		if base != nil {
			u = base.ResolveReference(u)
		}
		u.Fragment = ""
		if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || notContent(u) || seen[u.String()] {
			continue
		}
		seen[u.String()] = true
		out = append(out, u.String())
		if len(out) == maxEmbedded {
			break
		}
	}
	return out
}

// Embedded pages that are never the content people are watching for.
var notContentHosts = []string{
	"youtube.com", "youtube-nocookie.com", "youtu.be", "vimeo.com", "dailymotion.com",
	"facebook.com", "instagram.com", "twitter.com", "x.com", "tiktok.com", "linkedin.com",
	"spotify.com", "soundcloud.com", "doubleclick.net", "googlesyndication.com",
	"googletagmanager.com", "recaptcha.net", "hcaptcha.com", "challenges.cloudflare.com",
	"maps.google.com",
}

func notContent(u *url.URL) bool {
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	for _, h := range notContentHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	// Google Maps and reCAPTCHA live on google.com; Google Docs and Sheets
	// (docs.google.com) can hold real content, so they are kept.
	if host == "google.com" || strings.HasPrefix(host, "google.") {
		return strings.HasPrefix(u.Path, "/maps") || strings.HasPrefix(u.Path, "/recaptcha")
	}
	return false
}

func simplifyNetError(err error) error {
	s := err.Error()
	switch {
	case strings.Contains(s, "no such host"):
		return fmt.Errorf("the website address was not found (check the address and your internet connection)")
	case strings.Contains(s, "Client.Timeout") || strings.Contains(s, "timeout"):
		return fmt.Errorf("the website took too long to answer")
	}
	return err
}

var (
	reHidden = regexp.MustCompile(`(?is)<script\b.*?</script\s*>|<style\b.*?</style\s*>|<!--.*?-->`)
	reTag    = regexp.MustCompile(`(?s)<[^>]*>`)
	reSpace  = regexp.MustCompile(`\s+`)
)

// htmlToText turns HTML into the text a person would read, roughly:
// scripts, styles, comments and tags are removed and entities decoded.
func htmlToText(s string) string {
	s = reHidden.ReplaceAllString(s, " ")
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.TrimSpace(reSpace.ReplaceAllString(s, " "))
}

// findWords returns the words that appear in text, ignoring case.
func findWords(text string, words []string, wholeWords bool) []string {
	lower := strings.ToLower(text)
	var out []string
	for _, w := range words {
		needle := strings.ToLower(reSpace.ReplaceAllString(strings.TrimSpace(w), " "))
		if needle != "" && contains(lower, needle, wholeWords) {
			out = append(out, w)
		}
	}
	return out
}

func isWordChar(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

// contains reports whether needle is in text. With wholeWords, a match
// must not be part of a longer word ("sale" doesn't match "wholesale").
// Works for any language, unlike regexp's \b which only knows ASCII.
func contains(text, needle string, wholeWords bool) bool {
	if !wholeWords {
		return strings.Contains(text, needle)
	}
	first, _ := utf8.DecodeRuneInString(needle)
	last, _ := utf8.DecodeLastRuneInString(needle)
	for start := 0; start <= len(text); {
		i := strings.Index(text[start:], needle)
		if i < 0 {
			return false
		}
		i += start
		end := i + len(needle)
		before, _ := utf8.DecodeLastRuneInString(text[:i])
		after, _ := utf8.DecodeRuneInString(text[end:])
		okBefore := i == 0 || !isWordChar(first) || !isWordChar(before)
		okAfter := end == len(text) || !isWordChar(last) || !isWordChar(after)
		if okBefore && okAfter {
			return true
		}
		_, size := utf8.DecodeRuneInString(text[i:])
		start = i + size
	}
	return false
}
