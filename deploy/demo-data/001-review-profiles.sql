-- Review/demo data for the joint LT-34/LT-39/LT-40/LT-51 live-URL review.
-- Per Priya's ruling: version-controlled, NEVER under migrations/ (so no
-- migration-Job code path can run it — it's not schema, it's fixture
-- data), applied by hand as a runbook step with the migrator credential,
-- idempotent, obviously fictional. Schema/CHECK constraints confirmed
-- against migrations/postgres/00001_initial_schema.sql + 00002 with
-- Renata before writing this.
--
-- Apply (after LT-51 lands, so the joint review has both data and a
-- working issuer to mint tokens against):
--
--   MIGRATOR_DSN=$(kubectl -n loginid-takehome get secret \
--     db-migrator-credential -o jsonpath='{.data.dsn}' | base64 -d)
--   psql "$MIGRATOR_DSN" -v ON_ERROR_STOP=1 -f deploy/demo-data/001-review-profiles.sql
--
-- Fixed UUIDs, not gen_random_uuid(): user_profile has no unique
-- constraint beyond its own PK (per Renata — no natural conflict target
-- exists), so idempotency here means ON CONFLICT (id) DO NOTHING against
-- ids chosen once and hardcoded, not letting the database generate a new
-- one every run. user_credential's ON CONFLICT targets the actual unique
-- index (on LOWER(username)), matching its expression, not a plain
-- equality target.

BEGIN;

INSERT INTO user_profile (id, name, phone, street_address, locality, region, postal_code, country, source)
VALUES
  ('00000000-0000-4000-8000-000000000001', 'Review Demo One',   '+12025550142', '742 Evergreen Terrace', 'Springfield', 'IL', '62704', 'US', 'direct'),
  ('00000000-0000-4000-8000-000000000002', 'Review Demo Two',   '+441134960001', '1 Test Close',          'Leeds',       NULL, 'LS1 1AA', 'GB', 'direct'),
  ('00000000-0000-4000-8000-000000000003', 'Review Demo Three', '+16135550199', '10 Sample Street',       'Ottawa',      'ON', 'K1A 0B1', 'CA', 'direct')
ON CONFLICT (id) DO NOTHING;

-- Reserved test/fictional ranges used above, not real subscriber numbers:
--   +1 202 555 01xx / +1 613 555 01xx — NANP reserved fictional exchange (555-01xx)
--   +44 113 496 0xxx                  — Ofcom-reserved UK "drama" number range

INSERT INTO user_credential (id, user_id, username, method_id, secret_state)
VALUES
  ('00000000-0000-4000-8000-000000000101', '00000000-0000-4000-8000-000000000001', 'demo.one@example.com',   (SELECT id FROM auth_method WHERE name = 'password'), 'none'),
  ('00000000-0000-4000-8000-000000000102', '00000000-0000-4000-8000-000000000002', 'demo.two@example.com',   (SELECT id FROM auth_method WHERE name = 'password'), 'none'),
  ('00000000-0000-4000-8000-000000000103', '00000000-0000-4000-8000-000000000003', 'demo.three@example.com', (SELECT id FROM auth_method WHERE name = 'password'), 'none')
ON CONFLICT (LOWER(username)) DO NOTHING;

-- secret_state = 'none' and secret/hash_algo/hash_cost all left NULL,
-- per the schema's tying CHECK (a 'set' state would require all three
-- non-NULL) — no credential material anywhere in this file, by
-- construction, not by omission someone has to remember.

COMMIT;
