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
	"strconv"
	"strings"
	"time"
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
	if a.Email != nil {
		try("email", func() error { return sendEmail(a.Email, title, message) })
	}
	return sent, errors.Join(errs...)
}

func describeAlerts(a Alerts) string {
	var parts []string
	if a.Ntfy != nil {
		parts = append(parts, "phone notification (ntfy topic "+a.Ntfy.Topic+")")
	}
	if a.SMS != nil {
		parts = append(parts, "SMS to "+a.SMS.ToNumber)
	}
	if a.Email != nil {
		parts = append(parts, "email to "+a.Email.To)
	}
	return strings.Join(parts, ", ")
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

func sendSMS(c *TwilioConfig, message string) error {
	if len(message) > 1000 {
		message = message[:1000]
	}
	form := url.Values{"To": {c.ToNumber}, "From": {c.FromNumber}, "Body": {message}}
	endpoint := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Messages.json", twilioAPI, url.PathEscape(c.AccountSID))
	req, err := http.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.AccountSID, c.AuthToken)
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
			return fmt.Errorf("Twilio said: %s (code %d)", e.Message, e.Code)
		}
		return fmt.Errorf("Twilio answered %d", resp.StatusCode)
	}
	return nil
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
