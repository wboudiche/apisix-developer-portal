package notify

import (
	"context"
	"fmt"
	"log"
	"time"
)

const (
	kindRequested = "requested"
	kindApproved  = "approved"
	kindRejected  = "rejected"
)

const deliverTimeout = 20 * time.Second

// Resolver resolves recipient emails + display names (satisfied by *Repo).
type Resolver interface {
	OwnerEmailsForApp(ctx context.Context, appID int64) ([]Recipient, string, error)
	AdminEmails(ctx context.Context) ([]Recipient, error)
	ProductName(ctx context.Context, productID int64) (string, error)
	PlanName(ctx context.Context, planID int64) (string, error)
}

// Notifier renders + sends the approval-loop emails, best-effort and async.
type Notifier struct {
	sender  Sender
	repo    Resolver
	baseURL func() string // dynamic: reads the live settings snapshot on every send
}

func NewNotifier(sender Sender, repo Resolver, baseURL func() string) *Notifier {
	return &Notifier{sender: sender, repo: repo, baseURL: baseURL}
}

func (n *Notifier) SubscriptionRequested(appID, productID, planID int64) {
	go n.deliver(kindRequested, appID, productID, planID)
}
func (n *Notifier) SubscriptionApproved(appID, productID, planID int64) {
	go n.deliver(kindApproved, appID, productID, planID)
}
func (n *Notifier) SubscriptionRejected(appID, productID int64) {
	go n.deliver(kindRejected, appID, productID, 0)
}

type emailTemplate struct{ subject, body string }

// emailTemplates[kind][lang]. body is a fmt format string; the arg order per
// kind is fixed across languages (see deliver()).
var emailTemplates = map[string]map[string]emailTemplate{
	kindRequested: {
		"fr": {
			subject: "Nouvelle demande d'abonnement à examiner",
			body:    "Une nouvelle demande d'abonnement attend votre validation.\n\nApplication : %s\nAPI : %s\nForfait : %s\n\nExaminez-la ici : %s/admin/approvals\n",
		},
		"en": {
			subject: "New subscription request to review",
			body:    "A new subscription request is awaiting your approval.\n\nApplication: %s\nAPI: %s\nPlan: %s\n\nReview it here: %s/admin/approvals\n",
		},
	},
	kindApproved: {
		"fr": {
			subject: "Votre abonnement est approuvé",
			body:    "Bonne nouvelle ! L'abonnement de %s à %s (%s) est approuvé.\n\nRetrouvez vos identifiants ici : %s/applications\n",
		},
		"en": {
			subject: "Your subscription is approved",
			body:    "Good news! The subscription of %s to %s (%s) is approved.\n\nFind your credentials here: %s/applications\n",
		},
	},
	kindRejected: {
		"fr": {
			subject: "Votre demande d'abonnement a été refusée",
			body:    "La demande d'abonnement de %s à %s n'a pas été approuvée.\n\nParcourez le catalogue : %s/\n",
		},
		"en": {
			subject: "Your subscription request was declined",
			body:    "The subscription request of %s to %s was not approved.\n\nBrowse the catalog: %s/\n",
		},
	},
}

func normalizeLang(l string) string {
	if l == "en" {
		return "en"
	}
	return "fr"
}

// deliver resolves recipients, renders the template in each recipient's
// language, and sends one message per recipient. Best-effort: all errors are
// logged and dropped; empty recipient emails are skipped.
func (n *Notifier) deliver(kind string, appID, productID, planID int64) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("notify: recovered panic in deliver: %v", r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), deliverTimeout)
	defer cancel()

	product, err := n.repo.ProductName(ctx, productID)
	if err != nil {
		log.Printf("notify: product name (product=%d): %v", productID, err)
	}
	if product == "" {
		product = "une API"
	}
	owners, appName, err := n.repo.OwnerEmailsForApp(ctx, appID)
	if err != nil {
		log.Printf("notify: owner emails (app=%d): %v", appID, err)
	}
	if appName == "" {
		appName = "votre application"
	}

	var to []Recipient
	var args []any
	switch kind {
	case kindRequested:
		admins, err := n.repo.AdminEmails(ctx)
		if err != nil {
			log.Printf("notify: admin emails: %v", err)
			return
		}
		to = admins
		plan, _ := n.repo.PlanName(ctx, planID)
		if plan == "" {
			plan = "un forfait"
		}
		args = []any{appName, product, plan, n.baseURL()}
	case kindApproved:
		to = owners
		plan, _ := n.repo.PlanName(ctx, planID)
		if plan == "" {
			plan = "votre forfait"
		}
		args = []any{appName, product, plan, n.baseURL()}
	case kindRejected:
		to = owners
		args = []any{appName, product, n.baseURL()}
	default:
		return
	}

	for _, rc := range to {
		if rc.Email == "" {
			continue
		}
		tpl := emailTemplates[kind][normalizeLang(rc.Lang)]
		body := fmt.Sprintf(tpl.body, args...)
		if err := n.sender.Send(ctx, []string{rc.Email}, tpl.subject, body); err != nil {
			log.Printf("notify: send %q to %s: %v", kind, rc.Email, err)
		}
	}
}
