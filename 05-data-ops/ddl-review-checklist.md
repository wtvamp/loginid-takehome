# DDL review checklist — what 05 checks before a migration PR merges

Owner: 05-data-ops (Priya Nandakumar). Audience: **track 03, before writing migrations** — not only 05 at review time. Published ahead of LT-36/LT-37 so the checks are known in advance rather than discovered in review; the same reasoning as amendment A1, where flagging a contract ambiguity before implementation cost one message and finding it at review would have cost a rewrite.

**This introduces no new decisions.** Every item derives from `./multi-db-strategy.md` (the contract), `./migration-approach.md`, or `./pii-governance.md`, and each line names its source. If an item here disagrees with the contract, the contract wins and I have made an error — tell me and I will amend.

## A. Layout

1. **`shared/` contains seed data only — no `CREATE TABLE`.** All DDL lives in `postgres/` and `sqlite/`. (`migration-approach.md` §2, corrected under consistency-pass finding F28.)
2. **`postgres/` serves PostgreSQL *and* CockroachDB.** No third directory. (Contract §9 of the out-of-scope list.)
3. **Two goose invocations, separate version tables** (`goose_db_version_shared`, `goose_db_version_<driver>`). One shared table across two directories interleaves independent sequences and corrupts history. (`migration-approach.md` §2.)

## B. Every constraint is named — this one is load-bearing

4. **No anonymous constraints anywhere.** Error translation matches on SQLSTATE *plus the constraint name*, never on message text, because CockroachDB's message and `DETAIL` structure are not portable and its SQLSTATEs have a documented history of drift. An unnamed constraint is a sentinel that cannot be produced. (Contract §4; `research-cockroachdb-postgres-semantics.md` §3.)
5. Names the contract already fixes: `uq_user_credential_username`, `fk_user_credential_method`, `ck_user_credential_secret_state`, `ck_user_profile_phone_e164`, `ck_deletion_log_reason`, and `ck_user_profile_{street_address,locality,region,postal_code}_nonempty`. (Contract §5.)

## C. The three differences that already bit us in design

These are the defects the behavioural conformance suite exists to catch. Each was invisible when either implementation was read alone.

6. **Collation is explicit on `user_profile.name` in both engines** — `COLLATE "C"` on Postgres/CockroachDB, `COLLATE BINARY` on SQLite — in the DDL, not only in `ORDER BY`. Without it, page 1 of a search returns different rows per backend: Postgres sorts by database collation, SQLite's default byte order puts every uppercase letter before every lowercase one. (Contract §6.3.)
7. **The trigram index is an expression index on `LOWER(name)` with `gin_trgm_ops`, not on the bare column** — a GIN index on bare `name` is not used by a `LOWER(name)` predicate, so the accelerator would accelerate nothing. Postgres/CockroachDB only; **absent on SQLite, omitted rather than faked.** (Contract §5 Indexes, §6.3.)
8. **`country` carries `CHECK (country IS NULL OR (country = upper(country) AND length(country) = 2))`** — normalization at write, not only at query. A stored `us` is silently invisible to a search for `US`. SQLite's `upper()` is ASCII-only, which is sufficient for alpha-2 and is written down so nobody assumes it generalizes. (Contract §6.5.)

## D. Per-column checks

9. **UUID primary keys on every backend** — server-generated on Postgres/CockroachDB, app-generated in Go before insert on SQLite. No per-backend PK strategy. (Contract §6.6.)
10. **Absence is single-valued: NULL means absent, `''` is invalid.** All four address columns nullable with non-empty `CHECK`s; `name` `NOT NULL` with `length(trim(name)) > 0`. (Contract §5.)
11. **`phone`** — regex `CHECK` on Postgres/CockroachDB, the `GLOB` form given in contract §5 on SQLite. Both named `ck_user_profile_phone_e164`. Exact-match only per finding F9; the B-tree index would support prefix, the contract does not offer it.
12. **`secret_state`** — the table-level `CHECK` tying `secret_state` to `secret`/`hash_algo`/`hash_cost` must be present, or a row with a secret and no algorithm becomes representable. (Contract §5.)
13. **Timestamps** — `TIMESTAMPTZ` on Postgres/CockroachDB, `TEXT` RFC3339 UTC on SQLite.
14. **Booleans** — native on Postgres/CockroachDB, `INTEGER` 0/1 on SQLite, mapped at the DAO boundary.

## E. Foreign keys and `deletion_log`

15. **`user_credential.user_id` → `user_profile(id)` `ON DELETE CASCADE`**; `method_id` → `auth_method(id)` **RESTRICT**. The asymmetry is deliberate and documented: deleting a profile destroys its credentials, deleting a credential leaves the profile standing. (Contract §6.2.)
16. **`deletion_log.profile_id` has NO foreign key** — the row it names is meant not to exist. (Contract §5.)
17. **`deletion_log` carries no name, phone, address, username or secret column.** A conformance test asserts this; the DDL must not make it false. (Contract §6.11.)

## F. Seed data

18. **`auth_method` seeded with one `'password'` row, idempotent via `INSERT ... ON CONFLICT (name) DO NOTHING`.** In `shared/`, as a migration — not a runtime bootstrap. (`migration-approach.md` §4.)
19. **New auth methods are inserted as data, never added as migrations.** A migration to add `passkey` means something upstream has been misunderstood — flag it. (Contract §6.7.)

## What I will not do at review

Re-open decisions. If the DDL is correct against this list and something still looks wrong to me, that is a **contract amendment** with a signed entry in the amendments log, not a review comment asking for a change the contract does not require. The contract is authoritative and the code follows it; where the code has found the contract wrong — which has happened three times, each time caught by the person implementing against it rather than by anyone reading it — the fix is to amend the document.

---

*Work-By: Priya Nandakumar (05 Data Ops lead, Claude Opus 5). Derived from existing rulings; no new decisions. Items 6–8 trace to defects found by Yusuf Karadag (DDL/nullability review) and the S2 adversarial pair with Anders Vogel; item 17 to Beatriz Achterberg's retention work; the F28 layout correction to the cross-track consistency pass.*
