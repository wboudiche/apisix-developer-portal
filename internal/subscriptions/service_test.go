package subscriptions

import (
	"context"
	"testing"

	"apisix-portal/internal/apisix"
)

type memStore struct {
	creds    map[int64]Credential
	products map[int64]ProductInfo
	plans    map[int64]PlanInfo
	records  map[int64]*SubscriptionRecord // subID -> record
	nextID   int64
}

func newMemStore() *memStore {
	return &memStore{
		creds:    map[int64]Credential{},
		products: map[int64]ProductInfo{3: {ID: 3, ContextPath: "/pizzashack", Upstream: "echo:8080"}},
		plans:    map[int64]PlanInfo{2: {ID: 2, Count: 100, WindowSeconds: 60}},
		records:  map[int64]*SubscriptionRecord{},
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

func (m *memStore) findRecord(appID, productID int64) *SubscriptionRecord {
	for _, r := range m.records {
		if r.AppID == appID && r.ProductID == productID {
			return r
		}
	}
	return nil
}

func (m *memStore) SaveSubscription(_ context.Context, appID, productID, planID int64) error {
	if r := m.findRecord(appID, productID); r != nil {
		r.PlanID = planID
		r.Status = "active"
		return nil
	}
	m.nextID++
	m.records[m.nextID] = &SubscriptionRecord{ID: m.nextID, AppID: appID, ProductID: productID, PlanID: planID, Status: "active"}
	return nil
}
func (m *memStore) DeleteSubscription(_ context.Context, appID, productID int64) error {
	if r := m.findRecord(appID, productID); r != nil {
		delete(m.records, r.ID)
	}
	return nil
}
func (m *memStore) ConsumersForProduct(_ context.Context, productID int64) ([]string, error) {
	var out []string
	for _, r := range m.records {
		if r.ProductID == productID && r.Status == "active" {
			out = append(out, consumerName(r.AppID))
		}
	}
	return out, nil
}
func (m *memStore) ConsumersForPlan(_ context.Context, planID int64) ([]Credential, error) {
	var out []Credential
	for _, r := range m.records {
		if r.PlanID == planID && r.Status == "active" {
			if c, ok := m.creds[r.AppID]; ok {
				out = append(out, c)
			} else {
				out = append(out, Credential{ApplicationID: r.AppID, ConsumerUsername: consumerName(r.AppID)})
			}
		}
	}
	return out, nil
}
func (m *memStore) GetSubscription(_ context.Context, subID int64) (SubscriptionRecord, error) {
	if r, ok := m.records[subID]; ok {
		return *r, nil
	}
	return SubscriptionRecord{}, ErrNotFound
}
func (m *memStore) SetSubscriptionStatus(_ context.Context, subID int64, status string) error {
	r, ok := m.records[subID]
	if !ok {
		return ErrNotFound
	}
	r.Status = status
	return nil
}
func (m *memStore) AdminSubscriptions(_ context.Context, statusFilter string) ([]AdminSubscriptionView, error) {
	out := []AdminSubscriptionView{}
	for _, r := range m.records {
		if statusFilter == "" || r.Status == statusFilter {
			out = append(out, AdminSubscriptionView{ID: r.ID, Status: r.Status})
		}
	}
	return out, nil
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
	store.records[101] = &SubscriptionRecord{ID: 101, AppID: 1, ProductID: 7, PlanID: 2, Status: "active"}
	store.records[102] = &SubscriptionRecord{ID: 102, AppID: 2, ProductID: 7, PlanID: 2, Status: "active"}
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
	store.creds[1] = Credential{ApplicationID: 1, APIKey: "k1", ConsumerUsername: "app_1"}
	store.creds[2] = Credential{ApplicationID: 2, APIKey: "k2", ConsumerUsername: "app_2"}
	store.records[201] = &SubscriptionRecord{ID: 201, AppID: 1, ProductID: 3, PlanID: 2, Status: "active"}
	store.records[202] = &SubscriptionRecord{ID: 202, AppID: 2, ProductID: 3, PlanID: 2, Status: "active"}
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
