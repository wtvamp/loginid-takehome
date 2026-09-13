package api

import "context"

// StopgapAuthorizer is a TEMPORARY, default-deny Authorizer stand-in —
// NOT the real caller_referral-backed policy engine (handoff-03-auth.md
// v3's F8 resolution names that as a later story's own datastore work).
//
// Ruled by Marcus Ilori (02), in response to a direct question before
// this was wired to a live public URL: "A permissive allow-all
// Authorizer must never be deployed to a live public URL on a shared
// production cluster, 'temporary' or not — that's the exact failure this
// design exists to prevent, and it's a public repo, so being
// permissive-even-briefly is a permanent public fact." Ruling: default-
// deny, in-memory, hard-coded with a small fixed (sub, allowed record id)
// map for the seeded QA client credential (LT-51) — enough to
// demonstrate both the allow and deny paths of read:own's referral check
// in the live review, without ever allowing an arbitrary caller through.
//
// This only affects ScopeReadOwn's referral-membership check.
// ScopeReadAny's authorization is "has a valid reason_code," already
// checked in the handler before Authorize() is ever called; ScopeSearch
// has no per-record target for this Authorizer to gate at all. Both
// return true unconditionally here — there is no real caller_referral
// data this stopgap could consult for a real decision on those scopes
// either way, so denying them would not be more correct, only
// differently placeholder.
//
// A follow-up story replacing this with the real datastore-backed
// implementation must exist before this is considered load-bearing
// beyond the live-URL review — flag to the PM if it doesn't.
type StopgapAuthorizer struct {
	// ReadOwnAllow maps a caller sub to the set of record ids that sub
	// may read under ScopeReadOwn — the hard-coded "referral" set this
	// stopgap stands in for. Empty/nil means the map denies everything,
	// which is the safe default for any sub not explicitly seeded here.
	ReadOwnAllow map[string]map[string]bool
}

func (a *StopgapAuthorizer) Authorize(_ context.Context, sub string, scope Scope, target string) (bool, error) {
	switch scope {
	case ScopeReadOwn:
		allowed, ok := a.ReadOwnAllow[sub]
		if !ok {
			return false, nil
		}
		return allowed[target], nil
	case ScopeReadAny, ScopeSearch:
		return true, nil
	default:
		return false, nil
	}
}
