package conformance

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"

	"loginid-takehome/internal/dao"
)

// runAgainstAllBackends runs fn as a subtest per backend, giving each its
// own fresh fixture. This is the "one table-driven test, not three
// copies" shape LT-38's refinement doc requires. registerFixture is
// called before fn so the admin-only helpers below (createAuthMethod,
// deactivateMethod, countProfiles) can look up the matching admin *sql.DB
// from just the Repository fn was handed — these tests never run
// parallel subtests, so there's exactly one live fixture per repo at a
// time.
func runAgainstAllBackends(t *testing.T, fn func(t *testing.T, repo dao.Repository)) {
	t.Helper()
	for _, b := range backends {
		b := b
		t.Run(b.driver, func(t *testing.T) {
			fx := b.newFix(t)
			registerFixture(fx)
			fn(t, fx.repo)
		})
	}
}

// fixtureByRepoMu guards fixtureByRepo. The package comment on
// runAgainstAllBackends still states the actual invariant — subtests
// never run in parallel today, so there's exactly one live fixture at a
// time — but that invariant lives in a comment, and the next reasonable
// change to a three-live-database suite is t.Parallel() to cut wall-clock
// time, which would make an unguarded map write race silently (invisible
// without -race). The lock costs nothing today and turns tomorrow's
// silent race into tomorrow's ordinary, expected synchronization (Nolan
// Reyes, PR #16 review).
var (
	fixtureByRepoMu sync.Mutex
	fixtureByRepo   = map[dao.Repository]fixture{}
)

func registerFixture(fx fixture) {
	fixtureByRepoMu.Lock()
	defer fixtureByRepoMu.Unlock()
	fixtureByRepo[fx.repo] = fx
}

func fixtureFor(t *testing.T, repo dao.Repository) fixture {
	t.Helper()
	fixtureByRepoMu.Lock()
	fx, ok := fixtureByRepo[repo]
	fixtureByRepoMu.Unlock()
	if !ok {
		t.Fatal("fixtureFor: no fixture registered for this repo — internal test-helper bug")
	}
	return fx
}

// seededMethodIDFor looks up the 'password' auth_method every backend's
// setup seeds identically.
func seededMethodIDFor(t *testing.T, repo dao.Repository) string {
	t.Helper()
	m, err := repo.Methods().GetByName(context.Background(), "password")
	if err != nil {
		t.Fatalf("looking up seeded 'password' auth_method: %v", err)
	}
	return m.ID
}

var (
	authMethodCounterMu sync.Mutex
	authMethodCounter   int
)

func nextAuthMethodCounter() int {
	authMethodCounterMu.Lock()
	defer authMethodCounterMu.Unlock()
	authMethodCounter++
	return authMethodCounter
}

// createAuthMethod inserts a new auth_method row directly against the
// backend's admin connection, since AuthMethodRepository is read-only per
// the contract ("writes to this table are an operational action, not
// part of the normal request path" — multi-db-strategy.md).
func createAuthMethod(t *testing.T, repo dao.Repository, namePrefix string, requiresSecret bool) (string, error) {
	t.Helper()
	fx := fixtureFor(t, repo)
	name := fmt.Sprintf("%s-%d", namePrefix, nextAuthMethodCounter())

	id := uuid.NewString()
	var query string
	var args []any
	if fx.driver == "sqlite" {
		query = "INSERT INTO auth_method (id, name, requires_secret, is_active, created_at) VALUES (?, ?, ?, 1, '2026-01-01T00:00:00Z')"
		args = []any{id, name, boolToInt(requiresSecret)}
	} else {
		query = "INSERT INTO auth_method (id, name, requires_secret, is_active, created_at) VALUES ($1, $2, $3, true, now())"
		args = []any{id, name, requiresSecret}
	}
	_, err := fx.admin.Exec(query, args...)
	return id, err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// deactivateMethod flips is_active to false for the given method id.
func deactivateMethod(t *testing.T, repo dao.Repository, methodID string) error {
	t.Helper()
	fx := fixtureFor(t, repo)
	if fx.driver == "sqlite" {
		_, err := fx.admin.Exec("UPDATE auth_method SET is_active = 0 WHERE id = ?", methodID)
		return err
	}
	_, err := fx.admin.Exec("UPDATE auth_method SET is_active = false WHERE id = $1", methodID)
	return err
}

// countProfiles returns the current row count in user_profile.
func countProfiles(t *testing.T, repo dao.Repository) int {
	t.Helper()
	fx := fixtureFor(t, repo)
	var n int
	if err := fx.admin.QueryRow("SELECT COUNT(*) FROM user_profile").Scan(&n); err != nil {
		t.Fatalf("counting user_profile rows: %v", err)
	}
	return n
}

// countDeletionLogRowsForProfile returns the number of deletion_log rows
// naming profileID — the out-of-interface check TestConformance_
// DeleteWritesExactlyOneDeletionLogRow needs, since neither DeleteProfile
// nor DeleteExpired returns a row count for its own log write.
func countDeletionLogRowsForProfile(t *testing.T, repo dao.Repository, profileID string) int {
	t.Helper()
	fx := fixtureFor(t, repo)
	query := "SELECT COUNT(*) FROM deletion_log WHERE profile_id = $1"
	if fx.driver == "sqlite" {
		query = "SELECT COUNT(*) FROM deletion_log WHERE profile_id = ?"
	}
	var n int
	if err := fx.admin.QueryRow(query, profileID).Scan(&n); err != nil {
		t.Fatalf("counting deletion_log rows for %s: %v", profileID, err)
	}
	return n
}
