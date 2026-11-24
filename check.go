package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
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

	for _, p := range cfg.Pages {
		words := cleanWords(p.Words)
		text, err := fetchText(p.URL)
		if err != nil {
			failures[p.URL] = st.Failures[p.URL] + 1
			for _, w := range words { // keep what we knew
				if st.Found[stateKey(p.URL, w)] {
					found[stateKey(p.URL, w)] = true
				}
			}
			report = append(report, fmt.Sprintf("Could not open %s: %v", p.URL, err))
			if failures[p.URL] == failuresBeforeAlert {
				msg := fmt.Sprintf("pagewatch has not been able to open %s for the last %d checks: %v. It will keep trying.",
					p.URL, failuresBeforeAlert, err)
				if _, aerr := alert("pagewatch: page not reachable", msg, p.URL); aerr != nil {
					report = append(report, "  Alert problem: "+aerr.Error())
				}
			}
			continue
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
			report = append(report, fmt.Sprintf("%s: none of the words are there", p.URL))
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

// fetchText downloads a page and returns its readable text.
func fetchText(pageURL string) (string, error) {
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; pagewatch/"+version+")")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", simplifyNetError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("the website answered with error %d (%s)", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return "", simplifyNetError(err)
	}
	return htmlToText(string(body)), nil
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
