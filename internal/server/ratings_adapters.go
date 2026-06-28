package server

import (
	"context"
	"errors"

	"apisix-portal/internal/catalog"
	"apisix-portal/internal/ratings"
	"apisix-portal/internal/subscriptions"
)

type ratingsProductsAdapter struct{ repo *catalog.Repo }

func (a ratingsProductsAdapter) ProductBySlug(ctx context.Context, slug string) (int64, error) {
	id, _, err := a.repo.ProductBySlug(ctx, slug)
	if errors.Is(err, catalog.ErrNotFound) {
		return 0, ratings.ErrNotFound
	}
	return id, err
}

type ratingsSubsAdapter struct{ subs *subscriptions.Repo }

func (a ratingsSubsAdapter) IsApprovedSubscriber(ctx context.Context, userID, productID int64) (bool, error) {
	apps, err := a.subs.ApprovedAppsForProduct(ctx, userID, productID)
	if err != nil {
		return false, err
	}
	return len(apps) > 0, nil
}
