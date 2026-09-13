package sweepstore

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"loginid-takehome/internal/dao"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

// migrationSQL reads a goose-format migration file's Up section — the
// real migration file, per this project's own established convention
// (see internal/dao/postgres/testutil_test.go), not a hand-copied
// schema.
func migrationSQL(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading migration %s: %v", path, err)
	}
	content := string(b)
	upStart := strings.Index(content, "-- +goose Up")
	downStart := strings.Index(content, "-- +goose Down")
	if upStart == -1 || downStart == -1 {
		t.Fatalf("migration %s missing +goose Up/Down markers", path)
	}
	return content[upStart+len("-- +goose Up") : downStart]
}

// setupPostgres connects to this package's throwaway database (see
// testmain_test.go), applies the real retention_sweep_run migration
// into a fresh schema, and returns a ready-to-use *Store plus a raw
// *sql.DB for test-side assertions/fixtures. Fails loudly (not skips)
// if TEST_POSTGRES_DSN is unset in CI, matching this project's own
// established fail-not-skip convention.
func setupPostgres(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	if testDBDSN == "" {
		if os.Getenv("CI") != "" {
			t.Fatalf("TEST_POSTGRES_DSN not set in CI — the sweepstore integration gate is not running")
		}
		t.Skip("TEST_POSTGRES_DSN not set — skipping sweepstore Postgres integration test")
	}

	db, err := sql.Open("pgx", testDBDSN)
	if err != nil {
		t.Fatalf("opening TEST_POSTGRES_DSN: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)

	schemaName := fmt.Sprintf("test_sweepstore_%d", rand.Int63())
	if _, err := db.Exec("CREATE SCHEMA " + schemaName); err != nil {
		t.Fatalf("creating test schema: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP SCHEMA " + schemaName + " CASCADE") })
	if _, err := db.Exec("SET search_path TO " + schemaName); err != nil {
		t.Fatalf("setting search_path: %v", err)
	}

	root := repoRoot(t)
	migration := migrationSQL(t, filepath.Join(root, "migrations", "postgres", "00003_retention_sweep_run.sql"))
	if _, err := db.Exec(migration); err != nil {
		t.Fatalf("applying retention_sweep_run migration: %v", err)
	}

	return NewStore(db, "pgx"), db
}

// setupSQLite is trivial by comparison — an in-memory, shared-cache
// database, no schema isolation needed since each test gets its own.
func setupSQLite(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:sweepstore_%d?mode=memory&cache=shared", rand.Int63())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("opening sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)

	root := repoRoot(t)
	migration := migrationSQL(t, filepath.Join(root, "migrations", "sqlite", "00002_retention_sweep_run.sql"))
	if _, err := db.Exec(migration); err != nil {
		t.Fatalf("applying retention_sweep_run migration: %v", err)
	}

	return NewStore(db, "sqlite"), db
}

// backends lets every guarantee below run against both real
// implementations from one table-driven test, per this project's own
// conformance-suite convention.
func backends(t *testing.T) []struct {
	name  string
	store *Store
	db    *sql.DB
} {
	pgStore, pgDB := setupPostgres(t)
	sqliteStore, sqliteDB := setupSQLite(t)
	return []struct {
		name  string
		store *Store
		db    *sql.DB
	}{
		{"postgres", pgStore, pgDB},
		{"sqlite", sqliteStore, sqliteDB},
	}
}

func TestStartRunFinishRun_RoundTrip(t *testing.T) {
	for _, b := range backends(t) {
		t.Run(b.name, func(t *testing.T) {
			ctx := context.Background()
			startedAt := time.Date(2026, 9, 13, 3, 0, 0, 0, time.UTC)

			runID, missedSlots, err := b.store.StartRun(ctx, dao.RetentionDirect, startedAt, time.Hour)
			if err != nil {
				t.Fatalf("StartRun: %v", err)
			}
			if runID == "" {
				t.Fatal("StartRun returned an empty runID")
			}
			if missedSlots != 0 {
				t.Errorf("missedSlots = %d, want 0 (no prior completed run for this class)", missedSlots)
			}

			finishedAt := startedAt.Add(5 * time.Second)
			oldest := startedAt.Add(-48 * time.Hour)
			if err := b.store.FinishRun(ctx, runID, finishedAt, 42, 10, true, &oldest); err != nil {
				t.Fatalf("FinishRun: %v", err)
			}

			snaps, err := b.store.LatestSnapshots(ctx)
			if err != nil {
				t.Fatalf("LatestSnapshots: %v", err)
			}
			var got *ClassSnapshot
			for i := range snaps {
				if snaps[i].Class == dao.RetentionDirect {
					got = &snaps[i]
				}
			}
			if got == nil {
				t.Fatalf("no snapshot for class %q", dao.RetentionDirect)
			}
			if got.RowsExamined != 42 || got.RowsDeleted != 10 {
				t.Errorf("RowsExamined/RowsDeleted = %d/%d, want 42/10", got.RowsExamined, got.RowsDeleted)
			}
			if got.LastFinishedAt == nil || !got.LastFinishedAt.Equal(finishedAt) {
				t.Errorf("LastFinishedAt = %v, want %v", got.LastFinishedAt, finishedAt)
			}
			if got.OldestSurvivingAt == nil || !got.OldestSurvivingAt.Equal(oldest) {
				t.Errorf("OldestSurvivingAt = %v, want %v", got.OldestSurvivingAt, oldest)
			}
			if got.LastRunDurationSeconds == nil || *got.LastRunDurationSeconds != 5 {
				t.Errorf("LastRunDurationSeconds = %v, want 5", got.LastRunDurationSeconds)
			}
		})
	}
}

// TestLastRunDuration_SubSecondPrecisionNeverNegative is a regression
// test for a real bug caught by live end-to-end testing, not by any
// unit test (every other test in this file used gaps of a second or
// more, which happened not to exercise it): converting a pgx-native
// time.Time to a string via plain RFC3339 truncates sub-second
// precision, so a started_at/finished_at pair a few milliseconds apart
// — the realistic case for a fast, near-empty sweep — could round to
// the same second or swap order, producing a small NEGATIVE duration.
// preciseTimeLayout (fixed-width nanosecond precision) fixes this; this
// test pins the fix with a sub-second gap specifically, on both
// backends, so the exact bug can't silently return.
func TestLastRunDuration_SubSecondPrecisionNeverNegative(t *testing.T) {
	for _, b := range backends(t) {
		t.Run(b.name, func(t *testing.T) {
			ctx := context.Background()
			startedAt := time.Date(2026, 9, 13, 19, 16, 8, 168726000, time.UTC)
			finishedAt := time.Date(2026, 9, 13, 19, 16, 8, 191736000, time.UTC) // 23.01ms later

			runID, _, err := b.store.StartRun(ctx, dao.RetentionDirect, startedAt, 0)
			if err != nil {
				t.Fatalf("StartRun: %v", err)
			}
			if err := b.store.FinishRun(ctx, runID, finishedAt, 1, 1, true, nil); err != nil {
				t.Fatalf("FinishRun: %v", err)
			}

			snaps, err := b.store.LatestSnapshots(ctx)
			if err != nil {
				t.Fatalf("LatestSnapshots: %v", err)
			}
			var got *ClassSnapshot
			for i := range snaps {
				if snaps[i].Class == dao.RetentionDirect {
					got = &snaps[i]
				}
			}
			if got == nil {
				t.Fatalf("no snapshot for class %q", dao.RetentionDirect)
			}
			if got.LastRunDurationSeconds == nil {
				t.Fatalf("LastRunDurationSeconds is nil, want ~0.023")
			}
			if *got.LastRunDurationSeconds < 0 {
				t.Errorf("LastRunDurationSeconds = %v, want a small POSITIVE value — negative means sub-second precision was lost somewhere in the round trip", *got.LastRunDurationSeconds)
			}
			const want = 0.023010
			const tolerance = 0.001
			if diff := *got.LastRunDurationSeconds - want; diff < -tolerance || diff > tolerance {
				t.Errorf("LastRunDurationSeconds = %v, want ~%v (±%v)", *got.LastRunDurationSeconds, want, tolerance)
			}
		})
	}
}

func TestStartRun_UnfinishedRunHasNoLastFinishedOrDuration(t *testing.T) {
	for _, b := range backends(t) {
		t.Run(b.name, func(t *testing.T) {
			ctx := context.Background()
			_, _, err := b.store.StartRun(ctx, dao.RetentionIDPCache, time.Now(), time.Hour)
			if err != nil {
				t.Fatalf("StartRun: %v", err)
			}
			// Deliberately never call FinishRun — simulating "started
			// and died" (§3d's second distinguishable state).

			snaps, err := b.store.LatestSnapshots(ctx)
			if err != nil {
				t.Fatalf("LatestSnapshots: %v", err)
			}
			var got *ClassSnapshot
			for i := range snaps {
				if snaps[i].Class == dao.RetentionIDPCache {
					got = &snaps[i]
				}
			}
			if got == nil {
				t.Fatalf("no snapshot for class %q", dao.RetentionIDPCache)
			}
			if got.LastFinishedAt != nil {
				t.Errorf("LastFinishedAt = %v, want nil (never finished)", got.LastFinishedAt)
			}
			if got.LastRunDurationSeconds != nil {
				t.Errorf("LastRunDurationSeconds = %v, want nil (never finished)", *got.LastRunDurationSeconds)
			}
			if got.RowsExamined != 0 || got.RowsDeleted != 0 {
				t.Errorf("RowsExamined/RowsDeleted = %d/%d, want 0/0 (row's own DEFAULT, never updated)", got.RowsExamined, got.RowsDeleted)
			}
		})
	}
}

func TestStartRun_MissedSlotsComputedAgainstInterval(t *testing.T) {
	for _, b := range backends(t) {
		t.Run(b.name, func(t *testing.T) {
			ctx := context.Background()
			interval := time.Hour

			first := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
			runID, missed, err := b.store.StartRun(ctx, dao.RetentionIDPCacheOrphan, first, interval)
			if err != nil {
				t.Fatalf("StartRun (first): %v", err)
			}
			if missed != 0 {
				t.Errorf("first run: missedSlots = %d, want 0 (no prior completed run)", missed)
			}
			if err := b.store.FinishRun(ctx, runID, first.Add(time.Minute), 1, 1, true, nil); err != nil {
				t.Fatalf("FinishRun (first): %v", err)
			}

			// Normal case: next run starts almost exactly one interval
			// later — zero missed slots.
			normalNext := first.Add(time.Minute).Add(interval)
			_, missed, err = b.store.StartRun(ctx, dao.RetentionIDPCacheOrphan, normalNext, interval)
			if err != nil {
				t.Fatalf("StartRun (normal): %v", err)
			}
			if missed != 0 {
				t.Errorf("normal-cadence run: missedSlots = %d, want 0", missed)
			}

			// Skipped case: a run starting 3 intervals after the last
			// completion should report 2 missed slots (floor(gap/I) - 1).
			skippedNext := first.Add(time.Minute).Add(3 * interval)
			_, missed, err = b.store.StartRun(ctx, dao.RetentionIDPCacheOrphan, skippedNext, interval)
			if err != nil {
				t.Fatalf("StartRun (skipped): %v", err)
			}
			if missed != 2 {
				t.Errorf("3-interval-gap run: missedSlots = %d, want 2", missed)
			}
		})
	}
}

// TestFinishedButNotDrained_ReportsCompletionButNeverOldestSurviving
// covers a run that FINISHED without draining (RetentionSweepNotReporting
// must still see it as "the sweep is reporting" — Priya Nandakumar's (05)
// own framing: "a genuine absence of rows, not a stale scrape," ungated
// on Drained), while OldestSurvivingAt — meaningful only from a Drained
// result — must stay nil, per the table's own tying CHECK.
//
// Uses time.Now() (a real wall-clock reading, nanosecond precision in
// Go) rather than a hand-constructed time.Date, deliberately: this is
// the one test in this file exercising the actual column precision
// contract. Postgres's TIMESTAMPTZ stores microsecond precision, not
// nanosecond — every other test in this package hand-constructs
// timestamps whose sub-microsecond digits are already zero, which
// hides this. The comparison here truncates to microseconds before
// comparing, on both backends, so the assertion is true of what the
// column actually stores rather than of Go's in-memory value.
func TestFinishedButNotDrained_ReportsCompletionButNeverOldestSurviving(t *testing.T) {
	for _, b := range backends(t) {
		t.Run(b.name, func(t *testing.T) {
			ctx := context.Background()
			finishedAt := time.Now().Truncate(time.Microsecond)
			runID, _, err := b.store.StartRun(ctx, dao.RetentionDirect, finishedAt.Add(-time.Second), 0)
			if err != nil {
				t.Fatalf("StartRun: %v", err)
			}
			if err := b.store.FinishRun(ctx, runID, finishedAt, 5, 0, false, nil); err != nil {
				t.Fatalf("FinishRun: %v", err)
			}

			snaps, err := b.store.LatestSnapshots(ctx)
			if err != nil {
				t.Fatalf("LatestSnapshots: %v", err)
			}
			for _, s := range snaps {
				if s.Class != dao.RetentionDirect {
					continue
				}
				if s.LastFinishedAt == nil || !s.LastFinishedAt.Truncate(time.Microsecond).Equal(finishedAt) {
					t.Errorf("LastFinishedAt = %v, want %v (compared at microsecond precision — Postgres's TIMESTAMPTZ column resolution) — a finished-but-not-drained run must still be visible as a completion", s.LastFinishedAt, finishedAt)
				}
				if s.OldestSurvivingAt != nil {
					t.Errorf("OldestSurvivingAt = %v, want nil — this run finished but did not drain, so this field must stay unset", s.OldestSurvivingAt)
				}
			}
		})
	}
}

// TestMissedSlots_ReflectsOnlyTheLatestRun is Priya Nandakumar's (05)
// own correction (refinement/LT-49.md's "Update — mechanism superseded"
// pass): MissedSlots is a point-in-time value from the single most
// recent run, NOT a pre-summed running total across every retained row
// — Theo Bergman's (04) RetentionSweepSkipping alert rule does its own
// sum_over_time() aggregation across a rolling window from repeated
// scrapes, the standard Prometheus pattern, rather than this package
// pre-aggregating server-side (which is what an earlier, since-replaced
// retention_sweep_skipped_total design did).
func TestMissedSlots_ReflectsOnlyTheLatestRun(t *testing.T) {
	for _, b := range backends(t) {
		t.Run(b.name, func(t *testing.T) {
			ctx := context.Background()
			interval := time.Minute
			t0 := time.Now()

			runID, _, err := b.store.StartRun(ctx, dao.RetentionIDPCache, t0, interval)
			if err != nil {
				t.Fatalf("StartRun 1: %v", err)
			}
			if err := b.store.FinishRun(ctx, runID, t0, 1, 1, true, nil); err != nil {
				t.Fatalf("FinishRun 1: %v", err)
			}

			// Second run skips 2 slots (a 3-interval gap since the
			// first's completion).
			t1 := t0.Add(3 * interval)
			runID, missed1, err := b.store.StartRun(ctx, dao.RetentionIDPCache, t1, interval)
			if err != nil {
				t.Fatalf("StartRun 2: %v", err)
			}
			if missed1 != 2 {
				t.Fatalf("second run's own missedSlots = %d, want 2 (test setup assumption)", missed1)
			}
			if err := b.store.FinishRun(ctx, runID, t1, 1, 1, true, nil); err != nil {
				t.Fatalf("FinishRun 2: %v", err)
			}

			// Third run is on-schedule (missedSlots = 0) — the LATEST
			// run's own value, which must be what LatestSnapshots
			// reports, not 2 (the previous run's value) and not 2
			// (a sum of the two).
			t2 := t1.Add(interval)
			_, missed2, err := b.store.StartRun(ctx, dao.RetentionIDPCache, t2, interval)
			if err != nil {
				t.Fatalf("StartRun 3: %v", err)
			}
			if missed2 != 0 {
				t.Fatalf("third run's own missedSlots = %d, want 0 (test setup assumption)", missed2)
			}

			snaps, err := b.store.LatestSnapshots(ctx)
			if err != nil {
				t.Fatalf("LatestSnapshots: %v", err)
			}
			var got *ClassSnapshot
			for i := range snaps {
				if snaps[i].Class == dao.RetentionIDPCache {
					got = &snaps[i]
				}
			}
			if got == nil {
				t.Fatalf("no snapshot for class %q", dao.RetentionIDPCache)
			}
			if got.MissedSlots != 0 {
				t.Errorf("MissedSlots = %d, want 0 (the LATEST run's own value, not the previous run's 2, and not a sum)", got.MissedSlots)
			}
		})
	}
}

func TestPruneOlderThan_RemovesOldRowsOnly(t *testing.T) {
	for _, b := range backends(t) {
		t.Run(b.name, func(t *testing.T) {
			ctx := context.Background()
			now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

			oldRunID, _, err := b.store.StartRun(ctx, dao.RetentionDirect, now.Add(-40*24*time.Hour), 0)
			if err != nil {
				t.Fatalf("StartRun (old): %v", err)
			}
			if err := b.store.FinishRun(ctx, oldRunID, now.Add(-40*24*time.Hour), 1, 1, true, nil); err != nil {
				t.Fatalf("FinishRun (old): %v", err)
			}

			recentRunID, _, err := b.store.StartRun(ctx, dao.RetentionDirect, now.Add(-1*time.Hour), 0)
			if err != nil {
				t.Fatalf("StartRun (recent): %v", err)
			}
			if err := b.store.FinishRun(ctx, recentRunID, now, 2, 2, true, nil); err != nil {
				t.Fatalf("FinishRun (recent): %v", err)
			}

			if err := b.store.PruneOlderThan(ctx, now.Add(-30*24*time.Hour)); err != nil {
				t.Fatalf("PruneOlderThan: %v", err)
			}

			snaps, err := b.store.LatestSnapshots(ctx)
			if err != nil {
				t.Fatalf("LatestSnapshots: %v", err)
			}
			for _, s := range snaps {
				if s.Class == dao.RetentionDirect {
					if s.RowsExamined != 2 {
						t.Errorf("RowsExamined = %d, want 2 (the recent row) — old row should have been pruned", s.RowsExamined)
					}
				}
			}
		})
	}
}

// TestTimestampOrdering_SpreadAcrossAMonth is Priya Nandakumar's (05)
// own named priority conformance assertion (multi-db-strategy.md §3d):
// "the one that bites on exactly one backend." SQLite stores
// TIMESTAMPTZ as TEXT; a text timestamp sorts chronologically only if
// it's zero-padded, fixed-width, and in one offset (RFC3339 UTC, "Z"
// suffix) — a local-offset or variable-width rendering would sort
// lexicographically into the wrong order on SQLite alone, silently
// returning the wrong "latest" row while Postgres/CockroachDB (native
// TIMESTAMPTZ) look fine. Spread over a month, not a single-value round
// trip, since a narrow spread could accidentally still sort correctly
// even with a subtly wrong format.
func TestTimestampOrdering_SpreadAcrossAMonth(t *testing.T) {
	for _, b := range backends(t) {
		t.Run(b.name, func(t *testing.T) {
			ctx := context.Background()
			base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
			var lastRunID string
			var lastFinishedAt time.Time
			for i := 0; i < 10; i++ {
				startedAt := base.AddDate(0, 0, i*3) // spread across ~a month
				runID, _, err := b.store.StartRun(ctx, dao.RetentionDirect, startedAt, 0)
				if err != nil {
					t.Fatalf("StartRun %d: %v", i, err)
				}
				finishedAt := startedAt.Add(time.Minute)
				if err := b.store.FinishRun(ctx, runID, finishedAt, i, i, true, nil); err != nil {
					t.Fatalf("FinishRun %d: %v", i, err)
				}
				lastRunID = runID
				lastFinishedAt = finishedAt
			}
			_ = lastRunID

			snaps, err := b.store.LatestSnapshots(ctx)
			if err != nil {
				t.Fatalf("LatestSnapshots: %v", err)
			}
			var got *ClassSnapshot
			for i := range snaps {
				if snaps[i].Class == dao.RetentionDirect {
					got = &snaps[i]
				}
			}
			if got == nil {
				t.Fatalf("no snapshot for class %q", dao.RetentionDirect)
			}
			// The LAST-inserted row (i=9) has the highest RowsExamined
			// (9) and the latest started_at — if timestamp ordering is
			// wrong on this backend, an earlier row would win instead.
			if got.RowsExamined != 9 {
				t.Errorf("RowsExamined = %d, want 9 (the chronologically latest row, not necessarily the last one this test happened to insert)", got.RowsExamined)
			}
			if got.LastFinishedAt == nil || !got.LastFinishedAt.Equal(lastFinishedAt) {
				t.Errorf("LastFinishedAt = %v, want %v (the latest of 10 rows spread across ~a month)", got.LastFinishedAt, lastFinishedAt)
			}
		})
	}
}
