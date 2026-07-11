package notify

import (
	"context"
	"strings"
	"testing"
)

type captureSender struct {
	to      []string
	subject string
	body    string
}

func (c *captureSender) Send(_ context.Context, to []string, subject, body string) error {
	c.to, c.subject, c.body = to, subject, body
	return nil
}

func TestSendVerificationEmailFrench(t *testing.T) {
	s := &captureSender{}
	err := SendVerificationEmail(context.Background(), s, "fr", "dev@x.io", "Walid", "http://localhost:8088/verify-email?token=abc")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(s.to) != 1 || s.to[0] != "dev@x.io" {
		t.Fatalf("to = %v", s.to)
	}
	if !strings.Contains(s.subject, "Vérifiez") {
		t.Fatalf("subject = %q, want French", s.subject)
	}
	if !strings.Contains(s.body, "http://localhost:8088/verify-email?token=abc") {
		t.Fatal("body must contain the link")
	}
	if !strings.Contains(s.body, "Walid") {
		t.Fatal("body must greet the user by name")
	}
	if !strings.Contains(s.body, "24") {
		t.Fatal("body must mention the 24 h validity")
	}
}

func TestSendVerificationEmailEnglishAndFallbacks(t *testing.T) {
	s := &captureSender{}
	// Unknown language falls back like the notifier (normalizeLang), empty name
	// falls back to the email address in the greeting.
	if err := SendVerificationEmail(context.Background(), s, "de", "dev@x.io", "", "http://l/verify-email?token=t"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.Contains(s.subject, "Verify") && !strings.Contains(s.subject, "Vérifiez") {
		t.Fatalf("subject = %q, want a known-language fallback", s.subject)
	}
	if !strings.Contains(s.body, "dev@x.io") {
		t.Fatal("empty name must fall back to the email address")
	}
}
