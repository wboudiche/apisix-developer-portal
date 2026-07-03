package notify

import (
	"context"
	"strings"
	"testing"
)

type sent struct {
	to      []string
	subject string
	body    string
}

type fakeSender struct {
	sent []sent
}

func (f *fakeSender) Send(_ context.Context, to []string, subject, body string) error {
	f.sent = append(f.sent, sent{to: to, subject: subject, body: body})
	return nil
}

type fakeResolver struct {
	owners  []Recipient
	admins  []Recipient
	app     string
	product string
	plan    string
}

func (r fakeResolver) OwnerEmailsForApp(_ context.Context, _ int64) ([]Recipient, string, error) {
	return r.owners, r.app, nil
}
func (r fakeResolver) AdminEmails(_ context.Context) ([]Recipient, error) {
	return r.admins, nil
}
func (r fakeResolver) ProductName(_ context.Context, _ int64) (string, error) { return r.product, nil }
func (r fakeResolver) PlanName(_ context.Context, _ int64) (string, error)    { return r.plan, nil }

// defaultResolver mirrors the fixture data the pre-i18n fake resolver
// hardcoded: one French-speaking owner + admin.
func defaultResolver() *fakeResolver {
	return &fakeResolver{
		owners:  []Recipient{{Email: "dev@example.com", Lang: "fr"}},
		admins:  []Recipient{{Email: "admin@example.com", Lang: "fr"}},
		app:     "Mon App",
		product: "Orders API",
		plan:    "Gold",
	}
}

func TestDeliverApprovedToOwner(t *testing.T) {
	fs := &fakeSender{}
	n := NewNotifier(fs, defaultResolver(), "https://portal.example")
	n.deliver(kindApproved, 1, 2, 3)
	if len(fs.sent) != 1 || len(fs.sent[0].to) != 1 || fs.sent[0].to[0] != "dev@example.com" {
		t.Fatalf("sent=%+v", fs.sent)
	}
	body := fs.sent[0].body
	if !strings.Contains(body, "Orders API") || !strings.Contains(body, "Mon App") ||
		!strings.Contains(body, "https://portal.example/applications") {
		t.Fatalf("body missing details: %s", body)
	}
}

func TestDeliverRequestedToAdmins(t *testing.T) {
	fs := &fakeSender{}
	n := NewNotifier(fs, defaultResolver(), "https://portal.example")
	n.deliver(kindRequested, 1, 2, 3)
	if len(fs.sent) != 1 || len(fs.sent[0].to) != 1 || fs.sent[0].to[0] != "admin@example.com" {
		t.Fatalf("sent=%+v", fs.sent)
	}
	if !strings.Contains(fs.sent[0].body, "/admin/approvals") {
		t.Fatalf("admin body missing link: %s", fs.sent[0].body)
	}
}

func TestDeliverSkipsEmptyRecipients(t *testing.T) {
	fs := &fakeSender{}
	n := NewNotifier(fs, &emptyResolver{}, "https://portal.example")
	n.deliver(kindApproved, 1, 2, 3) // no owners -> no send
	if len(fs.sent) != 0 {
		t.Fatalf("expected no send, got %d", len(fs.sent))
	}
}

type emptyResolver struct{ fakeResolver }

func (emptyResolver) OwnerEmailsForApp(_ context.Context, _ int64) ([]Recipient, string, error) {
	return nil, "", nil
}

func TestEmailTemplateParity(t *testing.T) {
	for _, kind := range []string{kindRequested, kindApproved, kindRejected} {
		for _, lang := range []string{"fr", "en"} {
			tpl, ok := emailTemplates[kind][lang]
			if !ok || tpl.subject == "" || tpl.body == "" {
				t.Errorf("missing/empty template kind=%s lang=%s", kind, lang)
			}
		}
	}
}

func TestDeliverLocalizesPerRecipient(t *testing.T) {
	sender := &fakeSender{}
	repo := &fakeResolver{
		admins:  []Recipient{{Email: "fr-admin@x.io", Lang: "fr"}, {Email: "en-admin@x.io", Lang: "en"}},
		app:     "Mon App",
		product: "Une API",
		plan:    "Silver",
	}
	n := NewNotifier(sender, repo, "http://portal")
	n.deliver(kindRequested, 1, 2, 3) // synchronous

	if len(sender.sent) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(sender.sent))
	}
	bySubject := map[string]sent{}
	for _, s := range sender.sent {
		bySubject[s.to[0]] = s
	}
	if !strings.Contains(bySubject["fr-admin@x.io"].subject, "Nouvelle demande") {
		t.Errorf("fr admin subject = %q", bySubject["fr-admin@x.io"].subject)
	}
	if !strings.Contains(bySubject["en-admin@x.io"].subject, "New subscription") {
		t.Errorf("en admin subject = %q", bySubject["en-admin@x.io"].subject)
	}
}

func TestDeliverUnknownLangFallsBackToFrench(t *testing.T) {
	sender := &fakeSender{}
	repo := &fakeResolver{admins: []Recipient{{Email: "x@x.io", Lang: "de"}}, app: "A", product: "P", plan: "Pl"}
	NewNotifier(sender, repo, "http://portal").deliver(kindRequested, 1, 2, 3)
	if len(sender.sent) != 1 || !strings.Contains(sender.sent[0].subject, "Nouvelle demande") {
		t.Fatalf("unknown lang should fall back to fr: %+v", sender.sent)
	}
}
