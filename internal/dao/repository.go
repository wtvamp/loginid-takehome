package dao

import (
	"context"
	"time"

	"loginid-takehome/internal/model"
)

// ProfileRepository matches multi-db-strategy.md §3 exactly (types
// qualified with model. per §1a).
type ProfileRepository interface {
	Get(ctx context.Context, id string) (*model.UserProfile, error)
	Search(ctx context.Context, q ProfileQuery) (results []model.UserProfile, total int, err error)
	Create(ctx context.Context, p *model.UserProfile) (*model.UserProfile, error)
	Update(ctx context.Context, p *model.UserProfile) (*model.UserProfile, error)
	Upsert(ctx context.Context, p *model.UserProfile) (*model.UserProfile, error)
	Delete(ctx context.Context, id string) error
}

// CredentialRepository matches multi-db-strategy.md §3 exactly. Never
// accepts or returns a UserProfile field, in either direction — the
// PII/credential separation is a governance control (05-data-ops's
// pii-governance.md), not a modelling preference.
type CredentialRepository interface {
	GetByUsername(ctx context.Context, username string) (*model.UserCredential, error)
	ListByUserID(ctx context.Context, userID string) ([]model.UserCredential, error)
	Create(ctx context.Context, c *model.UserCredential) (*model.UserCredential, error)
	Update(ctx context.Context, c *model.UserCredential) (*model.UserCredential, error)
	Delete(ctx context.Context, id string) error
}

// AuthMethodRepository matches multi-db-strategy.md §3 exactly.
type AuthMethodRepository interface {
	Get(ctx context.Context, id string) (*model.AuthMethod, error)
	GetByName(ctx context.Context, name string) (*model.AuthMethod, error)
	List(ctx context.Context, activeOnly bool) ([]model.AuthMethod, error)
}

// Repository is the composite interface agreed directly with 05 (§1) and
// extended by the §3b (CreateProfileWithCredential) and §3c (DeleteExpired,
// DeleteProfile) addenda. It stays an interface, not a struct with exported
// fields, so it's mockable for this track's test-double strategy
// (decisions/test-double-strategy.md). The three accessors stay distinct
// rather than flattening into one interface because that separation is a
// PII/credential governance control: a call site must reach for the
// credential accessor deliberately.
type Repository interface {
	Profiles() ProfileRepository
	Credentials() CredentialRepository
	Methods() AuthMethodRepository

	// CreateProfileWithCredential is the one cross-table write in the
	// contract besides the retention methods below — both rows in a
	// single transaction. c.UserID is ignored on input and set from the
	// profile just created; see PrepareCreateProfileWithCredential.
	CreateProfileWithCredential(ctx context.Context, p *model.UserProfile, c *model.UserCredential) (*model.UserProfile, *model.UserCredential, error)

	// DeleteExpired batches a retention sweep for one class at a time
	// (never a mixed predicate), bounded by maxRows per call so a single
	// sweep never holds one unbounded transaction. Never call with a
	// request-scoped context — a sweep must not be silently cancelled
	// because an unrelated inbound request ended.
	DeleteExpired(ctx context.Context, class RetentionClass, olderThan time.Time, maxRows int) (SweepResult, error)

	// DeleteProfile is the only path for a subject-deletion request;
	// reason is asserted internally as "subject_request" by the backend,
	// never caller-supplied (05's ruling on Oren's F21 review finding #3).
	DeleteProfile(ctx context.Context, id string, externalRef *string) error

	Close() error
}
