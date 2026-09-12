# Track: Data Ops

Refer back to `../CLAUDE.md` for the full assignment text and how this track fits with the other four.

## Mission

Own the data model and the multi-database strategy that question 1 requires, plus the data governance implications of storing `user_profile` PII and `user_credential` secrets. The engineering track implements this track's design; this track does not write the Go DAO code itself.

## Owns

- Schema design for `user_profile` (name, address, phone) and `user_credential` (username, method, password), including how `method` is modeled (e.g., an enum for password vs. other auth methods) and how the two entities relate.
- The multi-database abstraction strategy: how one DAO interface serves PostgreSQL/CockroachDB and SQLite without diverging behavior — e.g., a repository interface with per-driver implementations, and which SQL dialect differences (upserts, autoincrement, JSON columns) need to be handled.
- Migration strategy: how schema changes would be versioned and applied across the supported databases.
- Data governance for PII: retention policy for `user_profile` data and for any cached data pulled from the third-party IDP connector's `/identity` endpoint, and how that interacts with credential storage (`user_credential` should never store PII, and the reverse).
- Indexing/query design to support "search and retrieve" from question 2 (e.g., what fields the API needs to search on, and what that implies for schema indexes).

## Does not own

- Access control and transport security for that data — that's `../02-ai-security-architecture/CLAUDE.md`.
- Writing the DAO code itself — that's `../03-engineering-delivery/CLAUDE.md`; this track hands it a schema and an interface contract to implement.
- Where the databases actually run and how migrations are executed in a pipeline — that's `../04-infra-devops/CLAUDE.md`.

## Deliverables

- Schema definitions (DDL or equivalent) for `user_profile` and `user_credential`, across the target databases.
- A written multi-database abstraction strategy the engineering track can implement directly.
- A migration approach.
- A short PII retention/governance note.

## AI tooling note

Record which AI tool/workflow was used to produce this track's schema and strategy design and how.

## Status

No data-ops work has started. This is the track the engineering DAO code most directly depends on — its schema and interface contract should land early.

<!-- profile-gen:start slug=priya-nandakumar -->
@profiles/priya-nandakumar/priya-nandakumar.md
<!-- profile-gen:end slug=priya-nandakumar -->
