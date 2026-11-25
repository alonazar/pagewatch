package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// fakeTwilio records the last request and answers with the given code
// (0 means success).
func fakeTwilio(t *testing.T, code *int, got *url.Values) func() {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		*got = r.PostForm
		if *code == 0 {
			w.WriteHeader(201)
			return
		}
		w.WriteHeader(400)
		fmt.Fprintf(w, `{"code": %d, "message": "something technical", "status": 400}`, *code)
	}))
	old := twilioAPI
	twilioAPI = srv.URL
	return func() { twilioAPI = old; srv.Close() }
}

func TestWhatsAppTemplateMessage(t *testing.T) {
	code := 0
	var got url.Values
	defer fakeTwilio(t, &code, &got)()

	c := &WhatsAppConfig{"AC1", "t", "+15005550006", "+61412345678", "HX0123456789abcdef0123456789abcdef"}
	msg := "Found \"tickets\",\n\t\"מבצע\"    on https://example.com" + strings.Repeat("x", 2000)
	if err := sendWhatsApp(c, msg); err != nil {
		t.Fatal(err)
	}
	if got.Get("From") != "whatsapp:+15005550006" || got.Get("To") != "whatsapp:+61412345678" {
		t.Errorf("numbers not sent as WhatsApp addresses: From=%q To=%q", got.Get("From"), got.Get("To"))
	}
	if got.Get("ContentSid") != c.TemplateSID || got.Get("Body") != "" {
		t.Errorf("expected a template message, got ContentSid=%q Body=%q", got.Get("ContentSid"), got.Get("Body"))
	}
	var vars map[string]string
	if err := json.Unmarshal([]byte(got.Get("ContentVariables")), &vars); err != nil {
		t.Fatalf("ContentVariables is not JSON: %v", err)
	}
	v := vars["1"]
	if !strings.HasPrefix(v, `Found "tickets", "מבצע" on https://example.com`) {
		t.Errorf("template value not cleaned up: %q", v[:60])
	}
	if strings.ContainsAny(v, "\n\t") || strings.Contains(v, "  ") || len(v) > 900 {
		t.Errorf("template value breaks WhatsApp's rules (len %d)", len(v))
	}
}

func TestWhatsAppSandboxMessage(t *testing.T) {
	code := 0
	var got url.Values
	defer fakeTwilio(t, &code, &got)()

	c := &WhatsAppConfig{AccountSID: "AC1", AuthToken: "t", FromNumber: "+14155238886", ToNumber: "+61412345678"}
	if err := sendWhatsApp(c, "hello\nthere"); err != nil {
		t.Fatal(err)
	}
	if got.Get("Body") != "hello\nthere" || got.Get("ContentSid") != "" {
		t.Errorf("expected a free-text Sandbox message, got Body=%q ContentSid=%q", got.Get("Body"), got.Get("ContentSid"))
	}
}

func TestWhatsAppErrorsExplained(t *testing.T) {
	code := 0
	var got url.Values
	defer fakeTwilio(t, &code, &got)()

	c := &WhatsAppConfig{"AC1", "t", "+15005550006", "+61412345678", ""}
	for c2, want := range map[int]string{
		63016: "approved message template",
		63007: "WhatsApp sender",
		63003: "doesn't seem to be on WhatsApp",
		21211: "Twilio said: something technical (code 21211)",
	} {
		code = c2
		err := sendWhatsApp(c, "hi")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("code %d: got %v, want it to mention %q", c2, err, want)
		}
	}
}

func TestWhatsAppConfigValidation(t *testing.T) {
	base := func() *Config {
		return &Config{
			Pages:  []Page{{URL: "https://example.com", Words: []string{"x"}}},
			Alerts: Alerts{WhatsApp: &WhatsAppConfig{"AC1", "t", "+1", "+2", ""}},
		}
	}
	if err := base().validate(); err != nil {
		t.Errorf("Sandbox settings (no template) should be valid: %v", err)
	}
	c := base()
	c.Alerts.WhatsApp.TemplateSID = "XX123"
	if err := c.validate(); err == nil {
		t.Error("a template SID not starting with HX should be rejected")
	}
	c = base()
	c.Alerts.WhatsApp.ToNumber = ""
	if err := c.validate(); err == nil {
		t.Error("a missing number should be rejected")
	}
}

func TestCleanAnswerRemovesInvisibleCharacters(t *testing.T) {
	bom, zwsp, zwj := string(rune(0xFEFF)), string(rune(0x200B)), string(rune(0x200D))
	if got := cleanAnswer(bom + " https://exa" + zwsp + "mple.com \r\n"); got != "https://example.com" {
		t.Errorf("cleanAnswer = %q", got)
	}
	if got := cleanAnswer("מבצע" + zwj); got != "מבצע" {
		t.Errorf("cleanAnswer = %q", got)
	}
}

func TestWhatsAppTemplateFollowsMetaRules(t *testing.T) {
	if strings.HasPrefix(WhatsAppTemplate, "{{") || strings.HasSuffix(WhatsAppTemplate, "}}") {
		t.Error("Meta rejects templates that start or end with a placeholder")
	}
	if strings.Count(WhatsAppTemplate, "{{") != 1 || !strings.Contains(WhatsAppTemplate, "{{1}}") {
		t.Error("the template must have exactly one placeholder, {{1}}")
	}
}
