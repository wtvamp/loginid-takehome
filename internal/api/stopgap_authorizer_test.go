package api

import (
	"context"
	"testing"
)

// TestStopgapAuthorizer_ReadOwn_AllowedAndDenied is Marcus Ilori's (02)
// ruling on the review finding that this stand-in's deny path had no
// test coverage: exercise both the allow and deny outcomes for
// ScopeReadOwn, the only scope this stopgap actually gates.
func TestStopgapAuthorizer_ReadOwn_AllowedAndDenied(t *testing.T) {
	a := &StopgapAuthorizer{
		ReadOwnAllow: map[string]map[string]bool{
			"qa-client": {"profile-1": true},
		},
	}
	ctx := context.Background()

	allowed, err := a.Authorize(ctx, "qa-client", ScopeReadOwn, "profile-1")
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if !allowed {
		t.Error("qa-client reading its seeded profile-1 should be allowed")
	}

	denied, err := a.Authorize(ctx, "qa-client", ScopeReadOwn, "profile-2")
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if denied {
		t.Error("qa-client reading a profile it was not seeded with should be denied")
	}

	deniedSub, err := a.Authorize(ctx, "some-other-client", ScopeReadOwn, "profile-1")
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if deniedSub {
		t.Error("a sub with no seeded entry at all should be denied, the safe default")
	}
}

// TestStopgapAuthorizer_EmptyMapDeniesEverything is the safe-default
// requirement itself: a StopgapAuthorizer constructed with no seed data
// (the out-of-the-box state before LT-51 seeds a QA client credential)
// must deny every ScopeReadOwn request, never fail open.
func TestStopgapAuthorizer_EmptyMapDeniesEverything(t *testing.T) {
	a := &StopgapAuthorizer{}
	allowed, err := a.Authorize(context.Background(), "anyone", ScopeReadOwn, "anything")
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if allowed {
		t.Error("an unseeded StopgapAuthorizer must deny every read:own request, not fail open")
	}
}

func TestStopgapAuthorizer_ReadAnyAndSearch_AlwaysAllowed(t *testing.T) {
	a := &StopgapAuthorizer{}
	ctx := context.Background()

	if allowed, _ := a.Authorize(ctx, "anyone", ScopeReadAny, "anything"); !allowed {
		t.Error("read:any has no per-record referral concept this stopgap gates — should not be denied here")
	}
	if allowed, _ := a.Authorize(ctx, "anyone", ScopeSearch, ""); !allowed {
		t.Error("search has no per-record target this stopgap gates — should not be denied here")
	}
}
