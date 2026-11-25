package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultInterval = 60
	minInterval     = 5
	maxInterval     = 24 * 60
)

// Config is the content of config.yaml.
type Config struct {
	CheckEveryMinutes int    `yaml:"check_every_minutes"`
	WholeWords        *bool  `yaml:"whole_words"`
	Pages             []Page `yaml:"pages"`
	Alerts            Alerts `yaml:"alerts"`
}

type Page struct {
	URL   string   `yaml:"url"`
	Words []string `yaml:"words"`
}

type Alerts struct {
	Ntfy     *NtfyConfig     `yaml:"ntfy,omitempty"`
	SMS      *TwilioConfig   `yaml:"sms,omitempty"`
	WhatsApp *WhatsAppConfig `yaml:"whatsapp,omitempty"`
	Email    *EmailConfig    `yaml:"email,omitempty"`
}

type WhatsAppConfig struct {
	AccountSID string `yaml:"account_sid"`
	AuthToken  string `yaml:"auth_token"`
	FromNumber string `yaml:"from_number"`
	ToNumber   string `yaml:"to_number"`
	// TemplateSID is the approved message template's Content SID (HX...).
	// Empty means free text, which only works with Twilio's WhatsApp Sandbox.
	TemplateSID string `yaml:"template_sid,omitempty"`
}

type NtfyConfig struct {
	Topic  string `yaml:"topic"`
	Server string `yaml:"server,omitempty"`
}

type TwilioConfig struct {
	AccountSID string `yaml:"account_sid"`
	AuthToken  string `yaml:"auth_token"`
	FromNumber string `yaml:"from_number"`
	ToNumber   string `yaml:"to_number"`
}

type EmailConfig struct {
	SMTPServer string `yaml:"smtp_server"`
	SMTPPort   int    `yaml:"smtp_port"`
	Username   string `yaml:"username"`
	Password   string `yaml:"password"`
	To         string `yaml:"send_to"`
}

func (c *Config) wholeWords() bool { return c.WholeWords == nil || *c.WholeWords }

func (c *Config) interval() int {
	if c.CheckEveryMinutes == 0 {
		return defaultInterval
	}
	return c.CheckEveryMinutes
}

// appDir is where pagewatch keeps its settings, memory and log:
// %AppData%\pagewatch on Windows, ~/Library/Application Support/pagewatch on a Mac.
func appDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d, _ = os.UserHomeDir()
	}
	return filepath.Join(d, "pagewatch")
}

func configPath() string { return filepath.Join(appDir(), "config.yaml") }
func statePath() string  { return filepath.Join(appDir(), "state.json") }
func logPath() string    { return filepath.Join(appDir(), "pagewatch.log") }

func loadConfig() (*Config, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("the settings file could not be read: %v", err)
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) validate() error {
	if n := c.interval(); n < minInterval || n > maxInterval {
		return fmt.Errorf("check_every_minutes must be between %d and %d", minInterval, maxInterval)
	}
	if len(c.Pages) == 0 {
		return errors.New("no pages to watch")
	}
	for i, p := range c.Pages {
		if err := validateURL(p.URL); err != nil {
			return fmt.Errorf("page %d: %v", i+1, err)
		}
		if len(cleanWords(p.Words)) == 0 {
			return fmt.Errorf("page %d (%s) has no words to look for", i+1, p.URL)
		}
	}
	a := c.Alerts
	if a.Ntfy == nil && a.SMS == nil && a.WhatsApp == nil && a.Email == nil {
		return errors.New("no alert method is set up (ntfy, sms, whatsapp or email)")
	}
	if a.Ntfy != nil && strings.TrimSpace(a.Ntfy.Topic) == "" {
		return errors.New("ntfy: topic is empty")
	}
	if s := a.SMS; s != nil {
		if s.AccountSID == "" || s.AuthToken == "" || s.FromNumber == "" || s.ToNumber == "" {
			return errors.New("sms: account_sid, auth_token, from_number and to_number are all needed")
		}
	}
	if w := a.WhatsApp; w != nil {
		if w.AccountSID == "" || w.AuthToken == "" || w.FromNumber == "" || w.ToNumber == "" {
			return errors.New("whatsapp: account_sid, auth_token, from_number and to_number are all needed")
		}
		if w.TemplateSID != "" && !strings.HasPrefix(w.TemplateSID, "HX") {
			return errors.New("whatsapp: template_sid should start with HX (or be left out to use the Sandbox)")
		}
	}
	if e := a.Email; e != nil {
		if e.SMTPServer == "" || e.SMTPPort == 0 || e.Username == "" || e.Password == "" || e.To == "" {
			return errors.New("email: smtp_server, smtp_port, username, password and send_to are all needed")
		}
	}
	return nil
}

func validateURL(s string) error {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%q is not a web address (it should start with https://)", s)
	}
	return nil
}

// cleanWords trims the words and drops empty ones.
func cleanWords(words []string) []string {
	var out []string
	for _, w := range words {
		if w = strings.TrimSpace(w); w != "" {
			out = append(out, w)
		}
	}
	return out
}

// q quotes a value for YAML. A JSON string is a valid YAML double-quoted string.
func q(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// saveConfig writes config.yaml with comments, so it is easy to edit by hand.
func saveConfig(c *Config) error {
	var b strings.Builder
	b.WriteString("# pagewatch settings. You can edit this file with any text editor.\n")
	b.WriteString("# Changes to pages, words and alerts apply at the next check.\n")
	b.WriteString("# After changing check_every_minutes, open pagewatch once so the schedule is updated.\n\n")

	b.WriteString("# How often to check, in minutes (5 to 1440).\n")
	fmt.Fprintf(&b, "check_every_minutes: %d\n\n", c.interval())

	b.WriteString("# true: \"sale\" does not match \"wholesale\". false: match the word anywhere,\n")
	b.WriteString("# which is useful for languages that attach letters to words (like Hebrew or Arabic).\n")
	fmt.Fprintf(&b, "whole_words: %t\n\n", c.wholeWords())

	b.WriteString("# The pages to watch and the words to look for on each. Upper/lower case does not matter.\n")
	b.WriteString("pages:\n")
	for _, p := range c.Pages {
		fmt.Fprintf(&b, "  - url: %s\n    words:\n", q(p.URL))
		for _, w := range cleanWords(p.Words) {
			fmt.Fprintf(&b, "      - %s\n", q(w))
		}
	}

	b.WriteString("\n# Where alerts go. You can have more than one.\nalerts:\n")
	if n := c.Alerts.Ntfy; n != nil {
		b.WriteString("  # Phone notification through the free ntfy app. Keep the topic private.\n")
		fmt.Fprintf(&b, "  ntfy:\n    topic: %s\n", q(n.Topic))
		if n.Server != "" {
			fmt.Fprintf(&b, "    server: %s\n", q(n.Server))
		}
	}
	if s := c.Alerts.SMS; s != nil {
		b.WriteString("  # Text message through Twilio. Phone numbers start with + and the country code.\n")
		fmt.Fprintf(&b, "  sms:\n    account_sid: %s\n    auth_token: %s\n    from_number: %s\n    to_number: %s\n",
			q(s.AccountSID), q(s.AuthToken), q(s.FromNumber), q(s.ToNumber))
	}
	if w := c.Alerts.WhatsApp; w != nil {
		b.WriteString("  # WhatsApp through Twilio. template_sid is your approved template's Content SID (HX...).\n")
		b.WriteString("  # The template text should be: " + WhatsAppTemplate + "\n")
		fmt.Fprintf(&b, "  whatsapp:\n    account_sid: %s\n    auth_token: %s\n    from_number: %s\n    to_number: %s\n",
			q(w.AccountSID), q(w.AuthToken), q(w.FromNumber), q(w.ToNumber))
		if w.TemplateSID != "" {
			fmt.Fprintf(&b, "    template_sid: %s\n", q(w.TemplateSID))
		}
	}
	if e := c.Alerts.Email; e != nil {
		b.WriteString("  # Email. For Gmail, password is an App Password, not your normal password.\n")
		fmt.Fprintf(&b, "  email:\n    smtp_server: %s\n    smtp_port: %d\n    username: %s\n    password: %s\n    send_to: %s\n",
			q(e.SMTPServer), e.SMTPPort, q(e.Username), q(e.Password), q(e.To))
	}

	if err := os.MkdirAll(appDir(), 0o700); err != nil {
		return err
	}
	return writeFileAtomic(configPath(), []byte(b.String()), 0o600)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
