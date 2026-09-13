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
type fakeTouchCounter struct {
	allowed     bool
	allowedErr  error
	recordFn    func(ctx context.Context, sub string, recordIDs []string) (int, error)
	recordedIDs []string
}

func (f *fakeTouchCounter) Allowed(ctx context.Context, sub string) (bool, error) {
	return f.allowed, f.allowedErr
}
func (f *fakeTouchCounter) RecordTouches(ctx context.Context, sub string, recordIDs []string) (int, error) {
	f.recordedIDs = append(f.recordedIDs, recordIDs...)
	if f.recordFn != nil {
		return f.recordFn(ctx, sub, recordIDs)
	}
	return len(f.recordedIDs), nil
}

// allowAllDeps builds a Deps whose collaborators all say yes — the
// baseline for a test that then overrides exactly one collaborator to
// exercise its own denial path.
func allowAllDeps() Deps {
	return Deps{
		Repo:         &fakeRepository{},
		Authz:        &fakeAuthorizer{allow: true},
		RateLimiter:  &fakeRateLimiter{allow: true},
		TouchCounter: &fakeTouchCounter{allowed: true},
	}
}
