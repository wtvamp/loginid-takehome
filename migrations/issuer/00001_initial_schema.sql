-- +goose Up
-- The token issuer's client store (LT-51), per Priya Nandakumar's (05)
-- ruling on this story: a separate `issuer` database — same PostgreSQL
-- instance as the main DAO's database, but a distinct database so a
-- cross-database join to user_profile/user_credential is structurally
-- impossible, not just policy-forbidden. Its own goose invocation
-- (GOOSE_DBSTRING pointed at this database) and its own version table
-- (goose_db_version_issuer), run as a third invocation alongside the
-- existing backend-directory + shared/ pair (05-data-ops/
-- migration-approach.md §"Mechanism 04 needs"). issuer_migrator/
-- issuer_runtime roles carry over the same DDL-vs-DML privilege split
-- LT-52's postgres-in-namespace.md already established for the main
-- database — provisioning those roles is 04's manifest/runbook
-- responsibility, not this file's.
--
-- oauth_client has no PII columns, ever (Priya's ruling) — client_id and
-- name identify a machine client application, not a person, so there is
-- no retention clock on this table (contrast user_profile/
-- user_credential's retention sweep, dao.Repository.DeleteExpired).
--
-- secret_state mirrors user_credential.secret_state's own lesson (05's
-- ruling, cited directly): three states (none/set/revoked) must be
-- asserted explicitly by a named CHECK, never collapsed into an
-- incidental "hash IS NULL" inference — a row can be `revoked` with a
-- NULL hash for a reason distinct from `none` never having had one.
--
-- client_id's charset CHECK is this table's own identifier rule —
-- 05-data-ops/multi-db-strategy.md amendment A4 (canonical UUID
-- validation) is explicitly NOT applied here, since client_id is a
-- human/CI-assigned slug (e.g. issued by the QA seeding runbook), not a
-- generated UUID primary key like every other table in this project.
--
-- granted_scopes is TEXT[] — deliberately PostgreSQL-only, unlike the
-- main DAO schema's portability requirement (05-data-ops/
-- multi-db-strategy.md), since the issuer database has no SQLite/
-- CockroachDB peer to stay portable against.
--
-- name and audience are additions beyond Priya's explicitly named column
-- list (client_id, client_secret_hash/secret_state, granted_scopes,
-- status, created_at/updated_at, no PII) — both operationally necessary
-- (name: a human-readable label for which application a row is, for
-- anyone reading this table directly; audience: LT-51's token claims
-- need a per-client `aud` value, since idp-connector and api-service are
-- different audiences) and neither is PII. Flagged explicitly for
-- Priya's DDL review rather than assumed silently.

CREATE TABLE oauth_client (
	client_id TEXT NOT NULL,
	name TEXT NOT NULL,
	client_secret_hash TEXT,
	secret_state TEXT NOT NULL,
	granted_scopes TEXT[] NOT NULL,
	audience TEXT NOT NULL,
	status TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	CONSTRAINT pk_oauth_client PRIMARY KEY (client_id),
	CONSTRAINT ck_oauth_client_id_charset CHECK (client_id ~ '^[A-Za-z0-9._-]{8,64}$'),
	CONSTRAINT ck_oauth_client_name_nonempty CHECK (length(trim(name)) > 0),
	CONSTRAINT ck_oauth_client_secret_state_enum CHECK (secret_state IN ('none', 'set', 'revoked')),
	CONSTRAINT ck_oauth_client_secret_state CHECK (
		(secret_state = 'set' AND client_secret_hash IS NOT NULL)
		OR
		(secret_state IN ('none', 'revoked') AND client_secret_hash IS NULL)
	),
	-- Exactly one scope, not merely a nonempty set: this project's
	-- provisioning model is one client credential per scope a caller
	-- needs (decisions/search-authz-scoping.md; LT-40's singleScope
	-- rejects a multi-scope token outright), so the DB-level invariant
	-- matches the Go-level one instead of merely allowing it.
	CONSTRAINT ck_oauth_client_granted_scopes_single CHECK (array_length(granted_scopes, 1) = 1),
	CONSTRAINT ck_oauth_client_audience_nonempty CHECK (length(trim(audience)) > 0),
	CONSTRAINT ck_oauth_client_status CHECK (status IN ('active', 'disabled'))
);

-- +goose Down
DROP TABLE oauth_client;
