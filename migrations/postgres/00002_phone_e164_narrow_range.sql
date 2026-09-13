-- +goose NO TRANSACTION
-- Each statement below commits independently rather than all three
-- running inside goose's default per-file transaction — required on
-- CockroachDB (see the cockroachdb migration's identical directive and
-- comment: it rejects an ADD CONSTRAINT reusing a name a DROP CONSTRAINT
-- removed earlier in the same transaction) and harmless here, since
-- NOT VALID + VALIDATE CONSTRAINT's whole point is letting the
-- table-scan half take a lighter lock than a single ADD CONSTRAINT would.
-- +goose Up
-- Contract amendment A6: the original ck_user_profile_phone_e164 accepted
-- 2-15 total digits after '+' ('^\+[1-9][0-9]{1,14}$'), while SQLite's own
-- phone CHECK accepts 7-15 (length BETWEEN 8 AND 16, including the '+').
-- "+1234" (5 digits) passed here and failed there — a profile writable on
-- one backend and rejected on the other, from the same input. Narrowed to
-- 7-15 digits ('^\+[1-9][0-9]{6,14}$'), matching SQLite exactly.
--
-- Editing 00001's CHECK text in place would have been a no-op against any
-- database that already applied it — goose tracks applied version
-- numbers, not file content, and does not re-run a migration once
-- recorded (migration-approach.md: "goose will not re-run an applied
-- migration anyway"). This is a real ALTER, applied through the normal
-- goose pipeline like any other schema change (Nolan Reyes, PR #24
-- review — caught before this shipped as a migration file nobody's
-- database would ever actually run).
--
-- ADD CONSTRAINT ... NOT VALID + a separate VALIDATE CONSTRAINT, not a
-- single ADD CONSTRAINT: on Postgres, a plain ADD CONSTRAINT CHECK takes
-- an ACCESS EXCLUSIVE lock for the full duration of the table scan that
-- validates every existing row: NOT VALID skips that scan up front (a
-- brief metadata-only lock instead), and VALIDATE CONSTRAINT then checks
-- existing rows with a lock level that doesn't block ordinary reads/
-- writes. This table has no production data behind this take-home, but
-- the pattern is the one to use regardless (Nolan Reyes, PR #24 review).
-- CockroachDB's schema changes are online by default and don't need the
-- same two-step split; see the cockroachdb migration's own comment.
ALTER TABLE user_profile DROP CONSTRAINT ck_user_profile_phone_e164;
ALTER TABLE user_profile ADD CONSTRAINT ck_user_profile_phone_e164
	CHECK (phone IS NULL OR phone ~ '^\+[1-9][0-9]{6,14}$') NOT VALID;
ALTER TABLE user_profile VALIDATE CONSTRAINT ck_user_profile_phone_e164;

-- +goose Down
ALTER TABLE user_profile DROP CONSTRAINT ck_user_profile_phone_e164;
ALTER TABLE user_profile ADD CONSTRAINT ck_user_profile_phone_e164
	CHECK (phone IS NULL OR phone ~ '^\+[1-9][0-9]{1,14}$') NOT VALID;
ALTER TABLE user_profile VALIDATE CONSTRAINT ck_user_profile_phone_e164;
