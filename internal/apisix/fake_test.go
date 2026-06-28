package apisix

import (
	"context"
	"testing"
)

func TestFakeRecordsConsumersAndRoutes(t *testing.T) {
	ctx := context.Background()
	f := NewFake()
	if err := f.EnsureConsumer(ctx, "app_1", "key-abc", RateLimit{Count: 60, WindowSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	if f.Consumers["app_1"].APIKey != "key-abc" || f.Consumers["app_1"].Limit.Count != 60 {
		t.Fatalf("consumer not recorded: %+v", f.Consumers["app_1"])
	}
	if err := f.EnsureRoute(ctx, "prod_3", "/pizzashack", "echo:8080", []string{"app_1"}); err != nil {
		t.Fatal(err)
	}
	if got := f.Routes["prod_3"].Allowed; len(got) != 1 || got[0] != "app_1" {
		t.Fatalf("route whitelist not recorded: %+v", got)
	}
	if err := f.DeleteConsumer(ctx, "app_1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Consumers["app_1"]; ok {
		t.Fatal("consumer not deleted")
	}
}

func TestFakeDeleteRoute(t *testing.T) {
	f := NewFake()
	if err := f.EnsureRoute(context.Background(), "prod_1", "/x", "echo:8080", nil); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := f.DeleteRoute(context.Background(), "prod_1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := f.Routes["prod_1"]; ok {
		t.Fatal("route prod_1 still present after DeleteRoute")
	}
	// Deleting a missing route is a no-op, not an error.
	if err := f.DeleteRoute(context.Background(), "prod_missing"); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
}
