// Package notify sends best-effort email notifications for the subscription
// approval loop over bring-your-own SMTP.
package notify

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Sender delivers one email to one or more recipients.
type Sender interface {
	Send(ctx context.Context, to []string, subject, body string) error
}

// SMTPSender sends via net/smtp. PlainAuth is used when a username is set
// (STARTTLS is negotiated by smtp.SendMail when the server advertises it);
// no auth otherwise (e.g. a local Mailpit).
type SMTPSender struct {
	host, port, username, password, from string
}

func NewSMTPSender(host, port, username, password, from string) *SMTPSender {
	return &SMTPSender{host: host, port: port, username: username, password: password, from: from}
}

func (s *SMTPSender) Send(_ context.Context, to []string, subject, body string) error {
	if len(to) == 0 {
		return nil
	}
	var auth smtp.Auth
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}
	addr := net.JoinHostPort(s.host, s.port)
	return smtp.SendMail(addr, auth, s.from, to, buildMessage(s.from, to, subject, body))
}

// buildMessage renders an RFC 5322 plaintext message with CRLF line endings.
// The body's \n are normalized to \r\n.
func buildMessage(from string, to []string, subject, body string) []byte {
	crlfBody := strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n")
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(to, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(crlfBody)
	return []byte(b.String())
}

var _ Sender = (*SMTPSender)(nil)
