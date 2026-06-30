package notify

import (
	"strings"
	"testing"
)

func TestBuildMessageHeadersAndBody(t *testing.T) {
	msg := string(buildMessage("portal@example.com", []string{"dev@example.com", "ops@example.com"}, "Sujet é", "Bonjour\nLigne 2"))
	for _, want := range []string{
		"From: portal@example.com\r\n",
		"To: dev@example.com, ops@example.com\r\n",
		"Subject: Sujet é\r\n",
		"MIME-Version: 1.0\r\n",
		"Content-Type: text/plain; charset=utf-8\r\n",
		"Date: ",
		"\r\n\r\nBonjour\r\nLigne 2", // blank line then CRLF-normalized body
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q\n--- got ---\n%s", want, msg)
		}
	}
}
