package main

import (
	"bufio"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ---------- asking questions ----------

type prompter struct{ r *bufio.Reader }

func newPrompter() *prompter { return &prompter{bufio.NewReader(os.Stdin)} }

func (p *prompter) line() string {
	s, err := p.r.ReadString('\n')
	if err != nil && (errors.Is(err, io.EOF) || s == "") {
		fmt.Println("\nNo more input; closing.")
		os.Exit(0)
	}
	return strings.TrimSpace(s)
}

// ask shows a question; an empty answer returns def.
func (p *prompter) ask(question, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", question, def)
	} else {
		fmt.Printf("%s: ", question)
	}
	if s := p.line(); s != "" {
		return s
	}
	return def
}

func (p *prompter) askValid(question, def string, valid func(string) (string, error)) string {
	for {
		v, err := valid(p.ask(question, def))
		if err == nil {
			return v
		}
		fmt.Println("  " + err.Error())
	}
}

func (p *prompter) yesNo(question string, def bool) bool {
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	for {
		fmt.Printf("%s (%s): ", question, hint)
		switch strings.ToLower(p.line()) {
		case "":
			return def
		case "y", "yes":
			return true
		case "n", "no":
			return false
		}
		fmt.Println("  Please type y or n.")
	}
}

func (p *prompter) choose(question string, options []string) int {
	fmt.Println(question)
	for i, o := range options {
		fmt.Printf("  %d) %s\n", i+1, o)
	}
	for {
		fmt.Print("Type a number: ")
		n, err := strconv.Atoi(p.line())
		if err == nil && n >= 1 && n <= len(options) {
			return n
		}
		fmt.Printf("  Please type a number from 1 to %d.\n", len(options))
	}
}

func (p *prompter) pressEnter(msg string) {
	fmt.Print(msg)
	_, _ = p.r.ReadString('\n')
}

func heading(s string) { fmt.Printf("\n=== %s ===\n\n", s) }

// ---------- main flow ----------

func interactive(forceSetup bool) {
	openConsole()
	p := newPrompter()
	defer p.pressEnter("\nPress Enter to close this window.")

	fmt.Printf("pagewatch %s — get an alert when words appear on a web page.\n", version)

	cfg, err := loadConfig()
	switch {
	case errors.Is(err, os.ErrNotExist):
		fmt.Println("\nWelcome! A few questions and you're done.")
		setupAndInstall(p, nil)
		return
	case err != nil:
		fmt.Printf("\nThere is a problem with your settings: %v\n(Settings file: %s)\n\n", err, configPath())
		if p.yesNo("Answer the setup questions again to fix it?", true) {
			setupAndInstall(p, nil)
		}
		return
	case forceSetup:
		setupAndInstall(p, cfg)
		return
	}

	// Opening pagewatch also refreshes the installed copy and the schedule,
	// so a newly downloaded version or a changed interval takes effect.
	if err := install(cfg); err != nil {
		fmt.Println("\nWarning:", err)
	}
	menu(p, cfg)
}

func setupAndInstall(p *prompter, existing *Config) {
	cfg, st := runSetup(p, existing)
	if cfg == nil {
		return
	}
	heading("Finishing up")
	if err := saveConfig(cfg); err != nil {
		fmt.Println("Could not save your settings:", err)
		return
	}
	if err := saveState(st); err != nil {
		fmt.Println("Could not save:", err)
	}
	if err := install(cfg); err != nil {
		fmt.Println("Problem:", err)
		return
	}
	logLine("Set up: " + describePages(cfg))

	fmt.Println("Running the first check now...")
	checkNow(cfg)

	fmt.Printf("\nAll set! pagewatch will check every %s while this computer is on.\n", minutesText(cfg.interval()))
	fmt.Println("You can close this window; checks happen in the background.")
	fmt.Println(reopenHint)
}

func checkNow(cfg *Config) {
	st := loadState()
	alert := func(title, msg, link string) (int, error) { return sendAlert(cfg.Alerts, title, msg, link) }
	for _, line := range runCheck(cfg, st, alert) {
		logLine(line)
		fmt.Println("  " + line)
	}
	if err := saveState(st); err != nil {
		fmt.Println("Could not save:", err)
	}
}

func menu(p *prompter, cfg *Config) {
	for {
		heading("pagewatch")
		fmt.Println("Watching:")
		for _, pg := range cfg.Pages {
			fmt.Printf("  %s\n    for %s\n", pg.URL, quoteList(cleanWords(pg.Words)))
		}
		fmt.Println("Alerts go to:", describeAlerts(cfg.Alerts))
		fmt.Printf("Checks every %s.", minutesText(cfg.interval()))
		if st := loadState(); !st.LastCheck.IsZero() {
			fmt.Printf(" Last check: %s.", st.LastCheck.Format("Mon 15:04"))
		}
		fmt.Println()
		fmt.Println("Settings file:", configPath())
		fmt.Println()

		switch p.choose("What would you like to do?", []string{
			"Check now",
			"Send a test alert",
			"Change settings (pages, words, alerts)",
			"Show recent activity",
			"Stop watching and remove pagewatch",
			"Exit",
		}) {
		case 1:
			fmt.Println("\nChecking...")
			checkNow(cfg)
		case 2:
			testAlert(cfg.Alerts)
		case 3:
			setupAndInstall(p, cfg)
			if c, err := loadConfig(); err == nil {
				cfg = c
			}
		case 4:
			showLog()
		case 5:
			if !p.yesNo("Stop all checks and remove pagewatch from this computer?", false) {
				continue
			}
			keep := p.yesNo("Keep your settings in case you set it up again later?", true)
			if err := uninstall(!keep); err != nil {
				fmt.Println("Problem:", err)
			} else {
				fmt.Println("Done. pagewatch will not run any more checks. You can delete the downloaded file too.")
			}
			return
		case 6:
			os.Exit(0) // skips the "press Enter" pause
		}
	}
}

func testAlert(a Alerts) bool {
	fmt.Println("\nSending a test alert...")
	n, err := sendAlert(a, "pagewatch test", "This is a test from pagewatch. Alerts will look like this.", "")
	if err != nil {
		fmt.Println("  Problem:", err)
	}
	if n > 0 {
		fmt.Println("  Sent. It should arrive within a minute.")
	}
	return n > 0 && err == nil
}

func showLog() {
	data, err := os.ReadFile(logPath())
	if err != nil {
		fmt.Println("\nNothing yet.")
		return
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) > 25 {
		lines = lines[len(lines)-25:]
	}
	fmt.Println("\nRecent activity (newest last):")
	for _, l := range lines {
		fmt.Println("  " + l)
	}
}

// ---------- setup questions ----------

func runSetup(p *prompter, existing *Config) (*Config, *State) {
	cfg := &Config{CheckEveryMinutes: defaultInterval}
	st := loadState()
	if existing != nil {
		cfg.CheckEveryMinutes = existing.CheckEveryMinutes
		cfg.WholeWords = existing.WholeWords
	}

	// Pages and words
	if existing != nil {
		heading("Pages")
		fmt.Println("You are watching:\n  " + describePages(existing))
	}
	if existing != nil && p.yesNo("Keep these pages and words?", true) {
		cfg.Pages = existing.Pages
	} else {
		for {
			heading(fmt.Sprintf("Page %d", len(cfg.Pages)+1))
			page, skipNow := askPage(p, cfg)
			cfg.Pages = append(cfg.Pages, page)
			for _, w := range skipNow {
				st.Found[stateKey(page.URL, w)] = true
			}
			if !p.yesNo("\nWatch another page too?", false) {
				break
			}
		}
	}

	// Alerts
	if existing != nil {
		heading("Alerts")
		fmt.Println("Alerts go to: " + describeAlerts(existing.Alerts))
		if p.yesNo("Keep these alert settings?", true) {
			cfg.Alerts = existing.Alerts
			return cfg, st
		}
	}
	for {
		cfg.Alerts = askAlerts(p)
		if testAlert(cfg.Alerts) && p.yesNo("Did the test alert arrive?", true) {
			break
		}
		fmt.Println("\nLet's try again. Check the details carefully (the tips above may help).")
	}
	return cfg, st
}

func askPage(p *prompter, cfg *Config) (Page, []string) {
	var page Page
	var text string
	for {
		page.URL = p.askValid("Which web page should I watch? (paste the address)", "", func(s string) (string, error) {
			s = strings.TrimSpace(s)
			if s == "" {
				return "", errors.New("Please paste the page address, for example https://example.com/news")
			}
			if !strings.Contains(s, "://") {
				s = "https://" + s
			}
			return s, validateURL(s)
		})
		fmt.Println("Opening the page to make sure it works...")
		t, err := fetchText(page.URL)
		if err == nil {
			fmt.Println("  OK, the page opened.")
			text = t
			break
		}
		fmt.Println("  Could not open it:", err)
		if p.yesNo("Use this address anyway?", false) {
			break
		}
	}

	page.Words = cleanWords(strings.Split(p.askValid("\nWhich words should I look for? (separate them with commas)", "", func(s string) (string, error) {
		if len(cleanWords(strings.Split(s, ","))) == 0 {
			return "", errors.New("Please type at least one word.")
		}
		return s, nil
	}), ","))

	if text == "" {
		return page, nil
	}
	present := findWords(text, page.Words, cfg.wholeWords())
	if len(present) == 0 {
		fmt.Println("\nNone of these words are on the page right now. You'll get an alert when one appears.")
		fmt.Println("(Tip: pagewatch reads the page's text without running its scripts. If a word you can see")
		fmt.Println(" in your browser is never found, the page may build its text with JavaScript.)")
		return page, nil
	}
	fmt.Printf("\nAlready on the page right now: %s\n", quoteList(present))
	if p.yesNo("Send an alert about these right away?", false) {
		return page, nil
	}
	fmt.Println("OK. You'll be alerted if they disappear and then come back.")
	return page, present
}

func askAlerts(p *prompter) Alerts {
	heading("How should I alert you?")
	switch p.choose("Pick one:", []string{
		"Phone notification with the free ntfy app (easiest, no account needed)",
		"Text message (SMS) through Twilio (needs an upgraded, paid Twilio account; a few cents per message)",
		"Email through Gmail",
	}) {
	case 1:
		return Alerts{Ntfy: askNtfy(p)}
	case 2:
		return Alerts{SMS: askTwilio(p)}
	default:
		return Alerts{Email: askEmail(p)}
	}
}

func askNtfy(p *prompter) *NtfyConfig {
	topic := "pagewatch-" + randomID(10)
	fmt.Println(`
1. On your phone, install the app called "ntfy" (App Store or Google Play).
2. Open it, tap "+" (Subscribe to topic) and type this topic name exactly:

       ` + topic + `

   Keep this name private: anyone who knows it can read your alerts.
`)
	p.pressEnter("Press Enter when you have subscribed...")
	return &NtfyConfig{Topic: topic}
}

var rePhone = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

func phoneNumber(s string) (string, error) {
	s = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", ".", "").Replace(s)
	if !rePhone.MatchString(s) {
		return "", errors.New("Use the international format: + then country code and number, e.g. +14155551234")
	}
	return s, nil
}

func askTwilio(p *prompter) *TwilioConfig {
	fmt.Print(`
You need an upgraded (paid) Twilio account (https://www.twilio.com/try-twilio):
  - A free trial account will NOT work: Twilio trials can only send Twilio's own
    preset messages, not pagewatch's alerts. Click "Upgrade" in the Twilio Console
    and add a payment method first. Texts then cost a few cents each.
  - In the Twilio Console home page, find your "Account SID" and "Auth Token".
  - Buy a phone number in the Console (Phone Numbers > Buy a number).
  - US numbers must be registered before they can send texts (Twilio guides you
    through "Toll-free verification" or "A2P 10DLC"); this can take a few days.

If you don't want to pay for Twilio, pick the free phone notification (ntfy) instead.
`)
	c := &TwilioConfig{}
	c.AccountSID = p.askValid("Account SID (starts with AC)", "", func(s string) (string, error) {
		if !strings.HasPrefix(s, "AC") || len(s) < 30 {
			return "", errors.New("The Account SID starts with AC and is 34 characters long.")
		}
		return s, nil
	})
	c.AuthToken = p.askValid("Auth Token", "", func(s string) (string, error) {
		if len(s) < 20 {
			return "", errors.New("That looks too short; copy the whole Auth Token.")
		}
		return s, nil
	})
	c.FromNumber = p.askValid("Your Twilio phone number (the one that sends)", "", phoneNumber)
	c.ToNumber = p.askValid("Your mobile number (the one that receives alerts)", "", phoneNumber)
	return c
}

func askEmail(p *prompter) *EmailConfig {
	fmt.Print(`
pagewatch sends email from a Gmail account. Gmail needs an "App Password":
  1. Turn on 2-Step Verification for the Google account, if it isn't already.
  2. Go to https://myaccount.google.com/apppasswords
  3. Create an app password named "pagewatch" and copy the 16 letters.
`)
	c := &EmailConfig{SMTPServer: "smtp.gmail.com", SMTPPort: 587}
	c.Username = p.askValid("Gmail address to send from", "", emailAddress)
	c.Password = strings.ReplaceAll(p.askValid("App Password (16 letters)", "", func(s string) (string, error) {
		if len(strings.ReplaceAll(s, " ", "")) < 8 {
			return "", errors.New("That looks too short.")
		}
		return s, nil
	}), " ", "")
	c.To = p.askValid("Send alerts to which address?", c.Username, emailAddress)
	return c
}

func emailAddress(s string) (string, error) {
	if !strings.Contains(s, "@") || !strings.Contains(s, ".") || strings.ContainsAny(s, " ,;<>") {
		return "", errors.New("That doesn't look like an email address.")
	}
	return s, nil
}

// ---------- helpers ----------

func randomID(n int) string {
	const letters = "abcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, n)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = letters[int(b[i])%len(letters)]
	}
	return string(b)
}

func describePages(c *Config) string {
	var parts []string
	for _, p := range c.Pages {
		parts = append(parts, p.URL+" for "+quoteList(cleanWords(p.Words)))
	}
	return strings.Join(parts, "\n  ")
}

func minutesText(m int) string {
	switch {
	case m == 60:
		return "hour"
	case m%60 == 0:
		return fmt.Sprintf("%d hours", m/60)
	default:
		return fmt.Sprintf("%d minutes", m)
	}
}
