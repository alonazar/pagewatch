package main

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// sendAlert sends the alert to every method that is set up. It returns how
// many succeeded, and an error describing any that failed.
func sendAlert(a Alerts, title, message, link string) (int, error) {
	sent := 0
	var errs []error
	try := func(name string, f func() error) {
		if err := f(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		} else {
			sent++
		}
	}
	if a.Ntfy != nil {
		try("phone notification (ntfy)", func() error { return sendNtfy(a.Ntfy, title, message, link) })
	}
	if a.SMS != nil {
		try("SMS (Twilio)", func() error { return sendSMS(a.SMS, message) })
	}
	if a.WhatsApp != nil {
		try("WhatsApp (Twilio)", func() error { return sendWhatsApp(a.WhatsApp, message) })
	}
	if a.Email != nil {
		try("email", func() error { return sendEmail(a.Email, title, message) })
	}
	return sent, errors.Join(errs...)
}

// sendAlertFn is what the menu and setup use to send alerts; tests replace it.
var sendAlertFn = sendAlert

type alertEntry struct {
	kind string // "ntfy", "sms", "whatsapp" or "email"
	desc string
}

// alertEntries lists the alert methods that are set up, in a fixed order.
func alertEntries(a Alerts) []alertEntry {
	var out []alertEntry
	if a.Ntfy != nil {
		out = append(out, alertEntry{"ntfy", "phone notification (ntfy topic " + a.Ntfy.Topic + ")"})
	}
	if a.SMS != nil {
		out = append(out, alertEntry{"sms", "SMS to " + a.SMS.ToNumber})
	}
	if a.WhatsApp != nil {
		out = append(out, alertEntry{"whatsapp", "WhatsApp to " + a.WhatsApp.ToNumber})
	}
	if a.Email != nil {
		out = append(out, alertEntry{"email", "email to " + a.Email.To})
	}
	return out
}

func describeAlerts(a Alerts) string {
	var parts []string
	for _, e := range alertEntries(a) {
		parts = append(parts, e.desc)
	}
	if len(parts) == 0 {
		return "(nothing yet)"
	}
	return strings.Join(parts, ", ")
}

// mergeAlerts adds every method set up in src to dst, replacing the same kind.
func mergeAlerts(dst *Alerts, src Alerts) {
	if src.Ntfy != nil {
		dst.Ntfy = src.Ntfy
	}
	if src.SMS != nil {
		dst.SMS = src.SMS
	}
	if src.WhatsApp != nil {
		dst.WhatsApp = src.WhatsApp
	}
	if src.Email != nil {
		dst.Email = src.Email
	}
}

func removeAlert(a *Alerts, kind string) {
	switch kind {
	case "ntfy":
		a.Ntfy = nil
	case "sms":
		a.SMS = nil
	case "whatsapp":
		a.WhatsApp = nil
	case "email":
		a.Email = nil
	}
}

func sendNtfy(c *NtfyConfig, title, message, link string) error {
	server := strings.TrimRight(c.Server, "/")
	if server == "" {
		server = "https://ntfy.sh"
	}
	payload := map[string]any{
		"topic":    c.Topic,
		"title":    title,
		"message":  message,
		"priority": 4,
		"tags":     []string{"mag"},
	}
	if link != "" {
		payload["click"] = link
	}
	body, _ := json.Marshal(payload)
	resp, err := httpClient.Post(server, "application/json", bytes.NewReader(body))
	if err != nil {
		return simplifyNetError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2000))
		return fmt.Errorf("ntfy answered %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// twilioAPI is a variable so tests can point it elsewhere.
var twilioAPI = "https://api.twilio.com"

// truncate shortens s to at most n bytes without cutting a letter in half.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

func sendSMS(c *TwilioConfig, message string) error {
	form := url.Values{"To": {c.ToNumber}, "From": {c.FromNumber}, "Body": {truncate(message, 1000)}}
	return twilioSend(c.AccountSID, c.AuthToken, form)
}

// WhatsAppTemplate is the message template users create in Twilio and get
// approved by Meta. Its one placeholder receives the alert text. (Meta rejects
// templates that start or end with a placeholder.)
const WhatsAppTemplate = "pagewatch alert: {{1}}. Open the page to see more."

var reNoTemplateSpace = regexp.MustCompile(`\s+`)

func sendWhatsApp(c *WhatsAppConfig, message string) error {
	form := url.Values{"To": {"whatsapp:" + c.ToNumber}, "From": {"whatsapp:" + c.FromNumber}}
	if c.TemplateSID != "" {
		// Template values may not contain new lines, tabs or runs of spaces.
		value := truncate(reNoTemplateSpace.ReplaceAllString(strings.TrimSpace(message), " "), 900)
		vars, _ := json.Marshal(map[string]string{"1": value})
		form.Set("ContentSid", c.TemplateSID)
		form.Set("ContentVariables", string(vars))
	} else {
		// No template: only works with Twilio's WhatsApp Sandbox, within 24 hours
		// of the receiving phone last messaging it.
		form.Set("Body", truncate(message, 1500))
	}
	return twilioSend(c.AccountSID, c.AuthToken, form)
}

// twilioSend sends a message through Twilio's Messages API (used for both
// SMS and WhatsApp) and turns Twilio's errors into plain words.
func twilioSend(accountSID, authToken string, form url.Values) error {
	endpoint := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Messages.json", twilioAPI, url.PathEscape(accountSID))
	req, err := http.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(accountSID, authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return simplifyNetError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4000))
		if json.Unmarshal(b, &e) == nil && e.Message != "" {
			if resp.StatusCode == 401 {
				return errors.New("Twilio did not accept the Account SID / Auth Token")
			}
			if hint := twilioHint(e.Code); hint != "" {
				return fmt.Errorf("%s (Twilio code %d)", hint, e.Code)
			}
			return fmt.Errorf("Twilio said: %s (code %d)", e.Message, e.Code)
		}
		return fmt.Errorf("Twilio answered %d", resp.StatusCode)
	}
	return nil
}

// twilioHint explains, in plain words, the Twilio errors new users are
// most likely to run into.
func twilioHint(code int) string {
	switch code {
	case 572006: // trial accounts may only send Twilio's preset template texts
		return "your Twilio account is a free trial, and trial accounts can't send pagewatch's alerts " +
			"(Twilio only lets them send its own preset messages). To get SMS alerts, upgrade your Twilio " +
			"account in the Twilio Console (texts then cost a few cents each), or choose the free phone notification (ntfy) instead"
	case 572002, 21608: // trial accounts may only text verified numbers
		return "your Twilio account is a free trial, and it can only text phone numbers you have verified " +
			"in the Twilio Console (Phone Numbers > Manage > Verified Caller IDs)"
	case 63016: // WhatsApp: free text outside the 24-hour window
		return "WhatsApp only allows an approved message template here. Add your approved template's " +
			"Content SID (starts with HX) to pagewatch's WhatsApp settings. (Without a template, WhatsApp only " +
			"works with Twilio's Sandbox, within 24 hours of the receiving phone messaging it)"
	case 63007: // WhatsApp: the From number isn't a WhatsApp sender on this account
		return "Twilio doesn't know this number as a WhatsApp sender. Check that it is registered under " +
			"Numbers & senders > WhatsApp in the Twilio Console and that its status is Online"
	case 63003: // WhatsApp: the To number isn't on WhatsApp
		return "the receiving number doesn't seem to be on WhatsApp. Check the number, including the + and country code"
	}
	return ""
}

func sendEmail(c *EmailConfig, subject, body string) error {
	addr := net.JoinHostPort(c.SMTPServer, strconv.Itoa(c.SMTPPort))
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	tlsConf := &tls.Config{ServerName: c.SMTPServer}

	var conn net.Conn
	var err error
	if c.SMTPPort == 465 {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsConf)
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return simplifyNetError(err)
	}
	_ = conn.SetDeadline(time.Now().Add(60 * time.Second))

	cl, err := smtp.NewClient(conn, c.SMTPServer)
	if err != nil {
		conn.Close()
		return err
	}
	defer cl.Close()

	if c.SMTPPort != 465 {
		if ok, _ := cl.Extension("STARTTLS"); !ok {
			return errors.New("the mail server does not offer a secure connection")
		}
		if err := cl.StartTLS(tlsConf); err != nil {
			return err
		}
	}
	if err := cl.Auth(smtp.PlainAuth("", c.Username, c.Password, c.SMTPServer)); err != nil {
		if strings.Contains(err.Error(), "535") || strings.Contains(err.Error(), "534") {
			return errors.New("the mail server did not accept the username/password. For Gmail, use an App Password (https://myaccount.google.com/apppasswords), not your normal password")
		}
		return err
	}
	if err := cl.Mail(c.Username); err != nil {
		return err
	}
	if err := cl.Rcpt(c.To); err != nil {
		return err
	}
	w, err := cl.Data()
	if err != nil {
		return err
	}
	msg := "From: pagewatch <" + c.Username + ">\r\n" +
		"To: " + c.To + "\r\n" +
		"Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n" +
		"Date: " + time.Now().Format(time.RFC1123Z) + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" +
		wrap76(base64.StdEncoding.EncodeToString([]byte(body)))
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return cl.Quit()
}

func wrap76(s string) string {
	var b strings.Builder
	for len(s) > 76 {
		b.WriteString(s[:76] + "\r\n")
		s = s[76:]
	}
	b.WriteString(s + "\r\n")
	return b.String()
}
