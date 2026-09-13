-- +goose Up
-- Amendment A7 (05-data-ops/multi-db-strategy.md §3d) — same table and
-- constraint names as migrations/postgres/00003_retention_sweep_run.sql
-- and migrations/cockroachdb/00003_retention_sweep_run.sql, in SQLite
-- types: TEXT for UUID/TIMESTAMPTZ (stored RFC3339, UTC, "Z" suffix —
-- required for chronological sort on this backend, per §3d's own
-- conformance note: a local-offset or variable-width rendering sorts
-- lexicographically into the wrong order here alone), INTEGER for
-- BOOLEAN, matching this project's existing convention (see
-- 00001_initial_schema.sql).
CREATE TABLE retention_sweep_run (
	id TEXT,
	class TEXT NOT NULL,
	started_at TEXT NOT NULL,
	finished_at TEXT,
	rows_examined INTEGER NOT NULL DEFAULT 0,
	rows_deleted INTEGER NOT NULL DEFAULT 0,
	oldest_surviving_at TEXT,
	drained INTEGER NOT NULL DEFAULT 0,
	missed_slots INTEGER NOT NULL DEFAULT 0,
	CONSTRAINT pk_retention_sweep_run PRIMARY KEY (id),
	CONSTRAINT ck_retention_sweep_run_class CHECK (class IN ('direct', 'idp_cache', 'idp_cache_orphan')),
	CONSTRAINT ck_retention_sweep_run_drained CHECK (drained != 0 OR oldest_surviving_at IS NULL)
);

CREATE INDEX idx_retention_sweep_run_class_finished_at ON retention_sweep_run (class, finished_at DESC);

-- +goose Down
DROP TABLE retention_sweep_run;
