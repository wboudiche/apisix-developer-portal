package subscriptions

import (
	"context"
	"errors"
	"testing"

	"apisix-portal/internal/apisix"
	"apisix-portal/internal/paging"
)

// routeFailGateway wraps a Fake but makes EnsureRoute fail, simulating APISIX
// being unreachable when the route whitelist is (re)provisioned.
type routeFailGateway struct {
	*apisix.Fake
	err error
}

func (g *routeFailGateway) EnsureRoute(_ context.Context, _, _, _ string, _ []string) error {
	return g.err
}

// captureLogger records the kinds emitted, to assert the service dispatches the
// right activity events. failOn returns an error for a given kind to prove a log
// failure can't break the action.
type captureLogger struct {
	kinds  []string
	failOn string
}

func (c *captureLogger) Log(_ context.Context, _ int64, kind string, _, _ *int64) error {
	c.kinds = append(c.kinds, kind)
	if kind == c.failOn {
		return errors.New("log boom")
	}
	return nil
}

func TestServiceEmitsLifecycleEvents(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	log := &captureLogger{}
	svc := NewService(store, gw, func() string { return "k" }, log)

	if _, err := svc.Subscribe(ctx, 42, 3, 2); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := svc.Approve(ctx, store.findRecord(42, 3).ID); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if err := svc.Unsubscribe(ctx, 42, 3); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	// A second app to exercise Reject without colliding with the unsubscribe above.
	if _, err := svc.Subscribe(ctx, 43, 3, 2); err != nil {
		t.Fatalf("Subscribe 43: %v", err)
	}
	if err := svc.Reject(ctx, store.findRecord(43, 3).ID); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	want := []string{"subscribed", "approved", "unsubscribed", "subscribed", "rejected"}
	if len(log.kinds) != len(want) {
		t.Fatalf("emitted kinds = %v, want %v", log.kinds, want)
	}
	for i := range want {
		if log.kinds[i] != want[i] {
			t.Fatalf("emitted kinds = %v, want %v", log.kinds, want)
		}
	}
}

func TestServiceActionSucceedsWhenEventLogFails(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "k" }, &captureLogger{failOn: "subscribed"})

	// A failing activity log must not fail the subscribe it merely records.
	if _, err := svc.Subscribe(ctx, 42, 3, 2); err != nil {
		t.Fatalf("Subscribe must succeed despite a log failure, got %v", err)
	}
	if r := store.findRecord(42, 3); r == nil || r.Status != StatusPending {
		t.Fatalf("subscription must be persisted despite the log failure, got %+v", r)
	}
}

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
		products: map[int64]ProductInfo{3: {ID: 3, ContextPath: "/pizzashack", Upstream: "echo:8080", Published: true}},
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
		r.Status = "pending"
		return nil
	}
	m.nextID++
	m.records[m.nextID] = &SubscriptionRecord{ID: m.nextID, AppID: appID, ProductID: productID, PlanID: planID, Status: "pending"}
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
func (m *memStore) SubscriptionStatus(_ context.Context, appID, productID int64) (string, error) {
	if r := m.findRecord(appID, productID); r != nil {
		return r.Status, nil
	}
	return "", nil
}

func (m *memStore) AdminSubscriptions(_ context.Context, statusFilter string, _ paging.Params) ([]AdminSubscriptionView, int, error) {
	out := []AdminSubscriptionView{}
	for _, r := range m.records {
		if statusFilter == "" || r.Status == statusFilter {
			out = append(out, AdminSubscriptionView{ID: r.ID, Status: r.Status})
		}
	}
	return out, len(out), nil
}

func TestSubscribeIsPendingAndDoesNotProvision(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "fixed-key" }, nil)

	cred, err := svc.Subscribe(ctx, 42, 3, 2)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if cred.APIKey != "fixed-key" || cred.ConsumerUsername != "app_42" {
		t.Fatalf("bad cred: %+v", cred)
	}
	if len(gw.Consumers) != 0 {
		t.Fatalf("expected no consumer provisioned on subscribe, got %v", gw.Consumers)
	}
	if len(gw.Routes) != 0 {
		t.Fatalf("expected no route provisioned on subscribe, got %v", gw.Routes)
	}
	r := store.findRecord(42, 3)
	if r == nil || r.Status != "pending" {
		t.Fatalf("expected a pending record, got %+v", r)
	}
}

func TestApproveProvisionsConsumerAndRoute(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "fixed-key" }, nil)

	if _, err := svc.Subscribe(ctx, 42, 3, 2); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	rec := store.findRecord(42, 3)
	if err := svc.Approve(ctx, rec.ID); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if store.records[rec.ID].Status != "active" {
		t.Fatalf("status = %q, want active", store.records[rec.ID].Status)
	}
	c, ok := gw.Consumers["app_42"]
	if !ok || c.Limit.Count != 100 {
		t.Fatalf("consumer not provisioned with plan limits: %+v", c)
	}
	r := gw.Routes["prod_3"]
	if len(r.Allowed) != 1 || r.Allowed[0] != "app_42" {
		t.Fatalf("route whitelist after approve: %+v", r.Allowed)
	}
}

func TestApproveDoesNotMarkActiveWhenRouteProvisioningFails(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	boom := errors.New("apisix down")
	gw := &routeFailGateway{Fake: apisix.NewFake(), err: boom}
	svc := NewService(store, gw, func() string { return "fixed-key" }, nil)

	if _, err := svc.Subscribe(ctx, 42, 3, 2); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	rec := store.findRecord(42, 3)
	err := svc.Approve(ctx, rec.ID)
	if !errors.Is(err, boom) {
		t.Fatalf("Approve: want route error, got %v", err)
	}
	// Invariant: active in DB ⇒ provisioned in gateway. Gateway provisioning
	// failed, so the subscription must NOT be active.
	if got := store.records[rec.ID].Status; got != StatusPending {
		t.Fatalf("status = %q, want pending (route provisioning failed)", got)
	}
}

func TestApproveRouteWhitelistIncludesNewlyApprovedConsumer(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "fixed-key" }, nil)

	// An existing active subscriber on the same product.
	store.creds[41] = Credential{ApplicationID: 41, APIKey: "k41", ConsumerUsername: "app_41"}
	store.records[500] = &SubscriptionRecord{ID: 500, AppID: 41, ProductID: 3, PlanID: 2, Status: "active"}

	if _, err := svc.Subscribe(ctx, 42, 3, 2); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	rec := store.findRecord(42, 3)
	if err := svc.Approve(ctx, rec.ID); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if store.records[rec.ID].Status != "active" {
		t.Fatalf("status = %q, want active", store.records[rec.ID].Status)
	}
	r := gw.Routes["prod_3"]
	seen := map[string]bool{}
	for _, a := range r.Allowed {
		seen[a] = true
	}
	if !seen["app_42"] {
		t.Fatalf("route whitelist missing newly approved consumer app_42: %+v", r.Allowed)
	}
	if !seen["app_41"] {
		t.Fatalf("route whitelist missing existing active consumer app_41: %+v", r.Allowed)
	}
	if len(r.Allowed) != 2 {
		t.Fatalf("route whitelist should have exactly 2 (no dupes): %+v", r.Allowed)
	}
}

func TestRejectSetsStatusAndDoesNotProvision(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "fixed-key" }, nil)

	if _, err := svc.Subscribe(ctx, 42, 3, 2); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	rec := store.findRecord(42, 3)
	if err := svc.Reject(ctx, rec.ID); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if store.records[rec.ID].Status != "rejected" {
		t.Fatalf("status = %q, want rejected", store.records[rec.ID].Status)
	}
	if _, ok := gw.Consumers["app_42"]; ok {
		t.Fatal("rejected subscription must not provision a consumer")
	}
	if r, ok := gw.Routes["prod_3"]; ok {
		for _, a := range r.Allowed {
			if a == "app_42" {
				t.Fatal("rejected app must not be in the route whitelist")
			}
		}
	}
}

func TestUnsubscribeRemovesFromWhitelist(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "k" }, nil)
	_, _ = svc.Subscribe(ctx, 42, 3, 2)
	_, _ = svc.Subscribe(ctx, 43, 3, 2)
	if err := svc.Approve(ctx, store.findRecord(42, 3).ID); err != nil {
		t.Fatalf("approve 42: %v", err)
	}
	if err := svc.Approve(ctx, store.findRecord(43, 3).ID); err != nil {
		t.Fatalf("approve 43: %v", err)
	}
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
	store.products[7] = ProductInfo{ID: 7, ContextPath: "/seven", Upstream: "echo:8080", Published: true}
	store.records[101] = &SubscriptionRecord{ID: 101, AppID: 1, ProductID: 7, PlanID: 2, Status: "active"}
	store.records[102] = &SubscriptionRecord{ID: 102, AppID: 2, ProductID: 7, PlanID: 2, Status: "active"}
	gw := apisix.NewFake()
	svc := NewService(store, gw, GenerateKey, nil)

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
	store.products[7] = ProductInfo{ID: 7, ContextPath: "/seven", Upstream: "echo:8080", Published: true}
	gw := apisix.NewFake()
	_ = gw.EnsureRoute(context.Background(), RouteID(7), "/seven/*", "echo:8080", nil)
	svc := NewService(store, gw, GenerateKey, nil)

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
	svc := NewService(store, gw, GenerateKey, nil)

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

func TestSubscribeRejectsUnpublishedProduct(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	store.products[8] = ProductInfo{ID: 8, ContextPath: "/x", Upstream: "echo:8080", Published: false}
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "k" }, nil)

	_, err := svc.Subscribe(ctx, 42, 8, 2)
	if err != ErrNotFound {
		t.Fatalf("Subscribe to unpublished product: want ErrNotFound, got %v", err)
	}
	if r := store.findRecord(42, 8); r != nil {
		t.Fatal("no record should be created for unpublished product")
	}
}

func TestSubscribeRejectsAlreadyActive(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "k" }, nil)

	if _, err := svc.Subscribe(ctx, 42, 3, 2); err != nil {
		t.Fatalf("first Subscribe: %v", err)
	}
	rec := store.findRecord(42, 3)
	if err := svc.Approve(ctx, rec.ID); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	_, err := svc.Subscribe(ctx, 42, 3, 2)
	if err != ErrAlreadySubscribed {
		t.Fatalf("second Subscribe (already active): want ErrAlreadySubscribed, got %v", err)
	}
}

func TestSubscribeAllowsResubscribeWhenRejected(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "k" }, nil)

	if _, err := svc.Subscribe(ctx, 42, 3, 2); err != nil {
		t.Fatalf("first Subscribe: %v", err)
	}
	rec := store.findRecord(42, 3)
	if err := svc.Reject(ctx, rec.ID); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if _, err := svc.Subscribe(ctx, 42, 3, 2); err != nil {
		t.Fatalf("re-subscribe after rejection: %v", err)
	}
	r := store.findRecord(42, 3)
	if r == nil || r.Status != StatusPending {
		t.Fatalf("expected pending record after re-subscribe, got %+v", r)
	}
}

func TestApproveRejectedReturnsInvalidTransition(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "k" }, nil)

	if _, err := svc.Subscribe(ctx, 42, 3, 2); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	rec := store.findRecord(42, 3)
	if err := svc.Reject(ctx, rec.ID); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if err := svc.Approve(ctx, rec.ID); err != ErrInvalidTransition {
		t.Fatalf("Approve of rejected: want ErrInvalidTransition, got %v", err)
	}
	if store.records[rec.ID].Status != StatusRejected {
		t.Fatalf("status should stay rejected, got %q", store.records[rec.ID].Status)
	}
	if _, ok := gw.Consumers["app_42"]; ok {
		t.Fatal("rejected subscription must not have provisioned a consumer")
	}
}

func TestRejectAlreadyRejectedReturnsInvalidTransition(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "k" }, nil)

	if _, err := svc.Subscribe(ctx, 42, 3, 2); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	rec := store.findRecord(42, 3)
	if err := svc.Reject(ctx, rec.ID); err != nil {
		t.Fatalf("first Reject: %v", err)
	}
	if err := svc.Reject(ctx, rec.ID); err != ErrInvalidTransition {
		t.Fatalf("second Reject: want ErrInvalidTransition, got %v", err)
	}
}
