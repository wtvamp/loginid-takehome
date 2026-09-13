-- +goose Up
-- Postgres/CockroachDB DDL per 05-data-ops/multi-db-strategy.md §5.
-- Every constraint is named — error translation matches on SQLSTATE plus
-- constraint name, never message text (contract §4; ddl-review-checklist.md B).

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE auth_method (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	name TEXT NOT NULL UNIQUE,
	requires_secret BOOLEAN NOT NULL,
	is_active BOOLEAN NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_profile (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	name TEXT COLLATE "C" NOT NULL CHECK (length(trim(name)) > 0),
	phone TEXT CONSTRAINT ck_user_profile_phone_e164 CHECK (phone IS NULL OR phone ~ '^\+[1-9][0-9]{1,14}$'),
	street_address TEXT CONSTRAINT ck_user_profile_street_address_nonempty CHECK (street_address IS NULL OR length(trim(street_address)) > 0),
	locality TEXT CONSTRAINT ck_user_profile_locality_nonempty CHECK (locality IS NULL OR length(trim(locality)) > 0),
	region TEXT CONSTRAINT ck_user_profile_region_nonempty CHECK (region IS NULL OR length(trim(region)) > 0),
	postal_code TEXT CONSTRAINT ck_user_profile_postal_code_nonempty CHECK (postal_code IS NULL OR length(trim(postal_code)) > 0),
	country TEXT CHECK (country IS NULL OR (country = upper(country) AND length(country) = 2)),
	source TEXT NOT NULL CHECK (source IN ('direct','idp_cache')),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Expression index on LOWER(name), not the bare column — a GIN index on
-- bare name is not used by a LOWER(name) predicate (checklist item 7).
CREATE INDEX idx_user_profile_name_trgm ON user_profile USING GIN (LOWER(name) gin_trgm_ops);
CREATE INDEX idx_user_profile_phone ON user_profile (phone);

CREATE TABLE user_credential (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id UUID NOT NULL REFERENCES user_profile(id) ON DELETE CASCADE,
	username TEXT NOT NULL,
	method_id UUID NOT NULL CONSTRAINT fk_user_credential_method REFERENCES auth_method(id),
	secret BYTEA,
	secret_state TEXT NOT NULL CHECK (secret_state IN ('none','set','revoked')),
	hash_algo TEXT,
	hash_cost INTEGER,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
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
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	profile_id UUID NOT NULL,
	source TEXT NOT NULL,
	reason TEXT NOT NULL CONSTRAINT ck_deletion_log_reason CHECK (reason IN ('retention_sweep','subject_request')),
	external_ref TEXT,
	job_run_id TEXT,
	deleted_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- No name, phone, address, username or secret column — contract §6.11,
-- checklist item 17.

-- +goose Down
DROP TABLE deletion_log;
DROP TABLE user_credential;
DROP TABLE user_profile;
DROP TABLE auth_method;
