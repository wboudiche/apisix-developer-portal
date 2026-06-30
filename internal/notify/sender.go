// Package notify sends best-effort email notifications for the subscription
// approval loop over bring-your-own SMTP.
package notify

import (
	"context"
	"crypto/tls"
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

func (s *SMTPSender) Send(ctx context.Context, to []string, subject, body string) error {
	if len(to) == 0 {
		return nil
	}
	addr := net.JoinHostPort(s.host, s.port)
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl) // bound the whole SMTP conversation incl. DATA
	}
	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return err
	}
	defer c.Close()
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: s.host}); err != nil {
			return err
		}
	}
	if s.username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
			return err
		}
	}
	if err := c.Mail(s.from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(buildMessage(s.from, to, subject, body)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
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
