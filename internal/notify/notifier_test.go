package notify

import (
	"context"
	"strings"
	"testing"
)

type fakeSender struct {
	to      []string
	subject string
	body    string
	calls   int
}

func (f *fakeSender) Send(_ context.Context, to []string, subject, body string) error {
	f.calls++
	f.to, f.subject, f.body = to, subject, body
	return nil
}

type fakeResolver struct{}

func (fakeResolver) OwnerEmailForApp(_ context.Context, _ int64) (string, string, error) {
	return "dev@example.com", "Mon App", nil
}
func (fakeResolver) AdminEmails(_ context.Context) ([]string, error) {
	return []string{"admin@example.com"}, nil
}
func (fakeResolver) ProductName(_ context.Context, _ int64) (string, error) { return "Orders API", nil }
func (fakeResolver) PlanName(_ context.Context, _ int64) (string, error)    { return "Gold", nil }

func TestDeliverApprovedToOwner(t *testing.T) {
	fs := &fakeSender{}
	n := NewNotifier(fs, fakeResolver{}, "https://portal.example")
	n.deliver(kindApproved, 1, 2, 3)
	if fs.calls != 1 || len(fs.to) != 1 || fs.to[0] != "dev@example.com" {
		t.Fatalf("to=%v calls=%d", fs.to, fs.calls)
	}
	if !strings.Contains(fs.body, "Orders API") || !strings.Contains(fs.body, "Mon App") ||
		!strings.Contains(fs.body, "https://portal.example/applications") {
		t.Fatalf("body missing details: %s", fs.body)
	}
}

func TestDeliverRequestedToAdmins(t *testing.T) {
	fs := &fakeSender{}
	n := NewNotifier(fs, fakeResolver{}, "https://portal.example")
	n.deliver(kindRequested, 1, 2, 3)
	if fs.calls != 1 || len(fs.to) != 1 || fs.to[0] != "admin@example.com" {
		t.Fatalf("to=%v", fs.to)
	}
	if !strings.Contains(fs.body, "/admin/approvals") {
		t.Fatalf("admin body missing link: %s", fs.body)
	}
}

func TestDeliverSkipsEmptyRecipients(t *testing.T) {
	fs := &fakeSender{}
	n := NewNotifier(fs, emptyResolver{}, "https://portal.example")
	n.deliver(kindApproved, 1, 2, 3) // owner email "" -> no send
	if fs.calls != 0 {
		t.Fatalf("expected no send, got %d", fs.calls)
	}
}

type emptyResolver struct{ fakeResolver }

func (emptyResolver) OwnerEmailForApp(_ context.Context, _ int64) (string, string, error) {
	return "", "", nil
}
