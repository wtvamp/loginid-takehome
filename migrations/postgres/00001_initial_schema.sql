-- +goose Up
-- Real PostgreSQL DDL per 05-data-ops/multi-db-strategy.md §5. Split from
-- CockroachDB (migrations/cockroachdb/) under amendment A5 after
-- discovering COLLATE "C" is invalid syntax on CockroachDB — the internal/
-- dao/postgres Go PACKAGE still serves both engines (query construction
-- and dialect are genuinely shared; only the retry seam and, now, this
-- one DDL clause differ), this is purely a migration-directory split.
-- Every constraint is named — error translation matches on SQLSTATE plus
-- constraint name, never message text (contract §4; ddl-review-checklist.md B).
-- Names must match migrations/cockroachdb/ and migrations/sqlite/ exactly
-- so all three migrations stay diffable against each other.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE auth_method (
	id UUID DEFAULT gen_random_uuid(),
	name TEXT NOT NULL,
	requires_secret BOOLEAN NOT NULL,
	is_active BOOLEAN NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	CONSTRAINT pk_auth_method PRIMARY KEY (id),
	CONSTRAINT uq_auth_method_name UNIQUE (name)
);

CREATE TABLE user_profile (
	id UUID DEFAULT gen_random_uuid(),
	name TEXT COLLATE "C" NOT NULL,
	phone TEXT,
	street_address TEXT,
	locality TEXT,
	region TEXT,
	postal_code TEXT,
	country TEXT,
	source TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	CONSTRAINT pk_user_profile PRIMARY KEY (id),
	CONSTRAINT ck_user_profile_name_nonempty CHECK (length(trim(name)) > 0),
	-- 7-15 total digits after the '+' (contract amendment A6 — the
	-- original {1,14} range accepted a 2-digit number, e.g. "+1234",
	-- that SQLite's own phone CHECK (length BETWEEN 8 AND 16) rejects;
	-- {6,14} makes both backends accept exactly the same set).
	CONSTRAINT ck_user_profile_phone_e164 CHECK (phone IS NULL OR phone ~ '^\+[1-9][0-9]{6,14}$'),
	CONSTRAINT ck_user_profile_street_address_nonempty CHECK (street_address IS NULL OR length(trim(street_address)) > 0),
	CONSTRAINT ck_user_profile_locality_nonempty CHECK (locality IS NULL OR length(trim(locality)) > 0),
	CONSTRAINT ck_user_profile_region_nonempty CHECK (region IS NULL OR length(trim(region)) > 0),
	CONSTRAINT ck_user_profile_postal_code_nonempty CHECK (postal_code IS NULL OR length(trim(postal_code)) > 0),
	CONSTRAINT ck_user_profile_country_alpha2 CHECK (country IS NULL OR (country = upper(country) AND length(country) = 2)),
	CONSTRAINT ck_user_profile_source CHECK (source IN ('direct','idp_cache'))
);

-- Expression index on LOWER(name), not the bare column — a GIN index on
-- bare name is not used by a LOWER(name) predicate (checklist item 7).
CREATE INDEX idx_user_profile_name_trgm ON user_profile USING GIN (LOWER(name) gin_trgm_ops);
CREATE INDEX idx_user_profile_phone ON user_profile (phone);

CREATE TABLE user_credential (
	id UUID DEFAULT gen_random_uuid(),
	user_id UUID NOT NULL,
	username TEXT NOT NULL,
	method_id UUID NOT NULL,
	secret BYTEA,
	secret_state TEXT NOT NULL,
	hash_algo TEXT,
	hash_cost INTEGER,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	CONSTRAINT pk_user_credential PRIMARY KEY (id),
	CONSTRAINT fk_user_credential_profile FOREIGN KEY (user_id) REFERENCES user_profile(id) ON DELETE CASCADE,
	CONSTRAINT fk_user_credential_method FOREIGN KEY (method_id) REFERENCES auth_method(id),
	CONSTRAINT ck_user_credential_secret_state_enum CHECK (secret_state IN ('none','set','revoked')),
	CONSTRAINT ck_user_credential_secret_state CHECK (
		(secret_state = 'set'  AND secret IS NOT NULL AND hash_algo IS NOT NULL AND hash_cost IS NOT NULL)
		OR
		(secret_state IN ('none','revoked') AND secret IS NULL AND hash_algo IS NULL AND hash_cost IS NULL)
	)
);

CREATE UNIQUE INDEX uq_user_credential_username ON user_credential (LOWER(username));
CREATE INDEX idx_user_credential_user_id ON user_credential (user_id);
CREATE INDEX idx_user_credential_method_id ON user_credential (method_id);

CREATE TABLE deletion_log (
	id UUID DEFAULT gen_random_uuid(),
	profile_id UUID NOT NULL,
	source TEXT NOT NULL,
	reason TEXT NOT NULL,
	external_ref TEXT,
	job_run_id TEXT,
	deleted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	CONSTRAINT pk_deletion_log PRIMARY KEY (id),
	CONSTRAINT ck_deletion_log_reason CHECK (reason IN ('retention_sweep','subject_request'))
);
-- No name, phone, address, username or secret column — contract §6.11,
-- checklist item 17.

-- +goose Down
DROP TABLE deletion_log;
DROP TABLE user_credential;
DROP TABLE user_profile;
DROP TABLE auth_method;
