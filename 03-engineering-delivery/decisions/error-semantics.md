# Decision: Error semantics (wrap vs. sentinel)

Pattern: rotating devil's advocate, decision 1 of 3 (seat: Oren Castellan). Proposer: Renata Cole.

## Proposal

Go 1.13+ error wrapping (`fmt.Errorf("...: %w", err)`) for propagating unexpected/lower-level errors up the call stack, plus a small set of exported sentinel errors (`dao.ErrNotFound`, `dao.ErrDuplicate`) for conditions callers are expected to branch on via `errors.Is`.

## Objections — Oren Castellan (Detail Hawk)

1. **Sentinel set must be closed and enumerated now**, not left open-ended — otherwise hires add ad hoc `ErrX` values across S2–S4 with no stable contract. Wants a fixed list plus a named owner and a propose-before-adding rule.
2. **Wrapped errors must never cross the HTTP boundary raw** — `err.Error()` on a wrapped DB error can leak driver text (DSN fragments, column/constraint names) to an API client. Wants an explicit translation function at the boundary; anything unmapped defaults to a generic 500 with no wrapped detail.
3. **`errors.Is` against a sentinel breaks silently if a backend returns a driver-native error instead** (e.g. `*pgconn.PgError` instead of `dao.ErrDuplicate`). Wants the `Repository` interface contract to state translation is *mandatory* per backend, verified by an S5 test case per backend.
4. **No stated rule for `context.DeadlineExceeded`/`context.Canceled`.** These aren't ordinary internal errors and shouldn't page as generic 500s. Wants them checked and mapped to a distinct response code before generic wrapping applies.

## Ruling (Renata Cole)

All four accepted; no decline.

1. Sentinel set is closed and fixed now: `dao.ErrNotFound`, `dao.ErrDuplicate`, `dao.ErrInvalidInput`. Defined once in `internal/dao`. Rule: no hire adds a sentinel unilaterally — propose it to me with the one-line reason, I add it. This applies for the rest of the design/implementation phase.
2. `internal/api` never serializes `err.Error()` on an unmapped error. A translation function (`internal/api` boundary layer) maps the three sentinels to their HTTP status/safe message; anything else defaults to a generic 500 with no wrapped detail surfaced to the client. Wrapped errors are for logs only.
3. The `Repository` interface contract states translation of known driver-native errors to the shared sentinels is **mandatory**, not optional, per backend implementation. S5's test strategy gets an explicit line item: one test per backend proving the translation happens (e.g., a unique-constraint violation surfaces as `dao.ErrDuplicate`, not a raw driver type).
4. `context.DeadlineExceeded` and `context.Canceled` are checked explicitly and mapped to a distinct response (504 for deadline exceeded; treat client-cancel as a non-alerting outcome, not logged as a 500) before generic wrapping/translation applies.

## Status

Ruled. Carries into S2 (DAO sentinel definitions + per-backend translation) and S3 (handler-layer translation function, context-error handling).

## Superseded in part — 05's contract lands (2026-09-12)

05's `multi-db-strategy.md` §4 is the authoritative sentinel set and translation mechanism for the DAO layer; it supersedes the specific names and the translation rule this decision guessed at before the contract existed. What stands vs. what's replaced:

- **Replaced:** the invented sentinel set (`ErrNotFound`/`ErrDuplicate`/`ErrInvalidInput`) is superseded by 05's actual seven: `ErrNotFound`, `ErrAlreadyExists`, `ErrDuplicateUsername`, `ErrInvalidMethod`, `ErrInvalidQuery`, `ErrInvalidArgument`, `ErrInvalidCredential`. These are 05's to own and extend (own-and-propose-before-adding now routes through them, not me, for anything DAO-internal) — my closed-set/no-ad-hoc-additions principle still applies, just to their list.
- **Replaced:** "match SQLSTATE only" is superseded by 05's corrected rule — match structured error codes (SQLSTATE on Postgres/CockroachDB, extended result codes on SQLite) plus constraint names we define ourselves in DDL, never free-text messages. This is a more complete version of objection 2's concern (leaking driver text), not a contradiction of it.
- **Stands unchanged:** the HTTP-boundary rule (objection 2 — `internal/api` never serializes `err.Error()` on an unmapped error; wrapped errors are for logs only) — this is 03's layer, not 05's, and 05's contract doesn't touch it. The context-cancellation rule (objection 4) also stands unchanged, same reason.
- **Stands, now backed by a real mechanism instead of a guess:** objection 3's demand for mandatory per-backend translation — 05's contract §4 gives the actual mapping table (condition → PG/CRDB code → SQLite code → sentinel), and §7's conformance suite makes "sentinel parity for every error condition in §4" a stated, required test — stronger than what this decision asked for.

---
Model: sonnet (Oren Castellan, objection seat; Renata Cole, lead, proposer + ruling). Turns consumed: 1 hire turn + lead synthesis.
