package apisix

import (
	"context"
	"errors"
	"testing"
)

func TestSwappableDelegatesAndSwaps(t *testing.T) {
	ctx := context.Background()
	f1, f2 := NewFake(), NewFake()
	sw := NewSwappable(f1)
	if !sw.Enabled() {
		t.Fatal("non-nil inner must be enabled")
	}

	if err := sw.EnsureConsumer(ctx, "u1", "k1", RateLimit{Count: 5, WindowSeconds: 60}); err != nil {
		t.Fatalf("delegate: %v", err)
	}
	// f1 must have recorded the call; f2 must not have seen it at all.
	if got, ok := f1.Consumers["u1"]; !ok || got.APIKey != "k1" {
		t.Fatalf("f1 did not record first call: %+v", f1.Consumers)
	}
	if _, ok := f2.Consumers["u1"]; ok {
		t.Fatal("f2 must not receive calls made before Swap")
	}

	sw.Swap(f2)

	if err := sw.EnsureConsumer(ctx, "u2", "k2", RateLimit{Count: 10, WindowSeconds: 30}); err != nil {
		t.Fatalf("post-swap delegate: %v", err)
	}
	// f2 must have recorded the post-swap call; f1 must not have seen it.
	if got, ok := f2.Consumers["u2"]; !ok || got.APIKey != "k2" {
		t.Fatalf("f2 did not record post-swap call: %+v", f2.Consumers)
	}
	if _, ok := f1.Consumers["u2"]; ok {
		t.Fatal("f1 must not receive calls made after Swap")
	}

	if err := sw.DeleteConsumer(ctx, "u2"); err != nil {
		t.Fatalf("delete delegate: %v", err)
	}
	if _, ok := f2.Consumers["u2"]; ok {
		t.Fatal("DeleteConsumer after swap must hit f2, but u2 still present")
	}
	// f1's earlier consumer must be untouched by calls routed to f2.
	if _, ok := f1.Consumers["u1"]; !ok {
		t.Fatal("f1's prior state must be unaffected by post-swap calls")
	}
}

func TestSwappableDisabled(t *testing.T) {
	sw := NewSwappable(nil)
	if sw.Enabled() {
		t.Fatal("nil inner must be disabled")
	}
	if err := sw.EnsureRoute(context.Background(), "r", "/x", "u:80", nil); !errors.Is(err, ErrGatewayDisabled) {
		t.Fatalf("disabled call: %v", err)
	}
	if err := sw.DeleteRoute(context.Background(), "r"); !errors.Is(err, ErrGatewayDisabled) {
		t.Fatalf("disabled DeleteRoute: %v", err)
	}
	if err := sw.EnsureConsumer(context.Background(), "u", "k", RateLimit{}); !errors.Is(err, ErrGatewayDisabled) {
		t.Fatalf("disabled EnsureConsumer: %v", err)
	}
	if err := sw.DeleteConsumer(context.Background(), "u"); !errors.Is(err, ErrGatewayDisabled) {
		t.Fatalf("disabled DeleteConsumer: %v", err)
	}
	if err := sw.EnsureOAuthRoute(context.Background(), "r", "/x", "u:80", "iss", "claim", nil); !errors.Is(err, ErrGatewayDisabled) {
		t.Fatalf("disabled EnsureOAuthRoute: %v", err)
	}

	sw.Swap(NewFake())
	if !sw.Enabled() {
		t.Fatal("swap-in must enable")
	}
	if err := sw.EnsureRoute(context.Background(), "r", "/x", "u:80", nil); err != nil {
		t.Fatalf("post swap-in delegate: %v", err)
	}
}
