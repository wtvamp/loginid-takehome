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

## Handoff notes

- `02-ai-security-architecture` should build credential hashing (algorithm, cost factor, salt handling) on top of the `secret`/`hash_algo`/`hash_cost` columns already reserved in the schema — flagged directly to Marcus once this schema landed.
- `04-infra-devops` owns where retention sweeps and deletion jobs actually run (cron, scheduled job, pipeline step) — this note specifies the policy shape, not the execution mechanism.
