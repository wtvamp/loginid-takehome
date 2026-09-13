-- +goose NO TRANSACTION
-- Required: CockroachDB rejects an ADD CONSTRAINT reusing a name a DROP
-- CONSTRAINT removed earlier in the same transaction ("duplicate
-- constraint name") — each statement below must commit on its own before
-- the next one runs.
-- +goose Up
-- Contract amendment A6 — see migrations/postgres/00002_phone_e164_narrow_range.sql
-- for the full reasoning (same defect, same fix, kept identical across
-- engines). CockroachDB's schema changes are online by default (no
-- ACCESS EXCLUSIVE-equivalent full-table lock the way an unvalidated
-- Postgres ADD CONSTRAINT would take), so this is a single ADD/DROP pair
-- rather than the NOT VALID + VALIDATE CONSTRAINT split used there —
-- CockroachDB doesn't support NOT VALID on a CHECK constraint anyway.
ALTER TABLE user_profile DROP CONSTRAINT ck_user_profile_phone_e164;
ALTER TABLE user_profile ADD CONSTRAINT ck_user_profile_phone_e164
	CHECK (phone IS NULL OR phone ~ '^\+[1-9][0-9]{6,14}$');

-- +goose Down
ALTER TABLE user_profile DROP CONSTRAINT ck_user_profile_phone_e164;
ALTER TABLE user_profile ADD CONSTRAINT ck_user_profile_phone_e164
	CHECK (phone IS NULL OR phone ~ '^\+[1-9][0-9]{1,14}$');
