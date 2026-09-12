# Decision: Context propagation into the DAO layer

Pattern: rotating devil's advocate, decision 2 of 3 (seat: Nolan Reyes). Proposer: Renata Cole.

## Proposal

Every `Repository` method (and its three sub-interfaces — `Profiles`/`Credentials`/`Methods`) takes `context.Context` as its first argument, propagated unbroken from the HTTP request through the handler into every DB call, so request cancellation/timeout actually cancels in-flight queries.

## Objections — Nolan Reyes (Veteran)

Agreed with the direction but pushed on three failure modes from production experience:

1. **Cancellation racing a multi-statement write.** A cancelled `SELECT` aborts cleanly; a cancelled multi-statement write (e.g., profile + credential saved together) risks a half-written row unless cancellation is scoped to a transaction, not a statement. Cited a migration job killed mid-batch by a deploy SIGTERM, leaving orphaned rows because cancellation was handled per-statement.
2. **Deadline budget, not a boolean.** Unbroken propagation means an inherited, already-partially-consumed deadline reaches the DAO call. If upstream work (e.g., a connector call) already burned most of the HTTP handler's timeout, the DAO gets a shrinking remainder and starts timing out under *normal* latency, not just failure — the good case starts looking like the bad case.
3. **Sub-interfaces are not uniformly cancel-safe.** Credential writes during auth are exactly the path that caused a real incident: a retry re-issued a credential write against an already-expired derived context and silently no-op'd instead of erroring. Wants explicit naming of which write paths deliberately detach from the inbound context (via `context.WithoutCancel` or a fresh background context) for durability, distinct from read paths that don't need it.

## Ruling (Renata Cole)

All three accepted; no decline.

1. Any `Repository` method that writes to more than one table (e.g., a profile+credential save) runs inside a single `Tx`. Context cancellation during that transaction triggers `Tx.Rollback`, never a partial commit — this is a stated requirement on the `Repository` implementation contract, not left to each backend's discretion.
2. Deadline propagation is a budget-allocation decision, stated explicitly in S3's middleware design: the HTTP-boundary timeout must be sized to cover realistic p99 DAO latency *plus* any upstream work (e.g., no DAO call should ever be handed a near-zero remaining deadline as a matter of routine operation). This gets a concrete number attached when S3's middleware design is written, not left as "some deadline."
3. Credential writes and any token-persistence write are named explicitly as the paths that use `context.WithoutCancel` (or an equivalent fresh background context with its own bounded timeout) for the actual commit — distinct from profile reads and other cancel-safe paths, which propagate the inbound context unmodified. This distinction is documented at the point each such method is implemented, not left implicit.

## Status

Ruled. Carries into S2 (Repository implementation contract: transaction scoping, detach points) and S3 (middleware deadline budget).

---
Model: sonnet (Nolan Reyes, objection seat; Renata Cole, lead, proposer + ruling). Turns consumed: 1 hire turn + lead synthesis.
