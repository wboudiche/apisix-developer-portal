package notify

import (
	"context"
	"errors"
	"testing"

	"apisix-portal/internal/settings"
)

type stubSource struct{ e *settings.Effective }

func (s stubSource) Snapshot() *settings.Effective { return s.e }

func eff(values map[string]string) *settings.Effective {
	e := &settings.Effective{Values: map[string]string{}, Source: map[string]string{}}
	for k, v := range values {
		e.Values[k] = v
	}
	return e
}

func TestDynamicSenderUnconfigured(t *testing.T) {
	d := NewDynamicSender(stubSource{eff(nil)})
	err := d.Send(context.Background(), []string{"a@b.c"}, "s", "b")
	if !errors.Is(err, ErrSMTPNotConfigured) {
		t.Fatalf("want ErrSMTPNotConfigured, got %v", err)
	}
}

func TestDynamicSenderReadsSnapshotPerSend(t *testing.T) {
	src := stubSource{eff(map[string]string{
		"SMTP_HOST": "127.0.0.1", "SMTP_PORT": "1", "SMTP_FROM": "x@y.z",
	})}
	d := NewDynamicSender(src)
	// Nothing listens on :1 — the point is that it TRIED the snapshot values
	// (connection refused), not that it succeeded.
	err := d.Send(context.Background(), []string{"a@b.c"}, "s", "b")
	if err == nil || errors.Is(err, ErrSMTPNotConfigured) {
		t.Fatalf("want a dial error from snapshot values, got %v", err)
	}
}
