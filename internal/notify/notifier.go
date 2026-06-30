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
	OwnerEmailForApp(ctx context.Context, appID int64) (string, string, error)
	AdminEmails(ctx context.Context) ([]string, error)
	ProductName(ctx context.Context, productID int64) (string, error)
	PlanName(ctx context.Context, planID int64) (string, error)
}

// Notifier renders + sends the approval-loop emails, best-effort and async.
type Notifier struct {
	sender  Sender
	repo    Resolver
	baseURL string
}

func NewNotifier(sender Sender, repo Resolver, baseURL string) *Notifier {
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

// deliver resolves recipients, renders the template, and sends. Synchronous and
// best-effort: all errors are logged and dropped; empty recipients are skipped.
func (n *Notifier) deliver(kind string, appID, productID, planID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), deliverTimeout)
	defer cancel()

	product, err := n.repo.ProductName(ctx, productID)
	if err != nil {
		log.Printf("notify: product name (product=%d): %v", productID, err)
	}
	if product == "" {
		product = "une API"
	}
	owner, appName, err := n.repo.OwnerEmailForApp(ctx, appID)
	if err != nil {
		log.Printf("notify: owner email (app=%d): %v", appID, err)
	}
	if appName == "" {
		appName = "votre application"
	}

	var to []string
	var subject, body string
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
		subject = "Nouvelle demande d'abonnement à examiner"
		body = fmt.Sprintf("Une nouvelle demande d'abonnement attend votre validation.\n\nApplication : %s\nAPI : %s\nForfait : %s\n\nExaminez-la ici : %s/admin/approvals\n",
			appName, product, plan, n.baseURL)
	case kindApproved:
		to = []string{owner}
		plan, _ := n.repo.PlanName(ctx, planID)
		if plan == "" {
			plan = "votre forfait"
		}
		subject = "Votre abonnement est approuvé"
		body = fmt.Sprintf("Bonne nouvelle ! L'abonnement de %s à %s (%s) est approuvé.\n\nRetrouvez vos identifiants ici : %s/applications\n",
			appName, product, plan, n.baseURL)
	case kindRejected:
		to = []string{owner}
		subject = "Votre demande d'abonnement a été refusée"
		body = fmt.Sprintf("La demande d'abonnement de %s à %s n'a pas été approuvée.\n\nParcourez le catalogue : %s/\n",
			appName, product, n.baseURL)
	default:
		return
	}

	// Drop empty recipients (e.g. a missing owner email or no admins).
	clean := to[:0]
	for _, addr := range to {
		if addr != "" {
			clean = append(clean, addr)
		}
	}
	if len(clean) == 0 {
		return
	}
	if err := n.sender.Send(ctx, clean, subject, body); err != nil {
		log.Printf("notify: send %q to %v: %v", kind, clean, err)
	}
}
