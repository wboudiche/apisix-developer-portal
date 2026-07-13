package notify

import (
	"context"
	"errors"

	"apisix-portal/internal/settings"
)

// ErrSMTPNotConfigured is returned by DynamicSender when the current settings
// snapshot has no SMTP host/from.
var ErrSMTPNotConfigured = errors.New("notify: SMTP not configured")

// SettingsSource is the read surface DynamicSender needs (satisfied by
// *settings.Service).
type SettingsSource interface{ Snapshot() *settings.Effective }

// DynamicSender resolves SMTP parameters from the settings snapshot at each
// Send, so runtime settings changes apply to the next email with no rewiring.
type DynamicSender struct{ src SettingsSource }

func NewDynamicSender(src SettingsSource) *DynamicSender { return &DynamicSender{src: src} }

func (d *DynamicSender) Send(ctx context.Context, to []string, subject, body string) error {
	e := d.src.Snapshot()
	if !e.SMTPConfigured() {
		return ErrSMTPNotConfigured
	}
	s := NewSMTPSender(e.Get("SMTP_HOST"), e.Get("SMTP_PORT"), e.Get("SMTP_USERNAME"), e.Get("SMTP_PASSWORD"), e.Get("SMTP_FROM"))
	return s.Send(ctx, to, subject, body)
}

var _ Sender = (*DynamicSender)(nil)
