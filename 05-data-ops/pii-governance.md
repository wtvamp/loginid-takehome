# PII Data Governance Note

Owner: 05-data-ops. Covers `user_profile` retention, IDP-cached PII (question 3's `/identity` response), and how retention interacts with `user_credential`. Threat modeling and access control for this data are `02-ai-security-architecture`'s call, not restated here — see `../02-ai-security-architecture/CLAUDE.md`.

## Data minimization

The schema collects only the fields the assignment names: name, phone, and the specific address subfields the `/identity` endpoint returns (`street_address`, `locality`, `region`, `postal_code`, `country`). No speculative fields (e.g., date of birth, government ID) were added. If a future requirement needs more PII, that's a deliberate schema change and a deliberate governance decision, not a default to expand into.

## Retention

`user_profile` rows are not retained indefinitely by default. Two clocks apply, distinguished by the `source` column:

- **`source = 'direct'`** — profile created through this system's own registration flow. Retention window is a product/legal decision this track doesn't own outright, but the schema supports enforcing whatever window is chosen: `created_at`/`updated_at` plus a scheduled deletion job keyed on those columns.
- **`source = 'idp_cache'`** — profile hydrated from a third-party IDP connector's `/identity` response. This data is subject to the **same retention obligation as directly-collected PII** — caching it does not reset or relax that obligation. If anything it adds a vendor-processor relationship on top: this system is now downstream custodian of another party's collected PII, which should be documented as a data-flow (third-party collection → this system → disposal) rather than left implicit.

Tagging cache rows distinctly by `source` is what makes a targeted deletion request or a bulk retention sweep possible without touching directly-collected profiles by accident.

## What must never happen, structurally enforced

- **`user_credential` never carries PII.** No name, phone, or address column exists on that table, and the `UserCredential` type described in `multi-db-strategy.md` has no fields that could hold it. A credential-store breach exposes usernames and hashes, not addresses.
- **`user_profile` never carries a secret.** No password hash or raw secret column exists on that table. A profile-store breach exposes PII, not authentication material.
- **The third-party IDP's own `/auth` credentials (the `username`/`password` this system POSTs to the vendor's `/auth` endpoint) are never persisted in `user_credential` or anywhere else in this schema.** That table is for this system's own login credentials. Storing a vendor's auth secret here would mean this system is custodian of a third party's credential material with no assignment requirement to do so — a clear scope and governance violation, not just a modeling mistake.

This separation is treated as a governance control, not merely a normalization convenience: it's the reason a breach of one table doesn't cascade into exposing the other class of data.

## Retention windows (S5)

**Every number below is a proposed POC default pending a product/legal ruling.** One asymmetry belongs stated plainly rather than buried: GDPR Art. 5(1)(e) storage limitation is a **principle** — data kept "no longer than is necessary" — demanding a window someone justifies. It is not a number anyone can look up. So every basis below is either an engineering rationale or the honest word *unknown*. There is no third kind, and a citation implying a regulation supplies a specific duration would be a false statement in a client-facing document.

| Data class | Clock column | Proposed window | Basis | Disposal | What cascades | Ratifier | Status |
|---|---|---|---|---|---|---|---|
| `user_profile`, `source='direct'` | `updated_at` | **24 months** since last update | *unknown — needs product/legal ruling.* Engineering basis only: long enough that a dormant account surviving a slow re-engagement cycle is not destroyed, short enough to be a defensible answer to "why do you still hold this." No regulation supplies this number | Hard delete | Cascade FK destroys all `user_credential` rows — the account stops existing | unknown — no named legal owner on this POC | Proposed |
| `user_profile`, `source='idp_cache'` | `updated_at` | **30 days** since last hydration | Engineering: **a cache is not a record.** Re-hydration costs one `/identity` call, so deleting too early costs a vendor round-trip. Vendor data also goes stale — a 14-month-old cached address is a correctness problem before it is a privacy one. We are downstream custodian of another party's collected PII with no independent relationship to the subject, which argues for the shortest window that still functions | Hard delete | Cascade FK; a cached profile should not normally have credentials — if it does, it is no longer merely a cache and product must say which clock wins | unknown | Proposed |
| `user_profile`, `source='idp_cache'`, **no referencing `user_credential`** (orphaned cache) | **`created_at`**, deliberately not `updated_at` | **7 days** since creation | Engineering: lookup residue, not an account — someone called `/identity`, we cached the answer, nothing was ever built on it. **The clock is `created_at` because `updated_at` is touched by the read and re-hydration path: an orphan re-read on any schedule would keep resetting its own expiry and never die.** Closing that loop is the entire reason this row exists | Hard delete | Nothing — by definition it has no children | unknown | Proposed |
| **Audit-log stream** (the emitted log entries evidencing deletions, as read by 04's sink) | the entry's own timestamp | **Proposed, pending ruling — see basis** | **unknown — needs a legal ruling.** The audit stream evidences that a deletion happened, which is useful for exactly as long as someone may ask. That period is a question about limitation windows and regulator expectations, not an engineering one, and no engineering rationale substitutes for it | Expiry at the sink | Nothing | unknown — needs product/legal ruling | **Proposed** |
| `user_credential` | — | **No independent window** | Wholly governed by its profile's window via the cascade FK. A credential cannot outlive its profile; a credential row with no profile cannot exist. A second clock could only ever disagree with the first, with no resolution rule | n/a | n/a | **Ruled** |

**The audit-stream row is enforced by 04, not by this schema.** Its enforcement site is 04's log-sink retention policy (`../04-infra-devops/observability.md`), which is what the consistency pass ruled and what `LT-20` cites. Nothing in the DAO reaches it: the sweep methods in `multi-db-strategy.md` §3c operate on `RetentionClass` values, all three of which are `user_profile` classes.

**The `deletion_log` table itself has no sweeper, and that is a named gap rather than an omission.** It is not in the table above because it would be a row nobody can act on. Two facts make it awkward in a way worth stating rather than tidying away:

1. **Nothing forces a window on it.** The table is PII-free by construction (`multi-db-strategy.md` §6.11), so it is *not* subject to the storage-limitation clock every other row here answers to. "Keep it forever" is therefore the path of least resistance rather than a decision anyone made — which is precisely why it needs one.
2. **No method sweeps it.** `DeleteExpired` takes a `RetentionClass`, and there is no class for it. If a window is ruled, implementing it is a **contract change** — either a fourth class or a separate method — not a configuration value someone can set. Flagging that now so the cost is visible at the time of the ruling rather than discovered after it.

Owner of the ruling: unknown, same as the windows above. Owner of the implementation once ruled: 05 (contract) then 03 (code), by the same route as F21.

### Two consequences that surprise people, stated loudly

1. **Deleting a profile silently destroys its credentials.** The account ceases to exist; there is no "profile deleted, login preserved" state. Deliberate.
2. **The reverse does not hold.** Deleting a credential leaves the profile standing. **A user who removes their last login method still has PII in this database, running on the `direct` clock.** If product wants "removing your last credential deletes the account," that is a policy decision, not a schema one — and it is currently *not* what the schema does.

### The cache window is "since last use", not "since first seen"

Re-hydration moves `updated_at`, so an actively-used cache row can live indefinitely. That is correct, and it must be written down, because it means the 30-day window measures **time since last use**. It is also exactly why the orphan row is keyed on `created_at`: the two rules are one rule seen from two sides — a row that is still being used should survive, and a row that is only being *re-read by the sweep's own neighbours* should not be able to refresh itself into immortality.

## The deletion mechanism (S5)

| Trigger | Selection predicate | Scope | Effect on `user_credential` | Disposal | Evidence left behind | What 04 provides | Failure mode if it never runs |
|---|---|---|---|---|---|---|---|
| **Bulk retention sweep** (scheduled) | `source` = the class, plus that class's clock older than that class's window; orphan class additionally requires no referencing credential. **One class per pass — never a mixed-class sweep**, because a single predicate spanning two clocks is how the wrong window gets applied to the wrong rows | Many rows | Cascade delete, implicit and unmentioned in the predicate — the sweep selects profiles and destroys credentials as a side effect. Stated because it is invisible at the point of use | **Hard delete** | One row in a separate `deletion_log`: profile UUID, `source`, `deleted_at`, reason `retention_sweep`, job run id. **No name, phone or address** | Schedule it; run it per class; emit the metrics below | **Nothing errors.** No exception, no alert, no symptom — rows accumulate and the policy is fiction nobody can see is fiction |
| **Subject-deletion request** | Exactly one profile id, resolved from the subject's identifier *before* the job runs — resolution is a lookup, not part of the deletion predicate | One profile plus its cascade | Cascade delete — the correct reading of "delete my data" | Hard delete, **same code path as the sweep** | Same `deletion_log` row, reason `subject_request`, plus the external request reference | An operator-invocable path that is **not** "someone runs SQL by hand against production" | Legal exposure rather than silent drift — this one gets noticed, which is why it must not be the only mechanism that works |
| **IDP re-hydration overwrite** | Existing `idp_cache` profile matched on the lookup key, fresh `/identity` response | One row | **None** — the profile id is stable across re-hydration, so credentials survive | Upsert in place; old field values are **gone, not versioned** | None, and none wanted | Nothing — request-path behaviour, not a job | n/a — but see the clock-reset note above |

**Why hard delete and not a soft-delete flag.** A flag means the PII is still sitting in the table, and every query written from that day forward must remember to filter it out. That is how a retention policy becomes decorative while still passing its own tests.

**Why subject deletion reuses the sweep's code path.** A separate "real delete" path used rarely is a path that is broken when you need it.

**Why the `deletion_log` holds no PII.** Otherwise the log is just a second copy of the problem with no clock on it. The retained UUID identifies a row that no longer exists and is not linkable to a person once the profile is gone.

### Where the mechanism lives in the contract

Added after the cross-track consistency pass (F21) found that none of the above was reachable from the DAO interface: the policy was specified here and had no method to be implemented against, and no track owned the gap.

`multi-db-strategy.md` §3c now defines, on the composite:

- **`DeleteExpired(ctx, class, olderThan, maxRows) (SweepResult, error)`** — one retention class per call, using that class's own clock column, cascading to credentials and writing a `deletion_log` row in the same transaction. **Batched: `maxRows` is required (1..10 000) and each batch is its own transaction**, so an interrupted sweep leaves fewer rows deleted rather than a half-written batch. The caller loops until `SweepResult.Drained` is true. It must not be called with a request-scoped context — a sweep cancelled because an inbound request went away is a retention policy silently dependent on request lifetimes.
- **`DeleteProfile(ctx, id, externalRef)`** for subject requests, on the same code path. **`reason` is not a parameter** — the method asserts `subject_request` internally, as `DeleteExpired` asserts `retention_sweep`. A caller-supplied reason code would let a copy-paste error mislabel an audit row, which destroys the one property the log exists to have: that its reasons are trustworthy.

`SweepResult` carries rows examined, rows deleted, `Drained`, and the age of the oldest surviving row — the health check below.

*(This paragraph previously described the pre-review signatures and was corrected after the consistency-pass re-run. It is the same failure this track has now hit three times: a document left pointing at a dependency that had moved.)*

### The health check, which is a requirement rather than a suggestion

A deletion job that stops running produces no error, no alert and no visible symptom. The only evidence is data that should be absent and isn't. So the sweep emits, per class and per run: **rows examined, rows deleted, and the age of the oldest surviving row in that class.**

The third is the actual health check. **04 should alert when the age of the oldest surviving row exceeds that class's window plus one sweep interval** — that is the condition that detects a job which has silently stopped, which rows-deleted counts cannot.

### Explicitly not modelled: derived data

Flagged by the hire who wrote the tables above, against her own stated blind spot — everything in them became a row because it fit a column, and this did not.

**A subject-deletion request currently reaches `user_profile` and, by cascade, `user_credential`. It does not reach derived data** — application logs, metrics, connector traces, or anything else that may have incidentally captured a name, phone or address in transit. That data has no `source` column, no retention clock, and in most cases no primary key to select on.

This is **not** a decision that derived data is out of scope for deletion. It is an unmodelled area, recorded as such so that its absence is not mistaken for a ruling. It sits across `../02-ai-security-architecture/` (what may be logged at all, and the never-log list) and `../04-infra-devops/` (log retention and where those logs physically live). The cheapest mitigation is upstream of this track entirely: if PII never enters a log, there is nothing to delete from it — which makes 02's never-log list the real control here, not a deletion job this track could specify.

## Handoff notes

- `02-ai-security-architecture` should build credential hashing (algorithm, cost factor, salt handling) on top of the `secret`/`hash_algo`/`hash_cost` columns already reserved in the schema — flagged directly to Marcus once this schema landed.
- `04-infra-devops` owns where retention sweeps and deletion jobs actually run (cron, scheduled job, pipeline step) — this note specifies the policy shape, not the execution mechanism.
