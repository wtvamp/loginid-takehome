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
3. Credential writes are named explicitly as the path that needs a detach point for durability, distinct from profile reads and other cancel-safe paths, which propagate the inbound context unmodified. This distinction is documented at the point each such method is implemented, not left implicit.

## Status

Ruled. Carries into S2 (Repository implementation contract: transaction scoping, detach points) and S3 (middleware deadline budget).

## Resolved against 05's contract (2026-09-12)

Oren's first-pass review of 05's DAO contract flagged a real tension: this ruling's item 1 requires any multi-table write to run in a single `Tx`, but the contract as first landed had no method spanning both `user_profile` and `user_credential`, and explicitly barred 03 from composing a cross-call transaction — leaving registration's profile+credential creation with no atomic path at all. 05 resolved it by adding `CreateProfileWithCredential(ctx, p, c)` on the `Repository` composite (05's §3b) — the one cross-table operation in the contract, DAO-generated IDs for both rows, single transaction, `c.UserID` ignored on input and set from the just-created profile. This ruling's item 1 now has exactly one caller-visible method satisfying it, rather than being a requirement with no concrete instance.

## Consistency-pass fixes (2026-09-12)

**F19 — a real contradiction with 05's contract, not just stale wording.** 05's `multi-db-strategy.md` states "`Context` is first on every method and cancellation is honoured" — a blanket guarantee. Item 3 above, as originally written, said credential writes detach via `context.WithoutCancel` *inside the DAO*, which would mean the DAO does not honor cancellation on those specific methods — directly contradicting 05's guarantee. Fixed: **the detach happens in the handler, before the DAO call, not inside the DAO.** The handler passes the DAO a fresh, independently-bounded context for a credential write (not derived from the inbound request's cancellation tree, but with its own timeout) — from the DAO's perspective, cancellation of the context it was given is still honored unconditionally, exactly as 05's contract states. The DAO layer's guarantee is never weakened; the detach decision is a handler-layer policy about *which* context to hand the DAO, not an exception the DAO carves out for itself.

**F17 — stale references to a token cache dropped.** Item 3 originally also named "any token-persistence write" as a detach point, left over from before 02's S3 ruling settled on no-cache-by-default for the connector's vendor tokens (`decisions/connector-token-lifecycle-redblue.md`). There is no token-persistence write in the current design, so the phrase is removed (done above) rather than left to describe a mechanism that no longer exists.

---
Model: sonnet (Nolan Reyes, objection seat; Renata Cole, lead, proposer + ruling). Turns consumed: 1 hire turn + lead synthesis.
