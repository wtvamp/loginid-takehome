-- +goose Up
-- Amendment A7 (05-data-ops/multi-db-strategy.md §3d): the retention
-- sweep persists its per-class result here so api-service (the always-up
-- pod) can expose it to Prometheus over /metrics — the sweep's CronJob
-- pod and the scraper never overlap in time, so the result has to live
-- somewhere the scraper can read it. Written twice per run (INSERT at
-- start with finished_at NULL, UPDATE at completion) so no row, a row
-- with NULL finished_at, and a row with finished_at distinguish skipped-
-- or-dead, started-and-died, and completed — the same three-states-in-
-- one-null defect secret_state exists to prevent. Not a RetentionClass,
-- not written to deletion_log, self-pruned by the sweep at 30 days (all
-- per §3d — this table holds operational metadata, no PII).
-- TIMESTAMPTZ here stores microsecond precision, not nanosecond — Go's
-- time.Time (nanosecond) is truncated on write. internal/sweepstore's
-- own tests compare timestamps at microsecond precision for exactly
-- this reason; don't assert nanosecond-exact equality against a value
-- that has round-tripped through this table.
CREATE TABLE retention_sweep_run (
	id UUID DEFAULT gen_random_uuid(),
	class TEXT NOT NULL,
	started_at TIMESTAMPTZ NOT NULL,
	finished_at TIMESTAMPTZ,
	rows_examined INTEGER NOT NULL DEFAULT 0,
	rows_deleted INTEGER NOT NULL DEFAULT 0,
	oldest_surviving_at TIMESTAMPTZ,
	drained BOOLEAN NOT NULL DEFAULT false,
	missed_slots INTEGER NOT NULL DEFAULT 0,
	CONSTRAINT pk_retention_sweep_run PRIMARY KEY (id),
	CONSTRAINT ck_retention_sweep_run_class CHECK (class IN ('direct', 'idp_cache', 'idp_cache_orphan')),
	-- oldest_surviving_at is meaningful only from a drained result (§3c)
	-- — a non-drained row carrying one must be unrepresentable, not
	-- merely discouraged. NULL with drained stays legal (an empty class).
	CONSTRAINT ck_retention_sweep_run_drained CHECK (drained OR oldest_surviving_at IS NULL)
);

-- Serves both reads: latest-per-class for the gauges, and the rolling-day
-- scan for the skip rate.
CREATE INDEX idx_retention_sweep_run_class_finished_at ON retention_sweep_run (class, finished_at DESC);

-- +goose Down
DROP TABLE retention_sweep_run;
