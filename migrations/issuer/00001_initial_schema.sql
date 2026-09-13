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
-- Priya's DDL review rather than assumed silently — she confirmed `name`
-- was already implicit in her ruling and approved `audience` as a real
-- amendment (PR #34 review).
--
-- client_id is case-SENSITIVE (a plain TEXT primary key, no folding) —
-- unlike user_credential.username in the main schema, which is
-- case-folded for uniqueness (its unique index is on LOWER(username)).
-- Deliberately different, not an inconsistency to "fix" later: OAuth2
-- client identifiers are conventionally case-sensitive, usernames in
-- this project are not (Priya's review, PR #34).
--
-- granted_scopes (an array constrained to exactly one element) and
-- audience (a scalar) both express the same "exactly one" invariant two
-- different ways — defensible today (scope is conceptually a set that
-- may later admit more than one; audience conceptually isn't), but if
-- the single-scope-per-credential commitment (decisions/
-- search-authz-scoping.md) ever relaxes, ck_oauth_client_granted_scopes_single
-- is what changes and audience's own singularity is easy to forget to
-- reconsider alongside it (Priya's review, PR #34).

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
	--
	-- cardinality(), not array_length(granted_scopes, 1): array_length
	-- returns NULL (not 0) for an empty array, and a CHECK passes on
	-- NULL — '{}' would have satisfied the original
	-- `array_length(granted_scopes, 1) = 1` text despite having zero
	-- elements (Priya Nandakumar's review, PR #34: "the accepted set is
	-- what the expression accepts, not what the comment says it
	-- accepts"). The two extra clauses close the neighboring holes
	-- cardinality alone wouldn't: ARRAY[NULL]::text[] and ARRAY['']::text[]
	-- both have cardinality 1.
	CONSTRAINT ck_oauth_client_granted_scopes_single CHECK (
		cardinality(granted_scopes) = 1
		AND granted_scopes[1] IS NOT NULL
		AND length(trim(granted_scopes[1])) > 0
	),
	CONSTRAINT ck_oauth_client_audience_nonempty CHECK (length(trim(audience)) > 0),
	CONSTRAINT ck_oauth_client_status CHECK (status IN ('active', 'disabled'))
);

-- updated_at has no writing-layer contract to rely on (unlike the main
-- DAO schema, whose Go Update methods pass now() explicitly) since this
-- table is written only by 04's out-of-band seeding/rotation runbook,
-- never application code — a trigger is the only mechanism that can't be
-- forgotten by whichever runbook step runs next (Priya Nandakumar's
-- review, PR #34).
--
-- The StatementBegin/StatementEnd markers just below are required here:
-- goose's default parser splits a migration file on every semicolon,
-- including the ones inside this plpgsql function body — without them
-- it breaks the CREATE FUNCTION apart mid dollar-quoted block and fails
-- with "unterminated dollar-quoted string" (reproduced against the real
-- goose CLI, not just psql, which has no such splitting behavior and
-- would not have caught this — deploy failure on the live cluster).
-- The markers tell goose to treat everything between them as one atomic
-- statement instead.
-- +goose StatementBegin
CREATE FUNCTION oauth_client_set_updated_at() RETURNS trigger AS $$
BEGIN
	NEW.updated_at := now();
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_oauth_client_set_updated_at
	BEFORE UPDATE ON oauth_client
	FOR EACH ROW
	EXECUTE FUNCTION oauth_client_set_updated_at();

-- +goose Down
DROP TRIGGER trg_oauth_client_set_updated_at ON oauth_client;
DROP FUNCTION oauth_client_set_updated_at();
DROP TABLE oauth_client;
