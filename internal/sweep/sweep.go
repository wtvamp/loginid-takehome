// Package sweep is LT-44's caller for 05's already-built, already-tested
// retention primitives (dao.Repository.DeleteExpired/DeleteProfile,
// internal/dao/conformance's own suite) — a designed-but-not-operational
// control until this package wires a scheduled invocation. See
// refinement/LT-44.md for the full acceptance criteria this implements.
package sweep

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"

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

// MetricEmitter is the seam Run reports through. EmitClassMetrics is
// called exactly once per class, and ONLY when that class's sweep
// reaches Drained == true — never from an intermediate batch.
// oldestSurvivingRowAgeSeconds is nil when Drained's own
// SweepResult.OldestSurvivingAt is nil (zero rows remaining in the
// class after this run), matching observability.md's own warning: a
// non-drained result publishing this field would fire the alert on
// every sweep and get muted, worse than not having the metric at all.
//
// MarkRunStarted is called exactly once, unconditionally, at the very
// top of Run — before any class is attempted, regardless of whether any
// class later errors or the whole run is cancelled. Raised by Tobias
// Lindqvist's original skip-visibility objection (carried from LT-44),
// but its actual alerting role was settled by Priya Nandakumar's (05)
// later alert-formula ruling: her design gets the skip-vs-dead
// distinction from per-class staleness of oldest_surviving_row_age_seconds
// itself (no fresh sample in more than 2 sweep intervals), deliberately
// property-based rather than mechanism-based — so MarkRunStarted's
// timestamp is NOT wired into any alert rule. It exists as a
// supplementary, kubectl-logs-level debugging signal only (confirmed
// with Theo Bergman, 04, PR #59): distinguishing "no process even
// started this cycle" from "a process started but errored before any
// class drained" is useful for a human diagnosing a specific incident,
// even though neither of Priya's alert rules needs the distinction to
// fire correctly.
type MetricEmitter interface {
	MarkRunStarted(now time.Time)
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

// RunRecorder persists each class's result to 05's retention_sweep_run
// table (multi-db-strategy.md §3d, amendment A7) — the seam that lets
// api-service, the always-up pod, expose these results to Prometheus
// long after this short-lived sweep process has exited
// (04-infra-devops/handoff-amber-observability-inventory.md:
// Prometheus scrapes HTTP endpoints, cannot ingest this process's
// stdout, and a CronJob pod is typically gone before a scrape interval
// could reach it anyway). internal/sweepstore.Store satisfies this
// interface; this package depends only on the narrow seam it actually
// calls, per decisions/test-double-strategy.md's own convention.
//
// StartRun writes the first of §3d's two required writes per run
// (INSERT with finished_at NULL) and returns the new row's id plus the
// missedSlots this call computed against interval (0 if interval <= 0,
// meaning "unknown/not configured" — this package never guesses at a
// schedule it wasn't told). FinishRun writes the second (UPDATE at
// completion) — called only when a class actually drains; an error
// leaves that class's row at finished_at == NULL, correctly recording
// "started and died" rather than "completed," per §3d's own three-state
// design. PruneOlderThan is this table's own 30-day self-prune (§3d:
// operational metadata, not PII, not a RetentionClass, never written to
// deletion_log), called once per Run, after every class has been
// attempted.
//
// A persistence failure anywhere here is logged and otherwise ignored —
// this table exists to make sweep results observable to Prometheus, not
// to gate the sweep's own actual retention work; a database hiccup on
// this seam must never turn a real, successful deletion into a reported
// failure.
type RunRecorder interface {
	StartRun(ctx context.Context, class dao.RetentionClass, startedAt time.Time, interval time.Duration) (runID string, missedSlots int, err error)
	FinishRun(ctx context.Context, runID string, finishedAt time.Time, rowsExamined, rowsDeleted int, drained bool, oldestSurvivingAt *time.Time) error
	PruneOlderThan(ctx context.Context, cutoff time.Time) error
}

// retentionSweepRunRetention is retention_sweep_run's own self-prune
// window (§3d: 30 days — comfortably covers the widest alert window,
// 24h, with a month of history left for debugging).
const retentionSweepRunRetention = 30 * 24 * time.Hour

// Run executes one full sweep: every class in allClasses, looping
// DeleteExpired per class until Drained, bounded by maxRowsPerBatch per
// call (the DAO's own already-validated bound, not a new parameter this
// package invents). ctx must be the caller's own job/scheduler context
// — never a request-scoped one, since a CronJob-invoked process has no
// request to derive a context from in the first place, and passing an
// already-cancelled ctx here (verified by this package's own tests, not
// just the DAO layer's already-covered case) must stop before deleting
// anything, in every class, not just the first.
//
// recorder may be nil (persistence not configured) — every call site
// below is guarded accordingly, matching emit's own existing nil
// tolerance. interval is the CronJob's own schedule, passed through to
// recorder.StartRun for its missedSlots computation; <= 0 means unknown.
func Run(ctx context.Context, repo ExpirySweeper, now time.Time, emit MetricEmitter, recorder RunRecorder, interval time.Duration) []ClassResult {
	if emit != nil {
		emit.MarkRunStarted(now)
	}
	results := make([]ClassResult, 0, len(allClasses))
	for _, class := range allClasses {
		results = append(results, runClass(ctx, repo, class, now, emit, recorder, interval))
	}
	if recorder != nil {
		if err := recorder.PruneOlderThan(ctx, now.Add(-retentionSweepRunRetention)); err != nil {
			log.Printf("sweep: pruning retention_sweep_run rows older than %s: %v", retentionSweepRunRetention, err)
		}
	}
	return results
}

func runClass(ctx context.Context, repo ExpirySweeper, class dao.RetentionClass, now time.Time, emit MetricEmitter, recorder RunRecorder, interval time.Duration) ClassResult {
	cutoff, ok := cutoffFor(now, class)
	if !ok {
		return ClassResult{Class: class, Err: fmt.Errorf("sweep: no cutoff defined for class %q", class)}
	}

	var runID string
	if recorder != nil {
		id, missedSlots, err := recorder.StartRun(ctx, class, now, interval)
		if err != nil {
			log.Printf("sweep: recording run start for class %q: %v", class, err)
		} else {
			runID = id
			if missedSlots > 0 {
				log.Printf("sweep: class %q missed %d scheduled run(s) since its last completion", class, missedSlots)
			}
		}
	}

	result := ClassResult{Class: class}
	for {
		if err := ctx.Err(); err != nil {
			result.Err = err
			// runID's row is deliberately left at finished_at == NULL —
			// §3d's "started and died" state, not overwritten here.
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
			if recorder != nil && runID != "" {
				// Deliberately time.Now(), not now: now is the single
				// invocation-time clock cutoffFor/StartRun/the age
				// calculation all share, but a drain can take multiple
				// batches and real elapsed wall-clock time — finished_at
				// must reflect when the run actually completed, not the
				// frozen moment Run was called, or
				// retention_sweep_last_run_duration_seconds would read
				// as near-zero regardless of true duration.
				if err := recorder.FinishRun(ctx, runID, time.Now(), result.RowsExamined, result.RowsDeleted, true, batch.OldestSurvivingAt); err != nil {
					log.Printf("sweep: recording run finish for class %q: %v", class, err)
				}
			}
			return result
		}
		// Not drained: loop again, same class, same cutoff — never a
		// mixed predicate across classes within one call to Run.
	}
}

// PrometheusMetricEmitter backs MetricEmitter with real
// github.com/prometheus/client_golang Gauge objects registered in their
// own prometheus.Registry — LT-49's own criterion that these values
// reach an actual prometheus.Client registration, not just a
// log.Printf or an internal struct field (superseding LT-44's original
// StdoutMetricEmitter, which only ever produced ad hoc log lines).
// Render renders the registry's current state in real Prometheus text
// exposition format — agreed with Theo Bergman (04, PR #59 review):
// stdout, not a Pushgateway push, is the accepted mechanism for this
// project's scale (no Pushgateway runs on the real cluster, and
// standing one up is disproportionate scope for this story, same
// disposition as LT-44's own metrics-mechanism call) — Theo's own
// sink reads these lines from the pod's log output via `kubectl logs`
// today, with Pushgateway named as the documented production next step.
type PrometheusMetricEmitter struct {
	registry     *prometheus.Registry
	rowsExamined *prometheus.GaugeVec
	rowsDeleted  *prometheus.GaugeVec
	oldestAge    *prometheus.GaugeVec
	lastRunTS    prometheus.Gauge
}

// NewPrometheusMetricEmitter constructs a ready-to-use emitter with its
// own private registry — never the global default registry, so this
// package's metrics can never collide with another package's
// same-process registration (moot for a one-shot batch binary today,
// but a self-contained registry costs nothing and rules the class of
// bug out structurally rather than by convention).
func NewPrometheusMetricEmitter() *PrometheusMetricEmitter {
	reg := prometheus.NewRegistry()
	e := &PrometheusMetricEmitter{
		registry: reg,
		rowsExamined: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "rows_examined",
			Help: "Rows examined by the most recent retention sweep for this class, summed across every batch it took to drain.",
		}, []string{"class"}),
		rowsDeleted: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "rows_deleted",
			Help: "Rows deleted by the most recent retention sweep for this class, summed across every batch it took to drain.",
		}, []string{"class"}),
		oldestAge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "oldest_surviving_row_age_seconds",
			Help: "Age of the oldest surviving row in this class, published only when the sweep drained (never from an intermediate batch).",
		}, []string{"class"}),
		lastRunTS: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "sweep_last_run_timestamp_seconds",
			Help: "Unix timestamp of the most recent sweep invocation's start, set unconditionally regardless of whether any class succeeded. Supplementary/debugging signal only (kubectl-logs-level inspection) — deliberately NOT wired into any alert rule: Priya Nandakumar's (05) alert-formula ruling is property-based (is data surviving, is the sweep still reporting per class), and an alert consuming a mechanism-level 'did a process start' signal is exactly what her ruling argues against. Confirmed with Theo Bergman (04, PR #59) that none of the three alert rules reference this metric.",
		}),
	}
	reg.MustRegister(e.rowsExamined, e.rowsDeleted, e.oldestAge, e.lastRunTS)
	return e
}

func (e *PrometheusMetricEmitter) MarkRunStarted(now time.Time) {
	e.lastRunTS.Set(float64(now.Unix()))
}

func (e *PrometheusMetricEmitter) EmitClassMetrics(class dao.RetentionClass, rowsExamined, rowsDeleted int, oldestSurvivingRowAgeSeconds *float64) {
	e.rowsExamined.WithLabelValues(string(class)).Set(float64(rowsExamined))
	e.rowsDeleted.WithLabelValues(string(class)).Set(float64(rowsDeleted))
	if oldestSurvivingRowAgeSeconds != nil {
		e.oldestAge.WithLabelValues(string(class)).Set(*oldestSurvivingRowAgeSeconds)
	}
}

// Render renders every metric this emitter has recorded so far, in
// real Prometheus text exposition format, to w — called once, after
// Run returns, by cmd/api-service's own runSweepMode, so the values
// reach the pod's stdout (and therefore `kubectl logs`) exactly once
// per invocation rather than interleaved with per-class log lines.
func (e *PrometheusMetricEmitter) Render(w io.Writer) error {
	mfs, err := e.registry.Gather()
	if err != nil {
		return fmt.Errorf("sweep: gathering metrics: %w", err)
	}
	enc := expfmt.NewEncoder(w, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range mfs {
		if err := enc.Encode(mf); err != nil {
			return fmt.Errorf("sweep: encoding metric family %q: %w", mf.GetName(), err)
		}
	}
	return nil
}
