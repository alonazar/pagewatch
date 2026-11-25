package main

import (
	"fmt"
	"strings"
)

// scheduleInfo is what the computer's scheduler says about pagewatch.
type scheduleInfo struct {
	Installed bool
	Minutes   int // 0 if unknown
}

func currentSchedule() scheduleInfo {
	s := scheduleInfo{Installed: scheduleInstalled()}
	if s.Installed {
		s.Minutes = scheduledInterval()
	}
	return s
}

// settingsReport describes the current settings for the menu's "Show
// current settings". Passwords and auth tokens are never shown, only
// whether they are filled in.
func settingsReport(cfg *Config, st *State, sched scheduleInfo) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	b.WriteString("\n=== Current settings ===\n\n")
	line("pagewatch version: %s", version)

	line("\nPages and words:")
	for i, p := range cfg.Pages {
		line("  %d. %s", i+1, p.URL)
		line("     looking for %s", quoteList(cleanWords(p.Words)))
	}
	if cfg.wholeWords() {
		line("  Matching: whole words only (\"sale\" does not match \"wholesale\")")
	} else {
		line("  Matching: anywhere in the text (whole_words: false)")
	}

	line("\nChecking:")
	line("  Every %s", minutesText(cfg.interval()))
	switch {
	case !sched.Installed:
		line("  Schedule: NOT set up on this computer. Open pagewatch again to fix it.")
	case sched.Minutes != 0 && sched.Minutes != cfg.interval():
		line("  Schedule: set up, but runs every %s. Open pagewatch again to update it.", minutesText(sched.Minutes))
	default:
		line("  Schedule: set up and running in the background")
	}
	if st != nil && !st.LastCheck.IsZero() {
		line("  Last check: %s", st.LastCheck.Format("Mon 2 Jan 15:04"))
		if st.LastResult != "" {
			line("  Result: %s", st.LastResult)
		}
	} else {
		line("  Last check: none yet")
	}

	line("\nAlerts:")
	a := cfg.Alerts
	if n := a.Ntfy; n != nil {
		line("  Phone notification (ntfy)")
		line("    Topic:  %s", n.Topic)
		if n.Server != "" {
			line("    Server: %s", n.Server)
		}
	}
	if s := a.SMS; s != nil {
		line("  Text message (SMS) through Twilio")
		line("    From:        %s", s.FromNumber)
		line("    To:          %s", s.ToNumber)
		line("    Account SID: %s", maskID(s.AccountSID))
		line("    Auth Token:  %s", secretStatus(s.AuthToken))
	}
	if w := a.WhatsApp; w != nil {
		line("  WhatsApp through Twilio")
		line("    From:        %s", w.FromNumber)
		line("    To:          %s", w.ToNumber)
		line("    Account SID: %s", maskID(w.AccountSID))
		line("    Auth Token:  %s", secretStatus(w.AuthToken))
		if w.TemplateSID != "" {
			line("    Template:    %s", w.TemplateSID)
		} else {
			line("    Template:    none (Twilio WhatsApp Sandbox)")
		}
	}
	if e := a.Email; e != nil {
		line("  Email")
		line("    To:          %s", e.To)
		line("    Sent from:   %s", e.Username)
		line("    Mail server: %s, port %d", e.SMTPServer, e.SMTPPort)
		line("    Password:    %s", secretStatus(e.Password))
	}

	line("\nFiles:")
	line("  Settings: %s", configPath())
	line("  Activity: %s", logPath())
	line("  Program:  %s", installedExe())
	return b.String()
}

// secretStatus says whether a secret is filled in, without showing it.
func secretStatus(s string) string {
	if strings.TrimSpace(s) == "" {
		return "missing"
	}
	return "set (hidden)"
}

// maskID shows only the start and end of an account ID, e.g. "ACb8…7194".
func maskID(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:4] + "…" + s[len(s)-4:]
}
