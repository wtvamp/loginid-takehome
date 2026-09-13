-- +goose Up
-- SQLite DDL per 05-data-ops/multi-db-strategy.md §5. Same four tables and
-- the same constraint names as migrations/postgres/, in SQLite types and
-- collations — no trigram index (no SQLite equivalent, omitted rather than
-- faked, per the contract's dialect table).

PRAGMA foreign_keys = ON;

CREATE TABLE auth_method (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	requires_secret INTEGER NOT NULL,
	is_active INTEGER NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE user_profile (
	id TEXT PRIMARY KEY,
	name TEXT COLLATE BINARY NOT NULL CHECK (length(trim(name)) > 0),
	phone TEXT CONSTRAINT ck_user_profile_phone_e164 CHECK (phone IS NULL OR (phone GLOB '+[1-9]*' AND length(phone) BETWEEN 8 AND 16 AND NOT phone GLOB '*[^+0-9]*')),
	street_address TEXT CONSTRAINT ck_user_profile_street_address_nonempty CHECK (street_address IS NULL OR length(trim(street_address)) > 0),
	locality TEXT CONSTRAINT ck_user_profile_locality_nonempty CHECK (locality IS NULL OR length(trim(locality)) > 0),
	region TEXT CONSTRAINT ck_user_profile_region_nonempty CHECK (region IS NULL OR length(trim(region)) > 0),
	postal_code TEXT CONSTRAINT ck_user_profile_postal_code_nonempty CHECK (postal_code IS NULL OR length(trim(postal_code)) > 0),
	country TEXT CHECK (country IS NULL OR (country = upper(country) AND length(country) = 2)),
	source TEXT NOT NULL CHECK (source IN ('direct','idp_cache')),
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE INDEX idx_user_profile_phone ON user_profile (phone);
-- No trigram/expression index here — SQLite has no GIN equivalent;
-- Search falls back to a full scan, documented in the contract.

CREATE TABLE user_credential (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL REFERENCES user_profile(id) ON DELETE CASCADE,
	username TEXT NOT NULL,
	method_id TEXT NOT NULL CONSTRAINT fk_user_credential_method REFERENCES auth_method(id),
	secret BLOB,
	secret_state TEXT NOT NULL CHECK (secret_state IN ('none','set','revoked')),
	hash_algo TEXT,
	hash_cost INTEGER,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
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
	id TEXT PRIMARY KEY,
	profile_id TEXT NOT NULL,
	source TEXT NOT NULL,
	reason TEXT NOT NULL CONSTRAINT ck_deletion_log_reason CHECK (reason IN ('retention_sweep','subject_request')),
	external_ref TEXT,
	job_run_id TEXT,
	deleted_at TEXT NOT NULL
);
-- No name, phone, address, username or secret column — contract §6.11.

-- +goose Down
DROP TABLE deletion_log;
DROP TABLE user_credential;
DROP TABLE user_profile;
DROP TABLE auth_method;
