// Package sweepstore persists LT-44's retention sweep's per-class
// results (05-data-ops/multi-db-strategy.md §3d, amendment A7) so
// api-service — the always-up pod — can expose them to Prometheus over
// /metrics. The sweep's own CronJob pod and the scraper never overlap in
// time (04-infra-devops/handoff-amber-observability-inventory.md:
// Prometheus scrapes HTTP endpoints and cannot ingest stdout, and a
// short-lived CronJob pod is typically gone before a scrape interval
// could catch it), so the result has to live somewhere the scraper can
// read it — a table, not a metric held only in the sweep process's own
// memory.
package sweepstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"loginid-takehome/internal/dao"
)

// preciseTimeLayout is used everywhere this package converts a
// time.Time to/from a string — both SQLite's own TEXT-column storage
// and this package's internal pgx-native-time-to-string bridging (used
// only inside LatestSnapshots, to give both backends one shared
// aggregation code path).
//
// Deliberately NOT time.RFC3339 (whole seconds only) or time.RFC3339Nano
// (Go's stdlib "trim trailing zero digits" fractional-second format,
// meaning two different timestamps serialize to DIFFERENT STRING
// LENGTHS depending on how many trailing zero nanosecond digits they
// happen to have) — both are wrong here for the same underlying reason,
// each in a different way this package hit for real:
//   - Plain RFC3339 silently truncates sub-second precision. This
//     produced a genuine bug caught by live-testing (not a unit test):
//     two timestamps a few milliseconds apart, once one is truncated to
//     whole seconds and the other isn't, can round to the SAME second
//     or even swap order, corrupting the finished-minus-started
//     duration calculation into a small negative number.
//   - RFC3339Nano's variable-width fractional seconds would defeat 05's
//     own §3d ordering requirement for SQLite's TEXT column: "a text
//     timestamp sorts chronologically only if it is zero-padded,
//     fixed-width and in one offset" — variable width breaks
//     lexicographic sort exactly the way that warning describes.
//
// "2006-01-02T15:04:05.000000000Z07:00": the "000000000" (not
// "999999999") forces Go's time.Format to always print all 9 fractional
// digits, zero-padded — fixed-width AND full nanosecond precision,
// closing both problems with one layout. Every value this package ever
// writes goes through this same layout (encodeTime, below), so every
// stored value always has the fixed 9-digit fractional part — verified
// this layout round-trips exactly (formatted, then parsed back,
// including the zero-fractional-second case) before relying on it here.
// A value from any OTHER source (hand-written test fixtures, a manual
// psql INSERT) that omits the fractional part entirely would fail to
// parse against this exact layout — not a concern for this table today
// (no prior data predates this layout; every row is written by this
// package alone), but worth knowing if that ever changes.
const preciseTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

// Store is a *sql.DB-backed implementation of internal/sweep.RunRecorder
// and of SnapshotReader (this package's own read seam for the /metrics
// handler) — one Go type usable against any of the three backends
// dao.New already selects between, since retention_sweep_run exists
// identically (same columns, same constraint names) in all three
// migration directories per §3d. driver is the underlying database/sql
// driver name ("pgx" or "sqlite" — cmd/api-service's own
// sqlDriverNameFor's output), since the two require different
// placeholder syntax and different timestamp encodings (pgx binds
// time.Time natively; SQLite's column is TEXT, so this package formats
// UTC in its own fixed-width, full-precision layout itself — see
// preciseTimeLayout's own doc comment for why plain RFC3339, and
// internal/dao/sqlite's own timeLayout, both fall short here).
type Store struct {
	db     *sql.DB
	isPgx  bool
	driver string
}

// NewStore wraps db. driver must be "pgx" or "sqlite" (cmd/api-service's
// sqlDriverNameFor's own output) — an unrecognized value is a
// programming error, not a runtime one, so callers should only ever
// pass one of the two.
func NewStore(db *sql.DB, driver string) *Store {
	return &Store{db: db, isPgx: driver == "pgx", driver: driver}
}

// ClassSnapshot is one class's current observable state, as read by the
// /metrics handler — everything a scrape needs, decoupled from the
// underlying table's own column shape.
type ClassSnapshot struct {
	Class dao.RetentionClass

	// OldestSurvivingAt comes from this class's most recent DRAINED row
	// only — nil if no drained row exists yet. Matches LT-44/49's own
	// established rule: meaningful only from a Drained result, never an
	// intermediate/failed one (the table's own
	// ck_retention_sweep_run_drained CHECK makes the opposite
	// unrepresentable).
	OldestSurvivingAt *time.Time

	// LastFinishedAt, RowsExamined, RowsDeleted, LastRunDurationSeconds,
	// and MissedSlots all come from this class's single MOST RECENT row
	// overall (by started_at), regardless of whether it drained — "from
	// the last run," not "from the last successful run." Deliberately
	// NOT gated on Drained: Priya Nandakumar's (05) RetentionSweepNotReporting
	// alert rule keys on "the age of the latest non-NULL finished_at
	// per class — a genuine absence of rows, not a stale scrape,"
	// which must see a died-but-not-drained run's completion just as
	// much as a successful one. LastFinishedAt and LastRunDurationSeconds
	// are both nil whenever that row hasn't finished yet (finished_at
	// NULL — started and either still running or died).
	LastFinishedAt         *time.Time
	RowsExamined           int
	RowsDeleted            int
	LastRunDurationSeconds *float64
	MissedSlots            int
}

// SnapshotReader is the seam the /metrics handler depends on — narrowed
// to just the one read this package's HTTP handler needs, per
// decisions/test-double-strategy.md's own convention, so that handler's
// own tests use a small fake instead of a real *Store.
type SnapshotReader interface {
	LatestSnapshots(ctx context.Context) ([]ClassSnapshot, error)
}

func (s *Store) ph(i int) string {
	if s.isPgx {
		return fmt.Sprintf("$%d", i)
	}
	return "?"
}

func (s *Store) encodeTime(t time.Time) any {
	if s.isPgx {
		return t
	}
	return t.UTC().Format(preciseTimeLayout)
}

func (s *Store) encodeTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return s.encodeTime(*t)
}

// StartRun records the beginning of one class's sweep attempt — the
// first of the two writes per run (05's §3d: insert at start with
// finished_at NULL, update at completion) that lets "no row," "a row
// with NULL finished_at," and "a row with finished_at" distinguish
// skipped-or-dead, started-and-died, and completed.
//
// interval is the CronJob's own schedule (cfg.SweepIntervalSeconds,
// parsed) — used only to compute missedSlots (§3d: floor(gap/I) - 1,
// zero in the normal case) against the gap since this class's last
// completed run. interval <= 0 (unset/unknown) means missedSlots is
// always 0 — this package doesn't guess at a schedule it was never told.
func (s *Store) StartRun(ctx context.Context, class dao.RetentionClass, startedAt time.Time, interval time.Duration) (runID string, missedSlots int, err error) {
	if interval > 0 {
		lastFinished, ok, err := s.lastFinishedAt(ctx, class)
		if err != nil {
			return "", 0, fmt.Errorf("sweepstore: querying last finished_at for class %q: %w", class, err)
		}
		if ok {
			gap := startedAt.Sub(lastFinished)
			slots := int(gap/interval) - 1
			if slots > 0 {
				missedSlots = slots
			}
		}
	}

	id := uuid.NewString()
	query := fmt.Sprintf(
		"INSERT INTO retention_sweep_run (id, class, started_at, missed_slots) VALUES (%s, %s, %s, %s)",
		s.ph(1), s.ph(2), s.ph(3), s.ph(4),
	)
	if _, err := s.db.ExecContext(ctx, query, id, string(class), s.encodeTime(startedAt), missedSlots); err != nil {
		return "", 0, fmt.Errorf("sweepstore: inserting run start for class %q: %w", class, err)
	}
	return id, missedSlots, nil
}

// FinishRun records the second of the two writes per run — the
// completion. oldestSurvivingAt is nil whenever drained's own
// SweepResult.OldestSurvivingAt was nil (matching the DAO's own
// zero-rows-remain semantics; the table's own
// ck_retention_sweep_run_drained CHECK requires oldestSurvivingAt be nil
// whenever drained is false, enforced at the database, not just here).
func (s *Store) FinishRun(ctx context.Context, runID string, finishedAt time.Time, rowsExamined, rowsDeleted int, drained bool, oldestSurvivingAt *time.Time) error {
	drainedValue := any(drained)
	if !s.isPgx {
		if drained {
			drainedValue = 1
		} else {
			drainedValue = 0
		}
	}
	query := fmt.Sprintf(
		`UPDATE retention_sweep_run SET finished_at = %s, rows_examined = %s, rows_deleted = %s, drained = %s, oldest_surviving_at = %s WHERE id = %s`,
		s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5), s.ph(6),
	)
	_, err := s.db.ExecContext(ctx, query, s.encodeTime(finishedAt), rowsExamined, rowsDeleted, drainedValue, s.encodeTimePtr(oldestSurvivingAt), runID)
	if err != nil {
		return fmt.Errorf("sweepstore: recording run finish for run %q: %w", runID, err)
	}
	return nil
}

// PruneOlderThan deletes every row whose started_at is before cutoff —
// this table's own retention (§3d: 30 days, self-pruned by the sweep at
// the end of each run, deliberately NOT a RetentionClass and NOT
// written to deletion_log, since it holds operational metadata, not
// PII).
func (s *Store) PruneOlderThan(ctx context.Context, cutoff time.Time) error {
	query := fmt.Sprintf("DELETE FROM retention_sweep_run WHERE started_at < %s", s.ph(1))
	if _, err := s.db.ExecContext(ctx, query, s.encodeTime(cutoff)); err != nil {
		return fmt.Errorf("sweepstore: pruning rows older than %s: %w", cutoff, err)
	}
	return nil
}

func (s *Store) lastFinishedAt(ctx context.Context, class dao.RetentionClass) (time.Time, bool, error) {
	query := fmt.Sprintf(
		"SELECT finished_at FROM retention_sweep_run WHERE class = %s AND finished_at IS NOT NULL ORDER BY finished_at DESC LIMIT 1",
		s.ph(1),
	)
	row := s.db.QueryRowContext(ctx, query, string(class))
	t, ok, err := s.scanNullableTime(row.Scan)
	if err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}
	if !ok {
		return time.Time{}, false, nil
	}
	return *t, true, nil
}

// scanNullableTime scans a single nullable timestamp column via scan
// (either *sql.Row.Scan or *sql.Rows.Scan) — pgx returns a real
// time.Time (scanned through sql.NullTime, since the column may be
// NULL); SQLite returns the preciseTimeLayout string this package
// itself wrote (scanned through sql.NullString, then parsed).
func (s *Store) scanNullableTime(scan func(dest ...any) error) (*time.Time, bool, error) {
	if s.isPgx {
		var nt sql.NullTime
		if err := scan(&nt); err != nil {
			return nil, false, err
		}
		if !nt.Valid {
			return nil, false, nil
		}
		t := nt.Time
		return &t, true, nil
	}
	var ns sql.NullString
	if err := scan(&ns); err != nil {
		return nil, false, err
	}
	if !ns.Valid {
		return nil, false, nil
	}
	t, err := time.Parse(preciseTimeLayout, ns.String)
	if err != nil {
		return nil, false, fmt.Errorf("parsing timestamp %q: %w", ns.String, err)
	}
	return &t, true, nil
}

// LatestSnapshots implements SnapshotReader — one row per class (all
// three RetentionClass values, even one with no rows at all yet:
// aggregation happens in Go over every row currently in the table
// rather than via backend-specific SQL (DISTINCT ON is Postgres/
// CockroachDB-only; SQLite has no equivalent), so this one
// implementation works identically against all three backends. The
// table is bounded by its own 30-day self-prune, so scanning every row
// on each scrape is cheap at this project's scale — no window functions,
// no per-backend query variants beyond placeholder/timestamp encoding.
func (s *Store) LatestSnapshots(ctx context.Context) ([]ClassSnapshot, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT class, started_at, finished_at, rows_examined, rows_deleted, oldest_surviving_at, drained, missed_slots
		FROM retention_sweep_run
		ORDER BY started_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("sweepstore: querying retention_sweep_run: %w", err)
	}
	defer func() { _ = rows.Close() }()

	byClass := make(map[dao.RetentionClass]*ClassSnapshot)
	haveLatestOverall := make(map[dao.RetentionClass]bool)
	haveLatestDrained := make(map[dao.RetentionClass]bool)

	for rows.Next() {
		var classStr string
		var finishedRaw, oldestRaw sql.NullString // overwritten below for pgx
		var rowsExamined, rowsDeleted, missedSlots int
		var drainedRaw any
		var startedAtTime time.Time

		if s.isPgx {
			var startedAt time.Time
			var finishedAt sql.NullTime
			var oldestAt sql.NullTime
			var drained bool
			if err := rows.Scan(&classStr, &startedAt, &finishedAt, &rowsExamined, &rowsDeleted, &oldestAt, &drained, &missedSlots); err != nil {
				return nil, fmt.Errorf("sweepstore: scanning retention_sweep_run row: %w", err)
			}
			startedAtTime = startedAt
			if finishedAt.Valid {
				finishedRaw = sql.NullString{String: finishedAt.Time.UTC().Format(preciseTimeLayout), Valid: true}
			}
			if oldestAt.Valid {
				oldestRaw = sql.NullString{String: oldestAt.Time.UTC().Format(preciseTimeLayout), Valid: true}
			}
			drainedRaw = drained
		} else {
			var startedAtStr string
			var drainedInt int
			if err := rows.Scan(&classStr, &startedAtStr, &finishedRaw, &rowsExamined, &rowsDeleted, &oldestRaw, &drainedInt, &missedSlots); err != nil {
				return nil, fmt.Errorf("sweepstore: scanning retention_sweep_run row: %w", err)
			}
			t, err := time.Parse(preciseTimeLayout, startedAtStr)
			if err != nil {
				return nil, fmt.Errorf("sweepstore: parsing started_at %q: %w", startedAtStr, err)
			}
			startedAtTime = t
			drainedRaw = drainedInt != 0
		}

		class := dao.RetentionClass(classStr)
		snap := byClass[class]
		if snap == nil {
			snap = &ClassSnapshot{Class: class}
			byClass[class] = snap
		}

		if !haveLatestOverall[class] {
			snap.RowsExamined = rowsExamined
			snap.RowsDeleted = rowsDeleted
			snap.MissedSlots = missedSlots
			if finishedRaw.Valid {
				finishedAtTime, err := time.Parse(preciseTimeLayout, finishedRaw.String)
				if err != nil {
					return nil, fmt.Errorf("sweepstore: parsing finished_at %q: %w", finishedRaw.String, err)
				}
				snap.LastFinishedAt = &finishedAtTime
				duration := finishedAtTime.Sub(startedAtTime).Seconds()
				snap.LastRunDurationSeconds = &duration
			}
			haveLatestOverall[class] = true
		}

		drained, _ := drainedRaw.(bool)
		if drained && !haveLatestDrained[class] {
			if oldestRaw.Valid {
				t, err := time.Parse(preciseTimeLayout, oldestRaw.String)
				if err != nil {
					return nil, fmt.Errorf("sweepstore: parsing oldest_surviving_at %q: %w", oldestRaw.String, err)
				}
				snap.OldestSurvivingAt = &t
			}
			haveLatestDrained[class] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sweepstore: iterating retention_sweep_run rows: %w", err)
	}

	out := make([]ClassSnapshot, 0, len(byClass))
	for _, snap := range byClass {
		out = append(out, *snap)
	}
	return out, nil
}
