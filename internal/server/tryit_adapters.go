package server

import (
	"context"
	"errors"

	"apisix-portal/internal/catalog"
	"apisix-portal/internal/subscriptions"
	"apisix-portal/internal/teams"
	"apisix-portal/internal/tryit"
)

type tryitProductsAdapter struct{ repo *catalog.Repo }

func (a tryitProductsAdapter) ProductBySlug(ctx context.Context, slug string) (int64, string, error) {
	id, ctxPath, err := a.repo.ProductBySlug(ctx, slug)
	if errors.Is(err, catalog.ErrNotFound) {
		return 0, "", tryit.ErrNotFound
	}
	return id, ctxPath, err
}

func (a tryitProductsAdapter) SandboxUpstream(ctx context.Context, slug string) (bool, error) {
	return a.repo.SandboxUpstreamBySlug(ctx, slug)
}

type tryitAccessAdapter struct {
	teams *teams.Repo
	subs  *subscriptions.Repo
}

func (a tryitAccessAdapter) OwnsApp(ctx context.Context, appID, userID int64) (bool, error) {
	return a.teams.IsMemberOfApp(ctx, userID, appID)
}

func (a tryitAccessAdapter) SubscriptionStatus(ctx context.Context, appID, productID int64) (string, error) {
	return a.subs.SubscriptionStatus(ctx, appID, productID)
}

func (a tryitAccessAdapter) APIKey(ctx context.Context, appID int64) (string, error) {
	c, err := a.subs.GetCredential(ctx, appID)
	if err != nil {
		return "", err
	}
	return c.APIKey, nil
}

func (a tryitAccessAdapter) ApprovedApps(ctx context.Context, userID, productID int64) ([]tryit.AppRef, error) {
	refs, err := a.subs.ApprovedAppsForProduct(ctx, userID, productID)
	if err != nil {
		return nil, err
	}
	out := make([]tryit.AppRef, len(refs))
	for i, r := range refs {
		out[i] = tryit.AppRef{ID: r.ID, Name: r.Name}
	}
	return out, nil
}

func (a tryitAccessAdapter) SandboxKey(ctx context.Context, appID int64) (string, error) {
	key, err := a.subs.GetSandboxKey(ctx, appID)
	if errors.Is(err, subscriptions.ErrNotFound) {
		return "", nil
	}
	return key, err
}
