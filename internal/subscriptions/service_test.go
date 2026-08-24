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
	svc := NewService(store, gw, nil, func() string { return "k" }, log)

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
	svc := NewService(store, gw, nil, func() string { return "k" }, &captureLogger{failOn: "subscribed"})

	// A failing activity log must not fail the subscribe it merely records.
	if _, err := svc.Subscribe(ctx, 42, 3, 2); err != nil {
		t.Fatalf("Subscribe must succeed despite a log failure, got %v", err)
	}
	if r := store.findRecord(42, 3); r == nil || r.Status != StatusPending {
		t.Fatalf("subscription must be persisted despite the log failure, got %+v", r)
	}
}

type memStore struct {
	creds            map[int64]Credential
	products         map[int64]ProductInfo
	plans            map[int64]PlanInfo
	records          map[int64]*SubscriptionRecord // subID -> record
	nextID           int64
	sandboxKeys      map[int64]string        // appID -> sandbox key
	sandboxProducts  map[int64][]ProductInfo // appID -> sandbox-enabled active products
	sandboxWhitelist map[int64][]string      // productID -> consumer usernames with sandbox key
	oidcClientIDs    map[int64]string        // appID -> oidc client id
	oauthWhitelist   map[int64][]string      // productID -> allowed client ids
	oauthProducts    map[int64][]ProductInfo // appID -> oauth2 products the app actively subscribes to
}

func newMemStore() *memStore {
	return &memStore{
		creds:            map[int64]Credential{},
		products:         map[int64]ProductInfo{3: {ID: 3, ContextPath: "/pizzashack", Upstream: "echo:8080", Published: true}},
		plans:            map[int64]PlanInfo{2: {ID: 2, Count: 100, WindowSeconds: 60}},
		records:          map[int64]*SubscriptionRecord{},
		sandboxKeys:      map[int64]string{},
		sandboxProducts:  map[int64][]ProductInfo{},
		sandboxWhitelist: map[int64][]string{},
		oidcClientIDs:    map[int64]string{},
		oauthWhitelist:   map[int64][]string{},
		oauthProducts:    map[int64][]ProductInfo{},
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

func (m *memStore) GetCredential(_ context.Context, appID int64) (Credential, error) {
	if c, ok := m.creds[appID]; ok {
		return c, nil
	}
	return Credential{}, ErrNotFound
}

func (m *memStore) ActivePlanForApp(_ context.Context, appID int64) (PlanInfo, error) {
	for _, r := range m.records {
		if r.AppID == appID && r.Status == StatusActive {
			if p, ok := m.plans[r.PlanID]; ok {
				return p, nil
			}
		}
	}
	return PlanInfo{}, ErrNoActiveSubscription
}

func (m *memStore) UpdateCredentialKey(_ context.Context, appID int64, newKey string) error {
	c, ok := m.creds[appID]
	if !ok {
		return ErrNotFound
	}
	c.APIKey = newKey
	m.creds[appID] = c
	return nil
}

func (m *memStore) GetSandboxKey(_ context.Context, appID int64) (string, error) {
	return m.sandboxKeys[appID], nil
}
func (m *memStore) UpdateSandboxKey(_ context.Context, appID int64, key string) error {
	m.sandboxKeys[appID] = key
	return nil
}
func (m *memStore) SandboxConsumersForProduct(_ context.Context, productID int64) ([]string, error) {
	return m.sandboxWhitelist[productID], nil
}
func (m *memStore) SandboxConsumersForPlan(_ context.Context, _ int64) ([]Credential, error) {
	return nil, nil
}
func (m *memStore) SandboxProductsForApp(_ context.Context, appID int64) ([]ProductInfo, error) {
	return m.sandboxProducts[appID], nil
}

func (m *memStore) OAuthClientsForProduct(_ context.Context, productID int64) ([]string, error) {
	return m.oauthWhitelist[productID], nil
}
func (m *memStore) OAuthProductsForApp(_ context.Context, appID int64) ([]ProductInfo, error) {
	return m.oauthProducts[appID], nil
}
func (m *memStore) GetAppOIDCClientID(_ context.Context, appID int64) (string, error) {
	return m.oidcClientIDs[appID], nil
}
func (m *memStore) SetAppOIDCClientID(_ context.Context, appID int64, clientID string) error {
	m.oidcClientIDs[appID] = clientID
	return nil
}
func (m *memStore) SubscriptionsForApp(_ context.Context, appID int64) ([]SubscriptionView, error) {
	var out []SubscriptionView
	for _, r := range m.records {
		if r.AppID == appID {
			out = append(out, SubscriptionView{ProductID: r.ProductID, PlanID: r.PlanID, Status: r.Status})
		}
	}
	return out, nil
}
func (m *memStore) DeleteApplication(_ context.Context, appID int64) error {
	for id, r := range m.records {
		if r.AppID == appID {
			delete(m.records, id)
		}
	}
	delete(m.creds, appID)
	delete(m.sandboxKeys, appID)
	delete(m.oidcClientIDs, appID)
	return nil
}

type fakeNotifier struct{ requested, approved, rejected int }

func (f *fakeNotifier) SubscriptionRequested(_, _, _ int64) { f.requested++ }
func (f *fakeNotifier) SubscriptionApproved(_, _, _ int64)  { f.approved++ }
func (f *fakeNotifier) SubscriptionRejected(_, _ int64)     { f.rejected++ }

func TestSubscribeNotifiesAdmins(t *testing.T) {
	store := newMemStore() // product 3 key-auth published, plan 2
	fn := &fakeNotifier{}
	svc := NewService(store, apisix.NewFake(), nil, func() string { return "k" }, nil)
	svc.SetNotifier(fn)
	if _, err := svc.Subscribe(context.Background(), 1, 3, 2); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if fn.requested != 1 {
		t.Fatalf("requested=%d want 1", fn.requested)
	}
}

func TestApproveRejectNotifyDeveloper(t *testing.T) {
	store := newMemStore()
	fn := &fakeNotifier{}
	svc := NewService(store, apisix.NewFake(), nil, func() string { return "k" }, nil)
	svc.SetNotifier(fn)
	// seed a pending subscription record the store can approve/reject (mirror the
	// existing approve/reject tests' record setup in this file)
	store.records[10] = &SubscriptionRecord{ID: 10, AppID: 1, ProductID: 3, PlanID: 2, Status: StatusPending}
	if err := svc.Approve(context.Background(), 10); err != nil {
		t.Fatalf("approve: %v", err)
	}
	store.records[11] = &SubscriptionRecord{ID: 11, AppID: 1, ProductID: 3, PlanID: 2, Status: StatusPending}
	if err := svc.Reject(context.Background(), 11); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if fn.approved != 1 || fn.rejected != 1 {
		t.Fatalf("approved=%d rejected=%d", fn.approved, fn.rejected)
	}
}

type fakeBiller struct{ calls [][3]int64 } // {appID, subID, planID}

func (f *fakeBiller) SubscriptionActivated(_ context.Context, appID, subID, planID int64) error {
	f.calls = append(f.calls, [3]int64{appID, subID, planID})
	return nil
}

func TestApproveFiresBillerKeyAuth(t *testing.T) {
	store := newMemStore() // product 3 key-auth published, plan 2
	fb := &fakeBiller{}
	svc := NewService(store, apisix.NewFake(), nil, func() string { return "k" }, nil)
	svc.SetBiller(fb)
	store.records[10] = &SubscriptionRecord{ID: 10, AppID: 11, ProductID: 3, PlanID: 2, Status: StatusPending}
	if err := svc.Approve(context.Background(), 10); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if len(fb.calls) != 1 || fb.calls[0] != [3]int64{11, 10, 2} {
		t.Fatalf("biller calls = %+v, want one {11,10,2}", fb.calls)
	}
}

func TestApproveFiresBillerOAuth2(t *testing.T) {
	store := newMemStore()
	store.products[9] = ProductInfo{ID: 9, ContextPath: "/orders", Upstream: "echo:8080", AuthType: "oauth2"}
	fb := &fakeBiller{}
	svc := NewService(store, apisix.NewFake(), nil, func() string { return "k" }, nil)
	svc.ConfigureOIDC("https://idp.example/realms/dev", "azp")
	svc.SetBiller(fb)
	store.records[20] = &SubscriptionRecord{ID: 20, AppID: 12, ProductID: 9, PlanID: 4, Status: StatusPending}
	if err := svc.Approve(context.Background(), 20); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if len(fb.calls) != 1 || fb.calls[0] != [3]int64{12, 20, 4} {
		t.Fatalf("biller calls = %+v, want one {12,20,4}", fb.calls)
	}
}

func TestNilNotifierSafe(t *testing.T) {
	store := newMemStore()
	svc := NewService(store, apisix.NewFake(), nil, func() string { return "k" }, nil)
	// no SetNotifier — must not panic
	if _, err := svc.Subscribe(context.Background(), 1, 3, 2); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
}

func TestSubscribeIsPendingAndDoesNotProvision(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, nil, func() string { return "fixed-key" }, nil)

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
	svc := NewService(store, gw, nil, func() string { return "fixed-key" }, nil)

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
	svc := NewService(store, gw, nil, func() string { return "fixed-key" }, nil)

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
	svc := NewService(store, gw, nil, func() string { return "fixed-key" }, nil)

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
	svc := NewService(store, gw, nil, func() string { return "fixed-key" }, nil)

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
	svc := NewService(store, gw, nil, func() string { return "k" }, nil)
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

func TestDeleteApplicationRemovesConsumerAndRoute(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, nil, func() string { return "k" }, nil)
	_, _ = svc.Subscribe(ctx, 42, 3, 2)
	_, _ = svc.Subscribe(ctx, 43, 3, 2)
	if err := svc.Approve(ctx, store.findRecord(42, 3).ID); err != nil {
		t.Fatalf("approve 42: %v", err)
	}
	if err := svc.Approve(ctx, store.findRecord(43, 3).ID); err != nil {
		t.Fatalf("approve 43: %v", err)
	}
	if _, ok := gw.Consumers["app_42"]; !ok {
		t.Fatal("setup: app_42 consumer should exist before delete")
	}

	if err := svc.DeleteApplication(ctx, 42); err != nil {
		t.Fatalf("DeleteApplication: %v", err)
	}

	if _, ok := gw.Consumers["app_42"]; ok {
		t.Error("app_42 consumer still present on the gateway after delete")
	}
	r := gw.Routes["prod_3"]
	if len(r.Allowed) != 1 || r.Allowed[0] != "app_43" {
		t.Fatalf("whitelist after delete: %+v (app_43's subscription must be untouched)", r.Allowed)
	}
	if _, err := store.GetCredential(ctx, 42); !errors.Is(err, ErrNotFound) {
		t.Errorf("credential for app 42 should be gone, got err=%v", err)
	}
	subs, err := store.SubscriptionsForApp(ctx, 42)
	if err != nil || len(subs) != 0 {
		t.Errorf("SubscriptionsForApp(42) = %+v, %v; want empty", subs, err)
	}
}

func TestDeleteApplicationRemovesSandboxConsumer(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	store.creds[42] = Credential{ApplicationID: 42, APIKey: "prodkey", ConsumerUsername: "app_42"}
	store.sandboxKeys[42] = "sbkey"
	prodGW, sbGW := apisix.NewFake(), apisix.NewFake()
	_ = prodGW.EnsureConsumer(ctx, "app_42", "prodkey", apisix.RateLimit{})
	_ = sbGW.EnsureConsumer(ctx, "app_42", "sbkey", apisix.RateLimit{})
	svc := NewService(store, prodGW, sbGW, func() string { return "k" }, nil)

	if err := svc.DeleteApplication(ctx, 42); err != nil {
		t.Fatalf("DeleteApplication: %v", err)
	}
	if _, ok := prodGW.Consumers["app_42"]; ok {
		t.Error("production consumer app_42 still present after delete")
	}
	if _, ok := sbGW.Consumers["app_42"]; ok {
		t.Error("sandbox consumer app_42 still present after delete")
	}
}

// DeleteApplication must not fail an app that never subscribed to anything —
// no credential was ever created, so there is no consumer to remove.
func TestDeleteApplicationNoCredentialNoop(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	svc := NewService(store, apisix.NewFake(), apisix.NewFake(), func() string { return "k" }, nil)

	if err := svc.DeleteApplication(ctx, 99); err != nil {
		t.Fatalf("DeleteApplication on a credential-less app: %v", err)
	}
}

func TestReprovisionRoute(t *testing.T) {
	store := newMemStore()
	store.products[7] = ProductInfo{ID: 7, ContextPath: "/seven", Upstream: "echo:8080", Published: true}
	store.records[101] = &SubscriptionRecord{ID: 101, AppID: 1, ProductID: 7, PlanID: 2, Status: "active"}
	store.records[102] = &SubscriptionRecord{ID: 102, AppID: 2, ProductID: 7, PlanID: 2, Status: "active"}
	gw := apisix.NewFake()
	svc := NewService(store, gw, nil, GenerateKey, nil)

	if err := svc.ReprovisionRoute(context.Background(), 7); err != nil {
		t.Fatalf("reprovision: %v", err)
	}
	r, ok := gw.Routes[RouteID(7)]
	if !ok {
		t.Fatalf("route %s not created", RouteID(7))
	}
	if r.Upstream != "echo:8080" || r.URI != "/seven" {
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
	svc := NewService(store, gw, nil, GenerateKey, nil)

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
	svc := NewService(store, gw, nil, GenerateKey, nil)

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
	svc := NewService(store, gw, nil, func() string { return "k" }, nil)

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
	svc := NewService(store, gw, nil, func() string { return "k" }, nil)

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
	svc := NewService(store, gw, nil, func() string { return "k" }, nil)

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
	svc := NewService(store, gw, nil, func() string { return "k" }, nil)

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
	svc := NewService(store, gw, nil, func() string { return "k" }, nil)

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

func TestRotateKey_HappyPath(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	store.creds[1] = Credential{ApplicationID: 1, APIKey: "old", ConsumerUsername: "app_1"}
	store.records[1] = &SubscriptionRecord{ID: 1, AppID: 1, ProductID: 3, PlanID: 2, Status: StatusActive}
	gw := apisix.NewFake()
	svc := NewService(store, gw, nil, func() string { return "newkey123" }, nil)

	got, err := svc.RotateKey(ctx, 1)
	if err != nil || got != "newkey123" {
		t.Fatalf("RotateKey = %q, %v", got, err)
	}
	if store.creds[1].APIKey != "newkey123" {
		t.Errorf("DB not updated with new key: %q", store.creds[1].APIKey)
	}
	if k := gw.Consumers["app_1"].APIKey; k != "newkey123" {
		t.Errorf("gateway consumer key = %q, want newkey123", k)
	}
}

func TestRotateKey_NoCredential(t *testing.T) {
	ctx := context.Background()
	svc := NewService(newMemStore(), apisix.NewFake(), nil, func() string { return "x" }, nil)
	if _, err := svc.RotateKey(ctx, 1); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("want ErrNoCredential, got %v", err)
	}
}

func TestRotateKey_NoActiveSubscription(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	store.creds[1] = Credential{ApplicationID: 1, APIKey: "old", ConsumerUsername: "app_1"}
	svc := NewService(store, apisix.NewFake(), nil, func() string { return "x" }, nil)
	if _, err := svc.RotateKey(ctx, 1); !errors.Is(err, ErrNoActiveSubscription) {
		t.Fatalf("want ErrNoActiveSubscription, got %v", err)
	}
}

func TestRotateKey_GatewayFailureKeepsOldKey(t *testing.T) {
	ctx := context.Background()
	gw := apisix.NewFake()
	gw.FailEnsureConsumer = true
	store := newMemStore()
	store.creds[1] = Credential{ApplicationID: 1, APIKey: "old", ConsumerUsername: "app_1"}
	store.records[1] = &SubscriptionRecord{ID: 1, AppID: 1, ProductID: 3, PlanID: 2, Status: StatusActive}
	svc := NewService(store, gw, nil, func() string { return "newkey123" }, nil)
	if _, err := svc.RotateKey(ctx, 1); err == nil {
		t.Fatal("expected gateway error")
	}
	if store.creds[1].APIKey != "old" {
		t.Errorf("DB key must NOT change on gateway failure, got %q", store.creds[1].APIKey)
	}
}

func TestEnableSandboxProvisionsConsumerAndRoutes(t *testing.T) {
	store := newMemStore()
	// Seed app 42 with a credential and an active plan subscription.
	store.creds[42] = Credential{ApplicationID: 42, APIKey: "prodkey", ConsumerUsername: "app_42"}
	store.records[1] = &SubscriptionRecord{ID: 1, AppID: 42, ProductID: 3, PlanID: 2, Status: StatusActive}
	// Product 9 must be in the store so reprovisionSandboxRoute can look it up.
	store.products[9] = ProductInfo{ID: 9, ContextPath: "/sb", SandboxUpstream: "echo:8080", Published: true}
	store.sandboxProducts[42] = []ProductInfo{{ID: 9, ContextPath: "/sb", SandboxUpstream: "echo:8080"}}
	store.sandboxWhitelist[9] = []string{"app_42"}
	prodGW, sbGW := apisix.NewFake(), apisix.NewFake()
	svc := NewService(store, prodGW, sbGW, func() string { return "sbkey" }, nil)

	key, err := svc.EnableSandbox(context.Background(), 42)
	if err != nil || key != "sbkey" {
		t.Fatalf("EnableSandbox = %q, %v", key, err)
	}
	if _, ok := sbGW.Consumers["app_42"]; !ok {
		t.Error("sandbox consumer app_42 not provisioned on the sandbox gateway")
	}
	if _, ok := prodGW.Consumers["app_42"]; ok {
		t.Error("production gateway must not be touched by EnableSandbox")
	}
	if store.sandboxKeys[42] != "sbkey" {
		t.Error("sandbox key not persisted")
	}
	// A sandbox route exists for product 9 on the sandbox gateway.
	if _, ok := sbGW.Routes[RouteID(9)]; !ok {
		t.Error("sandbox route for product 9 not provisioned")
	}
}

func TestEnableSandbox409WhenNoEligibleSubscription(t *testing.T) {
	store := newMemStore()
	// Seed app 42 with a credential and an active plan but NO sandbox-enabled products.
	store.creds[42] = Credential{ApplicationID: 42, APIKey: "prodkey", ConsumerUsername: "app_42"}
	store.records[1] = &SubscriptionRecord{ID: 1, AppID: 42, ProductID: 3, PlanID: 2, Status: StatusActive}
	svc := NewService(store, apisix.NewFake(), apisix.NewFake(), func() string { return "k" }, nil)
	if _, err := svc.EnableSandbox(context.Background(), 42); !errors.Is(err, ErrNoSandboxEligibleSubscription) {
		t.Fatalf("err = %v, want ErrNoSandboxEligibleSubscription", err)
	}
}

func TestRotateSandboxKey(t *testing.T) {
	store := newMemStore()
	store.sandboxKeys = map[int64]string{42: "old"}
	store.creds[42] = Credential{ApplicationID: 42, APIKey: "prodkey", ConsumerUsername: "app_42"}
	store.records[1] = &SubscriptionRecord{ID: 1, AppID: 42, ProductID: 3, PlanID: 2, Status: StatusActive}
	sbGW := apisix.NewFake()
	svc := NewService(store, apisix.NewFake(), sbGW, func() string { return "new" }, nil)

	key, err := svc.RotateSandboxKey(context.Background(), 42)
	if err != nil || key != "new" {
		t.Fatalf("RotateSandboxKey = %q, %v", key, err)
	}
	if store.sandboxKeys[42] != "new" {
		t.Error("sandbox key not updated in store")
	}
	if sbGW.Consumers["app_42"].APIKey != "new" {
		t.Errorf("sandbox gateway consumer key = %q, want new", sbGW.Consumers["app_42"].APIKey)
	}
}

func TestRotateSandboxKey409WhenNoKey(t *testing.T) {
	store := newMemStore()
	store.sandboxKeys = map[int64]string{} // no sandbox key
	store.creds[42] = Credential{ApplicationID: 42, APIKey: "prodkey", ConsumerUsername: "app_42"}
	svc := NewService(store, apisix.NewFake(), apisix.NewFake(), func() string { return "x" }, nil)
	if _, err := svc.RotateSandboxKey(context.Background(), 42); !errors.Is(err, ErrNoSandboxKey) {
		t.Fatalf("err = %v, want ErrNoSandboxKey", err)
	}
}

func TestReprovisionBranchesToOAuthRoute(t *testing.T) {
	store := newMemStore()
	store.products[9] = ProductInfo{ID: 9, ContextPath: "/orders", Upstream: "echo:8080", AuthType: "oauth2"}
	store.oauthWhitelist[9] = []string{"client-a"}
	gw := apisix.NewFake()
	svc := NewService(store, gw, nil, func() string { return "k" }, nil)
	svc.ConfigureOIDC("https://idp.example/realms/dev", "azp")
	if err := svc.ReprovisionRoute(context.Background(), 9); err != nil {
		t.Fatalf("reprovision: %v", err)
	}
	r, ok := gw.Routes[RouteID(9)]
	if !ok || len(r.Allowed) != 1 || r.Allowed[0] != "client-a" {
		t.Fatalf("oauth route not provisioned with whitelist: %+v", r)
	}
	if !r.OAuth {
		t.Fatalf("expected OAuth=true on the route, got false")
	}
}

func TestSetOIDCClientIDReprovisions(t *testing.T) {
	store := newMemStore()
	store.products[9] = ProductInfo{ID: 9, ContextPath: "/orders", Upstream: "echo:8080", AuthType: "oauth2"}
	store.oauthProducts[42] = []ProductInfo{store.products[9]}
	store.oauthWhitelist[9] = []string{"client-a"}
	gw := apisix.NewFake()
	svc := NewService(store, gw, nil, func() string { return "k" }, nil)
	svc.ConfigureOIDC("https://idp.example/realms/dev", "azp")
	if err := svc.SetOIDCClientID(context.Background(), 42, "client-a"); err != nil {
		t.Fatalf("SetOIDCClientID: %v", err)
	}
	if store.oidcClientIDs[42] != "client-a" {
		t.Fatalf("client id not persisted")
	}
	if _, ok := gw.Routes[RouteID(9)]; !ok {
		t.Fatalf("route not reprovisioned")
	}
}

func TestSetOIDCClientIDRejectsBadCharset(t *testing.T) {
	svc := NewService(newMemStore(), apisix.NewFake(), nil, func() string { return "k" }, nil)
	svc.ConfigureOIDC("https://idp.example", "azp")
	if err := svc.SetOIDCClientID(context.Background(), 42, `evil"]=true--`); !errors.Is(err, ErrInvalidClientID) {
		t.Fatalf("err = %v, want ErrInvalidClientID", err)
	}
}

// TestHasOAuthReadyConsumer covers the readiness check admin.Update uses to block
// an auth-type migration to oauth2 that would otherwise silently delete the route
// out from under subscribers who haven't registered an OIDC client id yet (#8).
func TestHasOAuthReadyConsumer(t *testing.T) {
	store := newMemStore()
	svc := NewService(store, apisix.NewFake(), nil, func() string { return "k" }, nil)

	ready, err := svc.HasOAuthReadyConsumer(context.Background(), 9)
	if err != nil {
		t.Fatalf("HasOAuthReadyConsumer: %v", err)
	}
	if ready {
		t.Fatal("expected not ready when no active subscriber has an oidc client id")
	}

	store.oauthWhitelist[9] = []string{"client-a"}
	ready, err = svc.HasOAuthReadyConsumer(context.Background(), 9)
	if err != nil {
		t.Fatalf("HasOAuthReadyConsumer: %v", err)
	}
	if !ready {
		t.Fatal("expected ready once an active subscriber has an oidc client id")
	}
}

// TestReprovisionOAuth2EmptyClientIDDoesNotCreateRoute is a regression test for the
// empty-extras filter fix: when an Approve carries a "" client id (the app has no
// OIDC client id yet) and the product has no other active oauth subscribers, the
// resulting whitelist is empty and the route must be DELETED, not created.
func TestReprovisionOAuth2EmptyClientIDDoesNotCreateRoute(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	store.products[9] = ProductInfo{ID: 9, ContextPath: "/orders", Upstream: "echo:8080", AuthType: "oauth2"}
	// oauthWhitelist[9] deliberately unset — no active subscribers with a client id.
	gw := apisix.NewFake()
	svc := NewService(store, gw, nil, func() string { return "k" }, nil)
	svc.ConfigureOIDC("https://idp.example/realms/dev", "azp")

	// Mirrors Approve for an oauth2 product where GetAppOIDCClientID returns "".
	if err := svc.reprovisionRoute(ctx, 9, ""); err != nil {
		t.Fatalf("reprovisionRoute: %v", err)
	}
	if _, ok := gw.Routes[RouteID(9)]; ok {
		t.Fatal("empty-whitelist oauth route must be deleted, not created")
	}
}

func TestSubscribeRejectsDeprecated(t *testing.T) {
	store := newMemStore()
	store.products[3] = ProductInfo{ID: 3, ContextPath: "/p", Upstream: "echo:8080", Published: true, LifecycleStatus: "deprecated"}
	svc := NewService(store, apisix.NewFake(), nil, func() string { return "k" }, nil)
	if _, err := svc.Subscribe(context.Background(), 1, 3, 2); !errors.Is(err, ErrProductDeprecated) {
		t.Fatalf("subscribe deprecated err = %v, want ErrProductDeprecated", err)
	}
	store.products[3] = ProductInfo{ID: 3, ContextPath: "/p", Upstream: "echo:8080", Published: true, LifecycleStatus: "sunset"}
	if _, err := svc.Subscribe(context.Background(), 1, 3, 2); !errors.Is(err, ErrProductDeprecated) {
		t.Fatalf("subscribe sunset err = %v, want ErrProductDeprecated", err)
	}
	store.products[3] = ProductInfo{ID: 3, ContextPath: "/p", Upstream: "echo:8080", Published: true, LifecycleStatus: "active"}
	if _, err := svc.Subscribe(context.Background(), 1, 3, 2); err != nil {
		t.Fatalf("subscribe active err = %v, want nil", err)
	}
}
