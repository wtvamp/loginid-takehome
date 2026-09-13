// Package sweep is LT-44's caller for 05's already-built, already-tested
// retention primitives (dao.Repository.DeleteExpired/DeleteProfile,
// internal/dao/conformance's own suite) — a designed-but-not-operational
// control until this package wires a scheduled invocation. See
// refinement/LT-44.md for the full acceptance criteria this implements.
package sweep

import (
	"context"
	"fmt"
	"log"
	"time"

	"loginid-takehome/internal/dao"
)

// maxRowsPerBatch is the DAO's own already-validated bound
// (multi-db-strategy.md §3c: DeleteExpired rejects anything outside
// 1..10_000) — this story doesn't invent a new batching parameter, it
// picks one value inside that existing range.
const maxRowsPerBatch = 1000

// cutoffFor computes the "older than" cutoff for class as of now — the
// retention windows themselves are sourced from pii-governance.md's
// table (24 months for direct, 30 days for idp_cache, 7 days for
// idp_cache_orphan), stated there as "Proposed," not "Ruled" — this
// package implements the numbers as given; deciding whether they're the
// right numbers is Priya's/legal's call, not this function's. Using
// calendar-based AddDate for the 24-month window rather than a fixed
// multiple of 30*24h (which would drift from a true "24 months ago" by
// several days depending on which months are in the window) — the other
// two classes are exact-day windows, where AddDate and a fixed duration
// agree exactly, so AddDate is used uniformly for all three rather than
// mixing two different arithmetic strategies across classes. Not
// perfectly calendar-correct at every boundary: AddDate normalizes a
// nonexistent target date (e.g. "Feb 31") forward per its own documented
// behavior, shifting the cutoff by a day or two in that specific case —
// immaterial at a 24-month granularity (Oren Castellan, PR #55 review).

func cutoffFor(now time.Time, class dao.RetentionClass) (time.Time, bool) {
	switch class {
	case dao.RetentionDirect:
		return now.AddDate(0, -24, 0), true
	case dao.RetentionIDPCache:
		return now.AddDate(0, 0, -30), true
	case dao.RetentionIDPCacheOrphan:
		return now.AddDate(0, 0, -7), true
	default:
		return time.Time{}, false
	}
}

// allClasses is the fixed, ordered set this sweep runs every invocation
// — one class at a time, never a mixed/shared predicate, per this
// story's own criterion. Order doesn't matter functionally (each class's
// DeleteExpired call is independent), but iterating orphans last means a
// row promoted out of "orphan" by a fresh credential mid-run (a genuine
// race, vanishingly unlikely at this project's scale but worth naming)
// is examined under its correct class rather than a stale one from
// earlier in the same run.
var allClasses = []dao.RetentionClass{
	dao.RetentionDirect,
	dao.RetentionIDPCache,
	dao.RetentionIDPCacheOrphan,
}

// ExpirySweeper is the narrow seam this package depends on — just the
// one dao.Repository method it actually calls, per
// decisions/test-double-strategy.md's own convention of depending on the
// smallest interface a caller needs rather than the full composite, so
// this package's own tests use a minimal hand-written fake instead of a
// full Repository double.
type ExpirySweeper interface {
	DeleteExpired(ctx context.Context, class dao.RetentionClass, olderThan time.Time, maxRows int) (dao.SweepResult, error)
}

// MetricEmitter is called exactly once per class, and ONLY when that
// class's sweep reaches Drained == true — never from an intermediate
// batch. oldestSurvivingRowAgeSeconds is nil when Drained's own
// SweepResult.OldestSurvivingAt is nil (zero rows remaining in the
// class after this run), matching observability.md's own warning: a
// non-drained result publishing this field would fire the alert on
// every sweep and get muted, worse than not having the metric at all.
type MetricEmitter interface {
	EmitClassMetrics(class dao.RetentionClass, rowsExamined, rowsDeleted int, oldestSurvivingRowAgeSeconds *float64)
}

// ClassResult is RunSweep's own per-class outcome — RowsExamined/
// RowsDeleted are summed across every batch DeleteExpired took to reach
// Drained for this class; Err is set (and the class's own metrics are
// NOT emitted) if any batch in this class's loop failed, so one class's
// database error can't produce a misleadingly-labeled partial metric —
// the run continues to the next class regardless, rather than aborting
// the whole sweep over one class's failure.
type ClassResult struct {
	Class        dao.RetentionClass
	RowsExamined int
	RowsDeleted  int
	Drained      bool
	Err          error
}

// Run executes one full sweep: every class in allClasses, looping
// DeleteExpired per class until Drained, bounded by maxRowsPerBatch per
// call (the DAO's own already-validated bound, not a new parameter this
// package invents). ctx must be the caller's own job/scheduler context
// — never a request-scoped one, since a CronJob-invoked process has no
// request to derive a context from in the first place, and passing an
// already-cancelled ctx here (verified by this package's own tests, not
// just the DAO layer's already-covered case) must stop before deleting
// anything, in every class, not just the first.
func Run(ctx context.Context, repo ExpirySweeper, now time.Time, emit MetricEmitter) []ClassResult {
	results := make([]ClassResult, 0, len(allClasses))
	for _, class := range allClasses {
		results = append(results, runClass(ctx, repo, class, now, emit))
	}
	return results
}

func runClass(ctx context.Context, repo ExpirySweeper, class dao.RetentionClass, now time.Time, emit MetricEmitter) ClassResult {
	cutoff, ok := cutoffFor(now, class)
	if !ok {
		return ClassResult{Class: class, Err: fmt.Errorf("sweep: no cutoff defined for class %q", class)}
	}

	result := ClassResult{Class: class}
	for {
		if err := ctx.Err(); err != nil {
			result.Err = err
			return result
		}

		batch, err := repo.DeleteExpired(ctx, class, cutoff, maxRowsPerBatch)
		if err != nil {
			result.Err = err
			return result
		}
		result.RowsExamined += batch.RowsExamined
		result.RowsDeleted += batch.RowsDeleted

		if batch.Drained {
			result.Drained = true
			var ageSeconds *float64
			if batch.OldestSurvivingAt != nil {
				age := now.Sub(*batch.OldestSurvivingAt).Seconds()
				ageSeconds = &age
			}
			if emit != nil {
				emit.EmitClassMetrics(class, result.RowsExamined, result.RowsDeleted, ageSeconds)
			}
			return result
		}
		// Not drained: loop again, same class, same cutoff — never a
		// mixed predicate across classes within one call to Run.
	}
}

// StdoutMetricEmitter writes each class's metrics as one
// Prometheus-exposition-compatible line per metric to the process's own
// stdout — this package's default, mechanism-agnostic emitter for a
// short-lived batch job with no HTTP scrape target of its own.
// 04-infra-devops owns how these lines actually reach Prometheus (a log
// scraper, a Pushgateway wrapper around this process, or something
// else) — this type's only job is emitting well-formed, correctly
// labeled values, per observability.md's exact metric names/labels.
type StdoutMetricEmitter struct{}

func (StdoutMetricEmitter) EmitClassMetrics(class dao.RetentionClass, rowsExamined, rowsDeleted int, oldestSurvivingRowAgeSeconds *float64) {
	log.Printf(`rows_examined{class=%q} %d`, class, rowsExamined)
	log.Printf(`rows_deleted{class=%q} %d`, class, rowsDeleted)
	if oldestSurvivingRowAgeSeconds != nil {
		log.Printf(`oldest_surviving_row_age_seconds{class=%q} %g`, class, *oldestSurvivingRowAgeSeconds)
	}
}
