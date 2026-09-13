package sweepstore

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewMetricsHandler builds api-service's /metrics endpoint — a fresh
// prometheus.Registry populated from reader.LatestSnapshots on EVERY
// scrape (not a background poller updating a shared registry), so a
// scrape always reflects the persisted table's current state rather
// than some earlier snapshot this process cached. Served on api-service
// because it's the always-up pod (04-infra-devops/
// handoff-amber-observability-inventory.md); the sweep's own CronJob pod
// is gone long before Prometheus's scrape interval could reach it.
//
// Metric names deliberately avoid the substring "health" anywhere —
// Amber's inventory found Filebeat silently drops any log line matching
// that pattern, and while this handler's own output never reaches
// Filebeat (it's scraped by Prometheus, not logged), the same naming
// discipline is kept here for consistency with every other metric this
// project emits.
func NewMetricsHandler(reader SnapshotReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reg := prometheus.NewRegistry()

		lastFinished := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "retention_sweep_last_finished_timestamp_seconds",
			Help: "Unix timestamp of this class's most recent sweep run's completion, whether or not it drained. Absent (no series) for a class whose most recent run hasn't finished yet. Feeds RetentionSweepNotReporting (Priya Nandakumar, 05): a genuine absence of completions, not gated on success, so a died-but-never-drained run still counts as reporting.",
		}, []string{"class"})
		oldestSurvivingAge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "oldest_surviving_row_age_seconds",
			Help: "Age of the oldest surviving row in this class, from the most recent drained sweep only. Absent when that class has zero rows remaining, or has never drained.",
		}, []string{"class"})
		missedSlots := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "retention_sweep_missed_slots",
			Help: "Scheduled runs missed since this class's previous completion, as recorded by its single most recent run (05's §3d: computed at start, since a skipped run under concurrencyPolicy: Forbid has no pod to record itself). A point-in-time value, not a pre-summed total — Theo Bergman's (04) RetentionSweepSkipping alert rule aggregates this itself via sum_over_time() across its own rolling window, per Priya Nandakumar's (05) ruling that superseded an earlier retention_sweep_skipped_total design.",
		}, []string{"class"})
		rowsExamined := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "rows_examined",
			Help: "Rows examined by this class's single most recent sweep run, whether or not it drained.",
		}, []string{"class"})
		rowsDeleted := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "rows_deleted",
			Help: "Rows deleted by this class's single most recent sweep run, whether or not it drained.",
		}, []string{"class"})
		lastRunDuration := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "retention_sweep_last_run_duration_seconds",
			Help: "Wall-clock duration of this class's single most recent sweep run (finished_at minus started_at). Absent when that run hasn't finished yet (still running, or died before finishing) — Priya Nandakumar's (05) early-warning rule at 0.5x the sweep interval.",
		}, []string{"class"})
		reg.MustRegister(lastFinished, oldestSurvivingAge, missedSlots, rowsExamined, rowsDeleted, lastRunDuration)

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		snapshots, err := reader.LatestSnapshots(ctx)
		if err != nil {
			// A scrape that can't reach the database is a real signal —
			// fail the scrape (500), don't serve an empty-but-200
			// response that would look identical to "genuinely nothing
			// to report yet."
			log.Printf("api-service: /metrics: reading sweep snapshots: %v", err)
			http.Error(w, "reading sweep snapshots", http.StatusInternalServerError)
			return
		}

		for _, snap := range snapshots {
			class := string(snap.Class)
			if snap.LastFinishedAt != nil {
				lastFinished.WithLabelValues(class).Set(float64(snap.LastFinishedAt.Unix()))
			}
			if snap.OldestSurvivingAt != nil {
				oldestSurvivingAge.WithLabelValues(class).Set(time.Since(*snap.OldestSurvivingAt).Seconds())
			}
			missedSlots.WithLabelValues(class).Set(float64(snap.MissedSlots))
			rowsExamined.WithLabelValues(class).Set(float64(snap.RowsExamined))
			rowsDeleted.WithLabelValues(class).Set(float64(snap.RowsDeleted))
			if snap.LastRunDurationSeconds != nil {
				lastRunDuration.WithLabelValues(class).Set(*snap.LastRunDurationSeconds)
			}
		}

		promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).ServeHTTP(w, r)
	})
}
