package subscriptions

import (
	"context"
	"testing"

	"apisix-portal/internal/apisix"
)

type memStore struct {
	creds         map[int64]Credential
	subs          map[int64][]string // productID -> consumer usernames
	products      map[int64]ProductInfo
	plans         map[int64]PlanInfo
	planConsumers map[int64][]Credential
}

func newMemStore() *memStore {
	return &memStore{
		creds:         map[int64]Credential{},
		subs:          map[int64][]string{},
		products:      map[int64]ProductInfo{3: {ID: 3, ContextPath: "/pizzashack", Upstream: "echo:8080"}},
		plans:         map[int64]PlanInfo{2: {ID: 2, Count: 100, WindowSeconds: 60}},
		planConsumers: map[int64][]Credential{},
	}
}

func (m *memStore) GetOrCreateCredential(_ context.Context, appID int64, genKey func() string) (Credential, error) {
	if c, ok := m.creds[appID]; ok {
		return c, nil
	}
	c := Credential{ApplicationID: appID, APIKey: genKey(), ConsumerUsername: consumerName(appID)}
	m.creds[appID] = c
	return c, nil
}
func (m *memStore) GetProduct(_ context.Context, id int64) (ProductInfo, error) {
	return m.products[id], nil
}
func (m *memStore) GetPlan(_ context.Context, id int64) (PlanInfo, error) { return m.plans[id], nil }
func (m *memStore) SaveSubscription(_ context.Context, appID, productID, _ int64) error {
	m.subs[productID] = append(m.subs[productID], consumerName(appID))
	return nil
}
func (m *memStore) DeleteSubscription(_ context.Context, appID, productID int64) error {
	cur := m.subs[productID]
	out := cur[:0]
	for _, u := range cur {
		if u != consumerName(appID) {
			out = append(out, u)
		}
	}
	m.subs[productID] = out
	return nil
}
func (m *memStore) ConsumersForProduct(_ context.Context, productID int64) ([]string, error) {
	return m.subs[productID], nil
}
func (m *memStore) ConsumersForPlan(_ context.Context, planID int64) ([]Credential, error) {
	return m.planConsumers[planID], nil
}

func TestSubscribeProvisionsConsumerAndRoute(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "fixed-key" })

	cred, err := svc.Subscribe(ctx, 42, 3, 2)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if cred.APIKey != "fixed-key" || cred.ConsumerUsername != "app_42" {
		t.Fatalf("bad cred: %+v", cred)
	}
	c := gw.Consumers["app_42"]
	if c.APIKey != "fixed-key" || c.Limit.Count != 100 {
		t.Fatalf("consumer not provisioned: %+v", c)
	}
	r := gw.Routes["prod_3"]
	if r.URI != "/pizzashack/*" || len(r.Allowed) != 1 || r.Allowed[0] != "app_42" {
		t.Fatalf("route not provisioned: %+v", r)
	}
}

func TestUnsubscribeRemovesFromWhitelist(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "k" })
	_, _ = svc.Subscribe(ctx, 42, 3, 2)
	_, _ = svc.Subscribe(ctx, 43, 3, 2)
	if err := svc.Unsubscribe(ctx, 42, 3); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	r := gw.Routes["prod_3"]
	if len(r.Allowed) != 1 || r.Allowed[0] != "app_43" {
		t.Fatalf("whitelist after unsubscribe: %+v", r.Allowed)
	}
}

func TestReprovisionRoute(t *testing.T) {
	store := newMemStore()
	store.products[7] = ProductInfo{ID: 7, ContextPath: "/seven", Upstream: "echo:8080"}
	store.subs[7] = []string{"app_1", "app_2"}
	gw := apisix.NewFake()
	svc := NewService(store, gw, GenerateKey)

	if err := svc.ReprovisionRoute(context.Background(), 7); err != nil {
		t.Fatalf("reprovision: %v", err)
	}
	r, ok := gw.Routes[RouteID(7)]
	if !ok {
		t.Fatalf("route %s not created", RouteID(7))
	}
	if r.Upstream != "echo:8080" || r.URI != "/seven/*" {
		t.Fatalf("unexpected route: %+v", r)
	}
	if len(r.Allowed) != 2 {
		t.Fatalf("want 2 allowed consumers, got %v", r.Allowed)
	}
}

func TestDeprovisionRoute(t *testing.T) {
	store := newMemStore()
	store.products[7] = ProductInfo{ID: 7, ContextPath: "/seven", Upstream: "echo:8080"}
	gw := apisix.NewFake()
	_ = gw.EnsureRoute(context.Background(), RouteID(7), "/seven/*", "echo:8080", nil)
	svc := NewService(store, gw, GenerateKey)

	if err := svc.DeprovisionRoute(context.Background(), 7); err != nil {
		t.Fatalf("deprovision: %v", err)
	}
	if _, ok := gw.Routes[RouteID(7)]; ok {
		t.Fatal("route still present after DeprovisionRoute")
	}
}

func TestReprovisionPlanUpdatesEachConsumerLimit(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	// Plan 2 exists in newMemStore with Count=100, Window=60. Two apps are on it.
	store.planConsumers[2] = []Credential{
		{ApplicationID: 1, APIKey: "k1", ConsumerUsername: "app_1"},
		{ApplicationID: 2, APIKey: "k2", ConsumerUsername: "app_2"},
	}
	gw := apisix.NewFake()
	svc := NewService(store, gw, GenerateKey)

	if err := svc.ReprovisionPlan(ctx, 2); err != nil {
		t.Fatalf("ReprovisionPlan: %v", err)
	}
	for _, name := range []string{"app_1", "app_2"} {
		c, ok := gw.Consumers[name]
		if !ok {
			t.Fatalf("consumer %s not provisioned", name)
		}
		if c.Limit.Count != 100 || c.Limit.WindowSeconds != 60 {
			t.Fatalf("consumer %s limit = %+v, want {100,60}", name, c.Limit)
		}
	}
	if gw.Consumers["app_1"].APIKey != "k1" {
		t.Fatalf("app_1 key not preserved: %q", gw.Consumers["app_1"].APIKey)
	}
}
