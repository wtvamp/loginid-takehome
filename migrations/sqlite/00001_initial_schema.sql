-- +goose Up
-- SQLite DDL per 05-data-ops/multi-db-strategy.md §5. Same four tables and
-- the same constraint names as migrations/postgres/, in SQLite types and
-- collations — no trigram index (no SQLite equivalent, omitted rather than
-- faked, per the contract's dialect table). Names must match
-- migrations/postgres/00001_initial_schema.sql exactly so the two
-- migrations stay diffable against each other.
--
-- No `PRAGMA foreign_keys = ON` here — it's per-connection, not a
-- property of the database file, and a no-op inside goose's own
-- transaction. It would silently imply the schema enforces something a
-- migration can't actually turn on. Enforcement site is the DSN
-- (`_pragma=foreign_keys(1)`), applied on every connection by
-- construction — see internal/dao/sqlite/sqlite.go (05's amendment A3,
-- surfaced during PR #8 review).

CREATE TABLE auth_method (
	id TEXT,
	name TEXT NOT NULL,
	requires_secret INTEGER NOT NULL,
	is_active INTEGER NOT NULL,
	created_at TEXT NOT NULL,
	CONSTRAINT pk_auth_method PRIMARY KEY (id),
	CONSTRAINT uq_auth_method_name UNIQUE (name)
);

CREATE TABLE user_profile (
	id TEXT,
	name TEXT COLLATE BINARY NOT NULL,
	phone TEXT,
	street_address TEXT,
	locality TEXT,
	region TEXT,
	postal_code TEXT,
	country TEXT,
	source TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	CONSTRAINT pk_user_profile PRIMARY KEY (id),
	CONSTRAINT ck_user_profile_name_nonempty CHECK (length(trim(name)) > 0),
	CONSTRAINT ck_user_profile_phone_e164 CHECK (phone IS NULL OR (phone GLOB '+[1-9]*' AND length(phone) BETWEEN 8 AND 16 AND NOT phone GLOB '*[^+0-9]*')),
	CONSTRAINT ck_user_profile_street_address_nonempty CHECK (street_address IS NULL OR length(trim(street_address)) > 0),
	CONSTRAINT ck_user_profile_locality_nonempty CHECK (locality IS NULL OR length(trim(locality)) > 0),
	CONSTRAINT ck_user_profile_region_nonempty CHECK (region IS NULL OR length(trim(region)) > 0),
	CONSTRAINT ck_user_profile_postal_code_nonempty CHECK (postal_code IS NULL OR length(trim(postal_code)) > 0),
	CONSTRAINT ck_user_profile_country_alpha2 CHECK (country IS NULL OR (country = upper(country) AND length(country) = 2)),
	CONSTRAINT ck_user_profile_source CHECK (source IN ('direct','idp_cache'))
);

CREATE INDEX idx_user_profile_phone ON user_profile (phone);
-- No trigram/expression index here — SQLite has no GIN equivalent;
-- Search falls back to a full scan, documented in the contract.

CREATE TABLE user_credential (
	id TEXT,
	user_id TEXT NOT NULL,
	username TEXT NOT NULL,
	method_id TEXT NOT NULL,
	secret BLOB,
	secret_state TEXT NOT NULL,
	hash_algo TEXT,
	hash_cost INTEGER,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
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
	id TEXT,
	profile_id TEXT NOT NULL,
	source TEXT NOT NULL,
	reason TEXT NOT NULL,
	external_ref TEXT,
	job_run_id TEXT,
	deleted_at TEXT NOT NULL,
	CONSTRAINT pk_deletion_log PRIMARY KEY (id),
	CONSTRAINT ck_deletion_log_reason CHECK (reason IN ('retention_sweep','subject_request'))
);
-- No name, phone, address, username or secret column — contract §6.11.

-- +goose Down
DROP TABLE deletion_log;
DROP TABLE user_credential;
DROP TABLE user_profile;
DROP TABLE auth_method;
