-- +goose Up
-- Amendment A7 (05-data-ops/multi-db-strategy.md §3d) — identical to
-- migrations/postgres/00003_retention_sweep_run.sql; this table has no
-- COLLATE clause or trigram index for A5's own split to apply to, so
-- there is no divergence between the two engines here. Same constraint
-- names as migrations/postgres/ and migrations/sqlite/ so all three stay
-- diffable against each other.
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
	CONSTRAINT ck_retention_sweep_run_drained CHECK (drained OR oldest_surviving_at IS NULL)
);

CREATE INDEX idx_retention_sweep_run_class_finished_at ON retention_sweep_run (class, finished_at DESC);

-- +goose Down
DROP TABLE retention_sweep_run;
