package api

import (
	"context"
	"time"

	"loginid-takehome/internal/dao"
	"loginid-takehome/internal/model"
)

// Hand-written fakes, per decisions/test-double-strategy.md: any change
// to Repository or its three sub-interfaces must update the corresponding
// fake in the same PR — a review-checklist item, not codegen.

// fakeRepository implements dao.Repository. Only Profiles() is exercised
// by this package's handlers; the rest panic if ever called, so a test
// that reaches them fails loudly instead of silently zero-valuing.
type fakeRepository struct {
	profiles fakeProfileRepository
}

func (f *fakeRepository) Profiles() dao.ProfileRepository       { return &f.profiles }
func (f *fakeRepository) Credentials() dao.CredentialRepository { panic("not used by internal/api") }
func (f *fakeRepository) Methods() dao.AuthMethodRepository     { panic("not used by internal/api") }
func (f *fakeRepository) CreateProfileWithCredential(ctx context.Context, p *model.UserProfile, c *model.UserCredential) (*model.UserProfile, *model.UserCredential, error) {
	panic("not used by internal/api")
}
func (f *fakeRepository) DeleteExpired(ctx context.Context, class dao.RetentionClass, olderThan time.Time, maxRows int) (dao.SweepResult, error) {
	panic("not used by internal/api")
}
func (f *fakeRepository) DeleteProfile(ctx context.Context, id string, externalRef *string) error {
	panic("not used by internal/api")
}
func (f *fakeRepository) Close() error { return nil }

// fakeProfileRepository implements dao.ProfileRepository with
// configurable funcs per test, per the ruled test-double strategy.
type fakeProfileRepository struct {
	getFn    func(ctx context.Context, id string) (*model.UserProfile, error)
	searchFn func(ctx context.Context, q dao.ProfileQuery) ([]model.UserProfile, int, error)
}

func (f *fakeProfileRepository) Get(ctx context.Context, id string) (*model.UserProfile, error) {
	return f.getFn(ctx, id)
}
func (f *fakeProfileRepository) Search(ctx context.Context, q dao.ProfileQuery) ([]model.UserProfile, int, error) {
	return f.searchFn(ctx, q)
}
func (f *fakeProfileRepository) Create(ctx context.Context, p *model.UserProfile) (*model.UserProfile, error) {
	panic("not used by internal/api")
}
func (f *fakeProfileRepository) Update(ctx context.Context, p *model.UserProfile) (*model.UserProfile, error) {
	panic("not used by internal/api")
}
func (f *fakeProfileRepository) Upsert(ctx context.Context, p *model.UserProfile) (*model.UserProfile, error) {
	panic("not used by internal/api")
}
func (f *fakeProfileRepository) Delete(ctx context.Context, id string) error {
	panic("not used by internal/api")
}

// fakeAuthorizer implements Authorizer with a configurable decision.
type fakeAuthorizer struct {
	allow bool
	err   error
}

func (f *fakeAuthorizer) Authorize(ctx context.Context, sub string, scope Scope, target string) (bool, error) {
	return f.allow, f.err
}

// fakeRateLimiter implements RateLimiter with a configurable decision —
// tests exercising the rate-limit path don't need real wall-clock timing.
type fakeRateLimiter struct {
	allow bool
	err   error
}

func (f *fakeRateLimiter) Allow(ctx context.Context, sub string, scope Scope, limit int) (bool, error) {
	return f.allow, f.err
}

// fakeTouchCounter implements TouchCounter with configurable behavior.
// reserveOK controls Reserve's decision; settledIDs/settledReserved
// record what Settle was called with, for tests asserting the
// reserve/settle pair is used correctly (in particular that a denied or
// failed request still releases its reservation).
type fakeTouchCounter struct {
	reserveOK       bool
	reserveErr      error
	settleErr       error
	settleFn        func(ctx context.Context, sub string, recordIDs []string, reserved int) error
	settledIDs      []string
	settledReserved []int
}

func (f *fakeTouchCounter) Reserve(ctx context.Context, sub string, maxNewTouches int) (bool, error) {
	return f.reserveOK, f.reserveErr
}
func (f *fakeTouchCounter) Settle(ctx context.Context, sub string, recordIDs []string, reserved int) error {
	f.settledIDs = append(f.settledIDs, recordIDs...)
	f.settledReserved = append(f.settledReserved, reserved)
	if f.settleFn != nil {
		return f.settleFn(ctx, sub, recordIDs, reserved)
	}
	return f.settleErr
}

// fakeAuditLogger records every event logged, for tests asserting which
// AuditEventKind a given code path emits — in particular that two paths
// producing an identical response body (a scope failure and an
// authorize() denial, per F7) still emit distinguishable audit events.
type fakeAuditLogger struct {
	events []AuditEvent
}

func (f *fakeAuditLogger) Log(ctx context.Context, event AuditEvent) {
	f.events = append(f.events, event)
}

// allowAllDeps builds a Deps whose collaborators all say yes — the
// baseline for a test that then overrides exactly one collaborator to
// exercise its own denial path.
func allowAllDeps() Deps {
	return Deps{
		Repo:         &fakeRepository{},
		Authz:        &fakeAuthorizer{allow: true},
		RateLimiter:  &fakeRateLimiter{allow: true},
		TouchCounter: &fakeTouchCounter{reserveOK: true},
		AuditLog:     &fakeAuditLogger{},
	}
}
