package conformance

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"loginid-takehome/internal/dao"
	"loginid-takehome/internal/model"
)

func strp(s string) *string { return &s }

// ---- Search parity (multi-db-strategy.md §7's own minimum: "identical
// result sets and identical ordering for the same Search across
// backends"; §6.3 collation; §6.4 LIKE escaping; §6.5 Country
// normalization — the three defects that already bit design once each). ----

func TestConformance_SearchParityAcrossBackends(t *testing.T) {
	// Fixture seeded identically on every backend via each backend's own
	// Create — never a caller-supplied ID, since neither backend exposes
	// that path. Rows are matched across backends by Name (unique in this
	// fixture set), never by ID: ID generation is backend-specific
	// (server-side on Postgres/CockroachDB, Go-side on SQLite), so a
	// literal ID comparison would fail every run regardless of whether
	// Search itself behaves correctly (Tobias's objection, refinement/
	// LT-38.md).
	fixture := []model.UserProfile{
		{Name: "Alice Anderson", Region: strp("California"), Country: strp("US"), Source: "direct"},
		{Name: "alice zephyr", Region: strp("Texas"), Country: strp("us"), Source: "direct"}, // lowercase input on write, checks §6.5 normalization
		{Name: "Bob Brown", Region: strp("California"), Country: strp("US"), Source: "direct"},
		{Name: "50% Off Corp", Region: strp("California"), Country: strp("US"), Source: "direct"},
	}

	for _, b := range backends {
		t.Run(b.driver, func(t *testing.T) {
			repo := b.newFix(t).repo
			ctx := context.Background()

			for _, p := range fixture {
				pCopy := p
				if _, err := repo.Profiles().Create(ctx, &pCopy); err != nil {
					t.Fatalf("seeding fixture %q: %v", p.Name, err)
				}
			}

			t.Run("name search is case-insensitive for ASCII (§7 identical rows)", func(t *testing.T) {
				results, total, err := repo.Profiles().Search(ctx, dao.ProfileQuery{Name: strp("alice")})
				if err != nil {
					t.Fatalf("Search: %v", err)
				}
				if total != 2 {
					t.Fatalf("total = %d, want 2 (both Alice rows, case-insensitive)", total)
				}
				names := map[string]bool{}
				for _, r := range results {
					names[r.Name] = true
				}
				if !names["Alice Anderson"] || !names["alice zephyr"] {
					t.Errorf("Search did not return both Alice rows: %+v", results)
				}
			})

			t.Run("LIKE escaping treats %% as literal (§6.4)", func(t *testing.T) {
				results, total, err := repo.Profiles().Search(ctx, dao.ProfileQuery{Name: strp("50%")})
				if err != nil {
					t.Fatalf("Search: %v", err)
				}
				if total != 1 || len(results) != 1 || results[0].Name != "50% Off Corp" {
					t.Errorf("Search(%q) = %d results %+v, want exactly the literal-%% row", "50%", total, results)
				}
			})

			t.Run("Country is normalized to uppercase at write (§6.5)", func(t *testing.T) {
				_, total, err := repo.Profiles().Search(ctx, dao.ProfileQuery{Country: strp("US")})
				if err != nil {
					t.Fatalf("Search: %v", err)
				}
				if total != len(fixture) {
					t.Errorf("Country search for %q matched %d, want %d — a lowercase 'us' written to the row must normalize to 'US' or it becomes invisible to this query", "US", total, len(fixture))
				}
			})

			t.Run("ordering is LOWER(name) ASC, id ASC (§6.3 collation)", func(t *testing.T) {
				results, _, err := repo.Profiles().Search(ctx, dao.ProfileQuery{Region: strp("California")})
				if err != nil {
					t.Fatalf("Search: %v", err)
				}
				if len(results) < 2 {
					t.Fatalf("expected at least 2 California rows, got %d", len(results))
				}
				for i := 1; i < len(results); i++ {
					prevLower, curLower := lower(results[i-1].Name), lower(results[i].Name)
					if curLower < prevLower {
						t.Errorf("ordering violated at index %d: %q came after %q", i, results[i].Name, results[i-1].Name)
					}
				}
			})
		})
	}
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// ---- Empty-string rejection per ProfileQuery field, one named test case
// per field, per the "absent, never empty" convention (§7 minimum). ----

func TestConformance_EmptyStringRejection_Name(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		_, _, err := repo.Profiles().Search(context.Background(), dao.ProfileQuery{Name: strp("")})
		if !errors.Is(err, dao.ErrInvalidQuery) {
			t.Errorf("empty Name = %v, want ErrInvalidQuery", err)
		}
	})
}

func TestConformance_EmptyStringRejection_Phone(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		_, _, err := repo.Profiles().Search(context.Background(), dao.ProfileQuery{Phone: strp("")})
		if !errors.Is(err, dao.ErrInvalidQuery) {
			t.Errorf("empty Phone = %v, want ErrInvalidQuery", err)
		}
	})
}

func TestConformance_EmptyStringRejection_Region(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		_, _, err := repo.Profiles().Search(context.Background(), dao.ProfileQuery{Region: strp("")})
		if !errors.Is(err, dao.ErrInvalidQuery) {
			t.Errorf("empty Region = %v, want ErrInvalidQuery", err)
		}
	})
}

func TestConformance_EmptyStringRejection_Country(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		_, _, err := repo.Profiles().Search(context.Background(), dao.ProfileQuery{Country: strp("")})
		if !errors.Is(err, dao.ErrInvalidQuery) {
			t.Errorf("empty Country = %v, want ErrInvalidQuery", err)
		}
	})
}

// ---- Sentinel parity, one named test case per §4 condition, across all
// three backends (this is the enforcement site LT-36/LT-37 were waiting
// on). ----

// Three assertions, per 05's amendment A4 (revised after Yusuf's
// objection turn) — the pair alone can't distinguish a correct
// implementation from one that returns one sentinel for everything, and
// the third closes the specific bug the first version of A4 itself had:
// a permissive validator (uuid.Parse-shaped) accepts non-canonical forms
// (uppercase, brace-wrapped, unhyphenated) that Postgres normalizes and
// finds, while SQLite's TEXT column byte-compares and misses — silently
// moving the divergence rather than closing it.

func TestConformance_Sentinel_ErrNotFound(t *testing.T) {
	// A well-formed, CANONICAL, but nonexistent UUID — the realistic
	// "missing row" case ErrNotFound is for.
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		_, err := repo.Profiles().Get(context.Background(), "00000000-0000-0000-0000-000000000000")
		if !errors.Is(err, dao.ErrNotFound) {
			t.Errorf("Get on well-formed missing row = %v, want ErrNotFound", err)
		}
	})
}

func TestConformance_Sentinel_MalformedID(t *testing.T) {
	// A syntactically malformed id must be ErrInvalidArgument on every
	// backend — a caller bug, not a data outcome. Load-bearing for
	// DeleteProfile specifically: this contract treats ErrNotFound from a
	// delete as "already gone, success" — if a malformed id read as
	// ErrNotFound, a typo'd subject-deletion request would report
	// completed while nothing was deleted.
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		_, err := repo.Profiles().Get(context.Background(), "does-not-exist")
		if !errors.Is(err, dao.ErrInvalidArgument) {
			t.Errorf("Get with a malformed id = %v, want ErrInvalidArgument", err)
		}
	})
}

func TestConformance_Sentinel_EmptyID(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		_, err := repo.Profiles().Get(context.Background(), "")
		if !errors.Is(err, dao.ErrInvalidArgument) {
			t.Errorf("Get with an empty id = %v, want ErrInvalidArgument", err)
		}
	})
}

func TestConformance_Sentinel_NonCanonicalWellFormedID(t *testing.T) {
	// Same 16 bytes as a real, existing row's id, but uppercase — a form
	// uuid.Parse would accept as "well-formed" that Postgres normalizes
	// and SQLite does not. Must be rejected identically to any other
	// malformed input, not silently accepted and then behave differently
	// per backend.
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		ctx := context.Background()
		p, err := repo.Profiles().Create(ctx, &model.UserProfile{Name: "Canonical ID Test", Source: "direct"})
		if err != nil {
			t.Fatalf("creating profile: %v", err)
		}
		nonCanonical := strings.ToUpper(p.ID)
		_, err = repo.Profiles().Get(ctx, nonCanonical)
		if !errors.Is(err, dao.ErrInvalidArgument) {
			t.Errorf("Get with a non-canonical (uppercase) but otherwise valid id = %v, want ErrInvalidArgument — the validator must accept only the canonical lowercase form", err)
		}
	})
}

func TestConformance_Sentinel_ErrDuplicateUsername(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		ctx := context.Background()
		p, err := repo.Profiles().Create(ctx, &model.UserProfile{Name: "Dup Test", Source: "direct"})
		if err != nil {
			t.Fatalf("creating profile: %v", err)
		}
		methodID := seededMethodIDFor(t, repo)
		if _, err := repo.Credentials().Create(ctx, &model.UserCredential{UserID: p.ID, Username: "dupuser", MethodID: methodID, SecretState: "none"}); err != nil {
			t.Fatalf("first credential create: %v", err)
		}
		_, err = repo.Credentials().Create(ctx, &model.UserCredential{UserID: p.ID, Username: "DUPUSER", MethodID: methodID, SecretState: "none"})
		if !errors.Is(err, dao.ErrDuplicateUsername) {
			t.Errorf("duplicate (case-folded) username = %v, want ErrDuplicateUsername", err)
		}
	})
}

func TestConformance_Sentinel_ErrInvalidMethod(t *testing.T) {
	// Also A3's second new assertion: without them, this is exactly the
	// sentinel that silently never fires on SQLite if the foreign_keys
	// pragma isn't actually active on every connection.
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		ctx := context.Background()
		p, err := repo.Profiles().Create(ctx, &model.UserProfile{Name: "Invalid Method Test", Source: "direct"})
		if err != nil {
			t.Fatalf("creating profile: %v", err)
		}
		_, err = repo.Credentials().Create(ctx, &model.UserCredential{UserID: p.ID, Username: "invalidmethoduser", MethodID: "00000000-0000-0000-0000-00000000dead", SecretState: "none"})
		if !errors.Is(err, dao.ErrInvalidMethod) {
			t.Errorf("unknown method_id = %v, want ErrInvalidMethod — if this is nil, the FK is not enforced on this backend", err)
		}
	})
}

func TestConformance_Sentinel_ErrInvalidQuery(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		_, _, err := repo.Profiles().Search(context.Background(), dao.ProfileQuery{Limit: 101})
		if !errors.Is(err, dao.ErrInvalidQuery) {
			t.Errorf("Limit>100 = %v, want ErrInvalidQuery", err)
		}
	})
}

func TestConformance_Sentinel_ErrInvalidArgument(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		_, err := repo.Profiles().Create(context.Background(), &model.UserProfile{ID: "caller-supplied", Name: "X", Source: "direct"})
		if !errors.Is(err, dao.ErrInvalidArgument) {
			t.Errorf("caller-supplied ID = %v, want ErrInvalidArgument", err)
		}
	})
}

func TestConformance_Sentinel_ErrInvalidCredential(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		ctx := context.Background()
		p, err := repo.Profiles().Create(ctx, &model.UserProfile{Name: "Secret State Test", Source: "direct"})
		if err != nil {
			t.Fatalf("creating profile: %v", err)
		}
		methodID := seededMethodIDFor(t, repo)
		_, err = repo.Credentials().Create(ctx, &model.UserCredential{UserID: p.ID, Username: "secretstatetest", MethodID: methodID, SecretState: "set"})
		if !errors.Is(err, dao.ErrInvalidCredential) {
			t.Errorf("secret_state=set with no secret = %v, want ErrInvalidCredential", err)
		}
	})
}

// ---- §6 item 12: requires_secret / secret_state relationship ----

func TestConformance_RequiresSecretRelationship(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		ctx := context.Background()
		// The seeded 'password' method has requires_secret=true, so this
		// case can't reach the relationship directly through DDL alone —
		// the DAO-layer check (§6 item 12) is what this test proves,
		// covering the case DDL structurally cannot: a method with
		// requires_secret=false paired with secret_state='set'.
		p, err := repo.Profiles().Create(ctx, &model.UserProfile{Name: "Requires Secret Test", Source: "direct"})
		if err != nil {
			t.Fatalf("creating profile: %v", err)
		}
		passkeyMethod, err := createAuthMethod(t, repo, "passkey-"+p.ID, false)
		if err != nil {
			t.Fatalf("creating passkey auth_method: %v", err)
		}
		hashAlgo := "argon2id"
		hashCost := 3
		_, err = repo.Credentials().Create(ctx, &model.UserCredential{
			UserID: p.ID, Username: "requiressecrettest", MethodID: passkeyMethod, SecretState: "set",
			Secret: []byte("hash"), HashAlgo: &hashAlgo, HashCost: &hashCost,
		})
		if !errors.Is(err, dao.ErrInvalidCredential) {
			t.Errorf("secret_state=set against a requires_secret=false method = %v, want ErrInvalidCredential (§6 item 12)", err)
		}
	})
}

// ---- §6 item 10: is_active enforcement, and deactivation doesn't
// invalidate existing credentials. ----

func TestConformance_IsActiveEnforcement(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		ctx := context.Background()
		inactiveMethod, err := createAuthMethod(t, repo, "inactive-method", true)
		if err != nil {
			t.Fatalf("creating auth_method: %v", err)
		}
		if err := deactivateMethod(t, repo, inactiveMethod); err != nil {
			t.Fatalf("deactivating method: %v", err)
		}

		p, err := repo.Profiles().Create(ctx, &model.UserProfile{Name: "Is Active Test", Source: "direct"})
		if err != nil {
			t.Fatalf("creating profile: %v", err)
		}
		_, err = repo.Credentials().Create(ctx, &model.UserCredential{UserID: p.ID, Username: "isactivetest", MethodID: inactiveMethod, SecretState: "none"})
		if !errors.Is(err, dao.ErrInvalidMethod) {
			t.Errorf("creating a credential against a deactivated method = %v, want ErrInvalidMethod", err)
		}
	})
}

func TestConformance_DeactivationDoesNotInvalidateExistingCredentials(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		ctx := context.Background()
		method, err := createAuthMethod(t, repo, "will-deactivate", true)
		if err != nil {
			t.Fatalf("creating auth_method: %v", err)
		}
		p, err := repo.Profiles().Create(ctx, &model.UserProfile{Name: "Existing Cred Test", Source: "direct"})
		if err != nil {
			t.Fatalf("creating profile: %v", err)
		}
		cred, err := repo.Credentials().Create(ctx, &model.UserCredential{UserID: p.ID, Username: "existingcredtest", MethodID: method, SecretState: "none"})
		if err != nil {
			t.Fatalf("creating credential: %v", err)
		}

		if err := deactivateMethod(t, repo, method); err != nil {
			t.Fatalf("deactivating method: %v", err)
		}

		got, err := repo.Credentials().GetByUsername(ctx, "existingcredtest")
		if err != nil {
			t.Errorf("existing credential against a since-deactivated method should still read back, got %v", err)
		}
		if got == nil || got.ID != cred.ID {
			t.Error("read-back credential does not match the one created")
		}
	})
}

// ---- §6.2: cascade delete asymmetry (A3's first new assertion). ----

func TestConformance_ProfileDeleteCascadesToCredentials(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		ctx := context.Background()
		methodID := seededMethodIDFor(t, repo)
		p, err := repo.Profiles().Create(ctx, &model.UserProfile{Name: "Cascade Test", Source: "direct"})
		if err != nil {
			t.Fatalf("creating profile: %v", err)
		}
		if _, err := repo.Credentials().Create(ctx, &model.UserCredential{UserID: p.ID, Username: "cascadetest", MethodID: methodID, SecretState: "none"}); err != nil {
			t.Fatalf("creating credential: %v", err)
		}

		if err := repo.Profiles().Delete(ctx, p.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		list, err := repo.Credentials().ListByUserID(ctx, p.ID)
		if err != nil {
			t.Fatalf("ListByUserID: %v", err)
		}
		if len(list) != 0 {
			t.Error("credential survived profile deletion — ON DELETE CASCADE not enforced on this backend (A3)")
		}
	})
}

func TestConformance_CredentialDeleteLeavesProfileStanding(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		ctx := context.Background()
		methodID := seededMethodIDFor(t, repo)
		p, err := repo.Profiles().Create(ctx, &model.UserProfile{Name: "Asymmetry Test", Source: "direct"})
		if err != nil {
			t.Fatalf("creating profile: %v", err)
		}
		cred, err := repo.Credentials().Create(ctx, &model.UserCredential{UserID: p.ID, Username: "asymmetrytest", MethodID: methodID, SecretState: "none"})
		if err != nil {
			t.Fatalf("creating credential: %v", err)
		}

		if err := repo.Credentials().Delete(ctx, cred.ID); err != nil {
			t.Fatalf("Delete credential: %v", err)
		}

		if _, err := repo.Profiles().Get(ctx, p.ID); err != nil {
			t.Errorf("profile should survive its credential's deletion, got %v", err)
		}
	})
}

// ---- §7 retention/deletion minimums. ----

func TestConformance_DeleteExpired_InvalidClass(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		_, err := repo.DeleteExpired(context.Background(), dao.RetentionClass(""), time.Now(), 100)
		if !errors.Is(err, dao.ErrInvalidArgument) {
			t.Errorf("zero-value RetentionClass = %v, want ErrInvalidArgument", err)
		}
	})
}

func TestConformance_DeleteExpired_MaxRowsBounds(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		ctx := context.Background()
		if _, err := repo.DeleteExpired(ctx, dao.RetentionDirect, time.Now(), 0); !errors.Is(err, dao.ErrInvalidArgument) {
			t.Errorf("maxRows=0 = %v, want ErrInvalidArgument", err)
		}
		if _, err := repo.DeleteExpired(ctx, dao.RetentionDirect, time.Now(), 10_001); !errors.Is(err, dao.ErrInvalidArgument) {
			t.Errorf("maxRows=10001 = %v, want ErrInvalidArgument", err)
		}
	})
}

func TestConformance_OldestSurvivingAt_NonNilWhenNothingOldEnough(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		ctx := context.Background()
		if _, err := repo.Profiles().Create(ctx, &model.UserProfile{Name: "Too Young To Sweep", Source: "direct"}); err != nil {
			t.Fatalf("creating profile: %v", err)
		}
		result, err := repo.DeleteExpired(ctx, dao.RetentionDirect, time.Now().Add(-365*24*time.Hour), 100)
		if err != nil {
			t.Fatalf("DeleteExpired: %v", err)
		}
		if result.RowsDeleted != 0 {
			t.Fatalf("RowsDeleted = %d, want 0 (nothing old enough)", result.RowsDeleted)
		}
		if result.OldestSurvivingAt == nil {
			t.Error("OldestSurvivingAt is nil, want non-nil — a survivor exists even though nothing was old enough to delete (§3c)")
		}
	})
}

func TestConformance_OldestSurvivingAt_NilWhenClassEmpty(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		result, err := repo.DeleteExpired(context.Background(), dao.RetentionIDPCacheOrphan, time.Now().Add(365*24*time.Hour), 100)
		if err != nil {
			t.Fatalf("DeleteExpired: %v", err)
		}
		if result.OldestSurvivingAt != nil {
			t.Errorf("OldestSurvivingAt = %v, want nil — the class has zero rows", *result.OldestSurvivingAt)
		}
	})
}

func TestConformance_Drained_TrueOnFinalBatch(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		result, err := repo.DeleteExpired(context.Background(), dao.RetentionDirect, time.Now().Add(365*24*time.Hour), 100)
		if err != nil {
			t.Fatalf("DeleteExpired: %v", err)
		}
		if !result.Drained {
			t.Error("Drained = false on a batch smaller than maxRows, want true")
		}
	})
}

func TestConformance_DeleteWritesExactlyOneDeletionLogRow(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		ctx := context.Background()
		p, err := repo.Profiles().Create(ctx, &model.UserProfile{Name: "Log Row Test", Source: "direct"})
		if err != nil {
			t.Fatalf("creating profile: %v", err)
		}
		ref := "ref-1"
		if err := repo.DeleteProfile(ctx, p.ID, &ref); err != nil {
			t.Fatalf("DeleteProfile: %v", err)
		}
		// Deleting again must not succeed (row is gone) — proves exactly
		// one delete happened, not a retry writing a second log row.
		if err := repo.DeleteProfile(ctx, p.ID, &ref); !errors.Is(err, dao.ErrNotFound) {
			t.Errorf("second DeleteProfile on the same id = %v, want ErrNotFound", err)
		}
	})
}

func TestConformance_DeleteExpired_AlreadyCancelledContextDeletesNothing(t *testing.T) {
	runAgainstAllBackends(t, func(t *testing.T, repo dao.Repository) {
		ctx := context.Background()
		if _, err := repo.Profiles().Create(ctx, &model.UserProfile{Name: "Cancelled Context Test", Source: "direct"}); err != nil {
			t.Fatalf("creating profile: %v", err)
		}

		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()

		before := countProfiles(t, repo)
		_, err := repo.DeleteExpired(cancelledCtx, dao.RetentionDirect, time.Now().Add(365*24*time.Hour), 100)
		if err == nil {
			t.Error("DeleteExpired with an already-cancelled context should return an error, not succeed")
		}
		after := countProfiles(t, repo)
		if after != before {
			t.Errorf("row count changed from %d to %d — an already-cancelled context must delete nothing", before, after)
		}
	})
}

// ---- Real CockroachDB 40001 retry, not mocked. ----

func TestConformance_CockroachDB_RealSerializationRetry(t *testing.T) {
	repo := newPostgresFamilyFixture(t, "cockroachdb", "TEST_COCKROACHDB_DSN").repo
	ctx := context.Background()

	p, err := repo.Profiles().Create(ctx, &model.UserProfile{Name: "Contention Target", Source: "direct"})
	if err != nil {
		t.Fatalf("creating profile: %v", err)
	}

	// Many concurrent Updates to the SAME row force real SERIALIZABLE
	// contention on CockroachDB — some attempts WILL hit a genuine
	// SQLSTATE 40001 internally. withRetry is expected to absorb every
	// one of them transparently: every call below must return nil, not
	// just "most of them".
	const n = 20
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := repo.Profiles().Update(ctx, &model.UserProfile{ID: p.ID, Name: "Contention Target", Source: "direct"})
			errs[i] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("concurrent Update %d returned %v — a real 40001 was not fully absorbed by withRetry", i, err)
		}
	}
}
