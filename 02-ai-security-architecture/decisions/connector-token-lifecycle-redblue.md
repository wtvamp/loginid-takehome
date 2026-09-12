# Decision: Connector Token Lifecycle — Red/Blue Run (S3)

Pattern: red team / blue team, amended per `CASTING.md` §5 ⟨02⟩. Surface: the IDP connector's handling of the vendor `access_token` (`connector-security.md` §1), against the assignment's `/auth` → `/identity` contract. Table shape: `planning-approach.md` Appendix A. This record is the full positions and rebuttals; the completed table is appended to `connector-security.md`.

## Seats

- **Red — Tomasz Wrede.** Ranked top five attack leaves against the design as it stood before this run.
- **Alternatives — Felix Adebayo.** One turn between red and blue: design changes that delete a leaf outright.
- **Blue — Helena Marsh.** Answers every leaf with a control or an explicitly accepted risk.
- **Rebuttal — Tomasz Wrede.** One turn on the top three, after blue's answer.
- **Lead — Marcus Ilori.** Residual scoring and ruling.

## Red: Tomasz's ranked top five

1. **Cache-key confusion → cross-tenant token/PII leak.** The pre-existing design said the cache is "keyed so one vendor-account's token is never returned to a caller resolving a different vendor account" without stating the key derives from an immutable vendor-account id rather than caller-supplied `name`/`phone`. If the key derives from request fields, an attacker who knows or guesses another user's name+phone collides with their cache entry.
2. **Fail-open on KMS unavailability.** §1 mandated encryption at rest via KMS but never stated the failure mode if KMS was down or slow during a caching window — a plaintext-cache fallback "to preserve availability" would be an instant compromise path.
3. **Vendor token replay, no binding to caller/session.** The cached token, once decrypted for use, had no stated binding (no DPoP-style proof-of-possession, no IP/session pinning); anyone reading it during its TTL could replay it against the vendor for the remaining TTL.
4. **Process-memory/crash-dump exposure.** The plaintext token transits application memory before any encryption happens; no stated memory-scrubbing or core-dump suppression.
5. **Connector as a credential-stuffing oracle against the vendor.** Rate-limiting was stated ("independent of the vendor's own limits") without stated granularity — per-caller, per-IP, or global.

Noted but not in the top five (Tomasz's own list, kept for the record, not scored in this run): long vendor-token TTLs treated as inherently acceptable without a stated ceiling; a concurrent-refresh race causing a thundering herd against vendor `/auth`; scope-narrowing silently dropped for vendors that don't support scoping.

## Alternatives: Felix's turn

Applied "what if the connector held no token at all?" against the specific five, not in the abstract:

- **Leaves 1 and 2 deleted by the same design change:** make caching forbidden by default rather than merely optional. Fetch a fresh vendor token per `/identity` call, hold it only inside that request's execution scope, discard on return. No cache → no cache key → leaf 1 has no substrate to exist in. No cache → no KMS-encrypted-at-rest dependency in the hot path → leaf 2's failure window doesn't exist.
- **Leaf 4 shrunk, not deleted:** a token living only inside one request's stack for one outbound call is a much smaller target than one sitting in an encrypted cache with a multi-minute TTL, but it still exists in memory while in use — that's physics, not a design flaw to wish away.
- **Leaves 3 and 5 not eliminated, stated plainly rather than stretched for:** replay resistance requires vendor-side PoP support the assignment's generic ABC/XYZ contract doesn't give us — "hold no token" narrows the exposure window but doesn't change the vendor's own bearer-token semantics. Leaf 5 is a rate-limiting problem on our own `/auth` pass-through, not a token-custody problem — no change to how we hold the token touches how often a caller can hit us.
- **Net proposal:** replace "TTL-bounded encrypted cache, optional" with "no persistent or cached token; fetched fresh and scoped to the single `/identity` request" as the default, with caching as an explicitly-justified exception requiring its own re-review. Flagged honestly: does nothing for replay or rate-limiting, and may add a vendor round-trip per call — Helena's tradeoff to weigh against latency.

## Blue: Helena's ruling and answers

**On Felix's proposal:** accepted. Not a reversal of §1 — a tightening of language that already made caching conditional into a hard default: caching is forbidden unless an explicit exception is opened, with its own re-review. Controls are written for the exception path only, since the default posture has no substrate for leaves 1 and 2 to attack.

| # | Leaf | Blue response | Owner | Source |
|---|---|---|---|---|
| 1 | Cache-key confusion | Control, exception-path only: any future caching exception must derive the key from an immutable, vendor-issued identifier (our tenant id + vendor id + the vendor's own opaque subject id) — never from request-supplied `name`/`phone`. Same object-level-authorization discipline as scope checking on the search API; a cache key is an authorization boundary. | 03, if exception ever opened | OWASP API1:2023 (BOLA) |
| 2 | KMS fail-open | Control, exception-path only: if the envelope key is unreachable, cache write/read fails closed to "no cache" for that call — a live vendor fetch, not an unencrypted write and not a hard connector failure. | 03/04 (KMS mechanism is 04's) | NIST SP 800-57 |
| 3 | Replay, no PoP | Accepted risk. Bearer tokens under RFC 6750 are possession-equals-authorization by design; PoP/binding needs vendor-side support not assumable for a generic vendor. Acceptable for this system because fetch-per-call bounds the window to one request's execution scope. Re-review trigger: a PoP-capable vendor surfacing in 01's research. | — | RFC 6750 §5; RFC 9700 §2.5 |
| 4 | Memory/crash-dump exposure | Control: exclude the token field from any generic request/response dump used in error handling (panics, debug dumps — same discipline as logs); scope the variable to the narrowest function, no passing beyond the vendor-call function. Residual accepted: Go gives no guaranteed memory zeroing without unsafe/cgo, so a crash dump mid-call could still capture it — acceptable given the window is one function call, not a cache TTL. Core-dump suppression is a 04 mechanism, requirement recorded here. | 03 (code discipline) / 04 (runtime config) | OWASP Logging Cheat Sheet |
| 5 | Credential-stuffing oracle | Control: rate limit keyed per calling-API-client and per source IP, with a stricter threshold on vendor-auth failure ratio specifically (credential stuffing shows as high failure ratio before high volume), paired with exponential backoff per identity attempted. Closes the granularity gap §4 already gestured at rather than adding a new requirement. | 03 (rate-limit logic) / 04 (shared store/edge mechanism, if gateway-enforced) | OWASP API4:2023; NIST SP 800-63B §5.2.2 |

## Rebuttal: Tomasz's turn on the top three

- **Leaf 1 — conceded, conditional.** The persistent-cache variant is dead: no cache, no key-collision cache hit. But "no cache by default" only kills the storage leaf, not a connection-reuse variant Helena and Felix didn't address: a pooled/keep-alive HTTP client that doesn't reset the `Authorization` header per call can attach request A's token to request B's outbound call — same cross-tenant leak, zero cache required. Asked for an explicit requirement that the outbound client construct a fresh, isolated auth header per request, not per pooled connection.
- **Leaf 2 — conceded outright.** No cache removes the write-time KMS dependency; the exception-path fail-closed control is exactly right. No hole found.
- **Leaf 3 — held, with a boundary condition, not reopening the ruling.** Agrees bearer + no PoP for a generic vendor is a real constraint, not a design failure, and fetch-per-call shrinks the window. But "one request's execution scope" needed the boundary stated explicitly: fetch → use in the immediately following call → zeroize, and specifically whether a retried `/identity` call (§4 permits retry/backoff) reuses the same token or forces a fresh `/auth` — if it reuses, the accepted window is however long the retry policy runs, not "one call."

## Ruling (Marcus Ilori)

Both of Tomasz's rebuttal points are accepted as tightenings, not reopenings of Helena's ruling, and are folded into `connector-security.md` §1 directly:

1. **Leaf 1, connection-reuse variant — closed.** Added as a required implementation discipline: the outbound HTTP client to the vendor must construct a fresh, isolated `Authorization` header per request, never inherited from connection reuse.
2. **Leaf 3, boundary — tightened.** Added explicit fetch-use-zeroize language: the token's scope runs from the `/auth` response to the immediately following `/identity` call and no further; a retried `/identity` call re-fetches via `/auth` and never reuses a token from a prior attempt.

Residual risk: leaves 1, 2, 4, 5 score **L** after blue's response and this run's tightening — each has either had its substrate eliminated by design or a checkable control that closes the gap Tomasz named. Leaf 3 scores **M**: accepted, not eliminated, because it is a genuine vendor-contract limitation this project has no lever to remove, not a design gap left unaddressed. No leaf scores **H**; no follow-up Story is required. Leaf 3's **M** is named here as the one residual risk Warren should see plainly in the artifact, per the amendment's own rule.

---
Model: sonnet (Tomasz Wrede, red + rebuttal; Felix Adebayo, alternatives; Helena Marsh, blue) / sonnet (Marcus Ilori, lead, ruling and residual scoring). Turns consumed: 5 (red top-five, alternatives, blue, rebuttal, lead ruling) — matches the 4-turns-plus-lead budget in `PLAN.md` §3.
