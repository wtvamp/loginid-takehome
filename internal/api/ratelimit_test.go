package api

import (
	"context"
	"testing"
	"time"
)

func TestInProcessRateLimiter_AllowsUpToLimit(t *testing.T) {
	rl := NewInProcessRateLimiter()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		allow, err := rl.Allow(ctx, "clientA", ScopeSearch, 3)
		if err != nil {
			t.Fatalf("Allow: %v", err)
		}
		if !allow {
			t.Fatalf("request %d denied, want allowed (limit 3)", i+1)
		}
	}
	allow, err := rl.Allow(ctx, "clientA", ScopeSearch, 3)
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if allow {
		t.Error("4th request within the same window should be denied at limit 3")
	}
}

func TestInProcessRateLimiter_KeyedPerSubAndScope(t *testing.T) {
	rl := NewInProcessRateLimiter()
	ctx := context.Background()
	if _, err := rl.Allow(ctx, "clientA", ScopeSearch, 1); err != nil {
		t.Fatalf("Allow: %v", err)
	}
	// A different sub, or a different scope for the same sub, must have
	// its own independent bucket.
	allowOtherSub, _ := rl.Allow(ctx, "clientB", ScopeSearch, 1)
	if !allowOtherSub {
		t.Error("a different sub must not share clientA's bucket")
	}
	allowOtherScope, _ := rl.Allow(ctx, "clientA", ScopeReadOwn, 1)
	if !allowOtherScope {
		t.Error("a different scope for the same sub must not share the bucket")
	}
}

func TestInProcessRateLimiter_WindowResets(t *testing.T) {
	rl := NewInProcessRateLimiter()
	fakeNow := time.Now()
	rl.now = func() time.Time { return fakeNow }
	ctx := context.Background()

	if allow, _ := rl.Allow(ctx, "clientA", ScopeSearch, 1); !allow {
		t.Fatal("first request should be allowed")
	}
	if allow, _ := rl.Allow(ctx, "clientA", ScopeSearch, 1); allow {
		t.Fatal("second request within the same window should be denied")
	}

	fakeNow = fakeNow.Add(time.Minute + time.Second)
	if allow, _ := rl.Allow(ctx, "clientA", ScopeSearch, 1); !allow {
		t.Error("request after the window rolled over should be allowed again")
	}
}
