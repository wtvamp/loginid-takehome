# Decision: Search-API Authorization Scoping — Adversarial Pair (S4)

Pattern: adversarial pair, amended per `CASTING.md` §5 ⟨02⟩. Surface: the scope vocabulary, `profile:search` provisioning, object-level policy, pagination/rate limits, and grant lifecycle for the REST API in `api-auth-design.md`. This record is the full positions and rebuttal; the ruling is folded directly into `api-auth-design.md`.

## Seats

- **Proposer — Helena Marsh.** Drafts the scoping proposal.
- **Skeptic — Ingrid Solano.** Numbered objections, each with a "what would change my mind" line.
- **Proposer — Helena Marsh.** Answers every objection in writing.
- **Lead — Marcus Ilori.** Rules.

## Proposer: Helena's initial draft

**Scope vocabulary**, sharpened from `api-auth-design.md`'s original sketch: `profile:read:own` (id lookup, constrained to a `caller_referral` mapping — not a search), `profile:read:any` (id lookup, no referral constraint), `profile:search` (fuzzy name/phone query, its own gated capability, not "read:any plus search").

**`profile:search` provisioning bar (all four required):** named service identity, not a shared team credential; documented business function on file before provisioning; change-controlled approval with a named human approver, logged; tighter rate/result limits than any other scope.

**Object-level policy hook:** `authorize(sub, scope, target)` runs after the scope check — `read:own` checks `caller_referral` membership; `read:any` requires a logged `reason_code`; `search` additionally enforces a result cap, rejecting outright (not truncating) a query that would exceed it unconstrained.

**Pagination/rate limits:** cursor-based (not offset), default page 20 / ceiling 50; rate limits 60 req/min for `read:own`/`read:any`, 10 req/min for `search`, with alerting on sustained near-limit usage.

## Skeptic: Ingrid's five objections

1. **Scope vocabulary built before callers were listed.** No enumerated table of who actually calls each scope — can't tell whether `read:any` reflects a real caller or is invented for symmetry with `read:own`. *Would change my mind:* a table of named calling services/roles mapped to the scope each needs, before finalizing.
2. **`reason_code` for `read:any` is logged but not bounded to a controlled vocabulary.** A freeform string is worthless in an audit six months later; `search`'s provisioning bar is rigorous while `read:any`'s control is an order of magnitude looser for a scope that's *less* constrained at query time. *Would change my mind:* a closed enum of reason codes, same rigor as the search bar.
3. **`caller_referral` membership has no stated owner or change process.** `read:own` leans entirely on this mapping with no named approver or re-approval, while `search` gets a four-point bar — the narrowest scope is the softer target, not the safer one. *Would change my mind:* the same named-approver-plus-re-approval discipline applied to `caller_referral` changes, or a citation for why it needs less.
4. **"Reject outright" on an over-cap query stops single-request enumeration, not decomposition.** A caller at 10 req/min can walk the namespace in under-cap slices and reassemble the population over time; alerting only covers sustained near-limit usage, which a slow enumerator won't trigger. *Would change my mind:* a cumulative-distinct-subjects-per-caller-per-window counter feeding the same alerting path, or evidence that the provisioning bar makes slow enumeration commercially irrational for anyone who'd clear it.
5. **90-day re-approval is a calendar control, not a lifecycle control.** Nothing ties revocation to offboarding or role-change; a grant can survive up to 89 days past when it should have been pulled. *Would change my mind:* an explicit statement that revocation is event-triggered with the 90-day cycle as backstop only, or a citation that no such feed exists and the backstop is an accepted residual risk, named as such.

## Proposer: Helena's answers

All five accepted; none declined.

1. **Revised.** `profile:read:any` now requires an entry in an explicit caller-role table (service name, business justification, and why `read:own`'s referral constraint is insufficient for that caller) before the grant is issued — gated the same way `profile:search` already is. If no caller meets the bar, the scope stays defined but unissued, since "any known record, no referral, audited" is a real access pattern some back-office functions need, but nobody gets it by default.
2. **Revised.** `reason_code` replaced with a closed, versioned enum (`support_ticket`, `fraud_review`, `kyc_reverification`, `legal_hold`) defined in `api-auth-design.md`; adding a value requires a documented change, matching the `search` provisioning bar's rigor.
3. **Revised — the sharpest of the five.** `caller_referral` entries are data owned by `05-data-ops`, written only through the same partner-onboarding workflow as any other provisioning decision: named approver, logged, reviewed on the same cadence as scope grants. No ad hoc inserts outside that workflow.
4. **Revised.** Added a cumulative distinct-record-touch counter per caller per rolling window (e.g., no more than N distinct profiles returned to one credential per 24h without a fresh `reason_code`/justification), tracked independently of per-minute rate — catches decomposition regardless of how slowly it's spread. Counting-store mechanism is 03/04's to build; the requirement (cumulative, not just per-minute, measurement) is asserted here.
5. **Revised.** All three scope grants — not just `search` — are revoked immediately, ahead of the 90-day cycle, on: the named approver's offboarding, the caller's business function changing, or a 30-day idle trigger (no usage → revoke pending re-justification). Requires 03/04 to consume an offboarding/ownership-change signal; the requirement (lifecycle-triggered revocation, not solely calendar) is set here, the plumbing is theirs.

## Ruling (Marcus Ilori)

Accepted in full — Helena's revised proposal is the design. All five of Ingrid's objections identified real gaps (asymmetric rigor between scopes, a control with no owner, a decomposition path the rate limit didn't cover, a calendar-only lifecycle control) and every fix closes the specific gap named rather than gesturing at a general improvement. Ingrid's objection 3 (`caller_referral` ownership) was the strongest single point in this run — the narrowest-privilege scope having the weakest control around it is exactly the kind of asymmetry an adversarial pair exists to catch, and a Proposer working alone would have had no structural reason to notice it. Folded into `api-auth-design.md` directly; this is the scope vocabulary, provisioning bar, and lifecycle rule `handoff-03-auth.md` (S6) builds on next.

---
Model: sonnet (Helena Marsh, Proposer) / sonnet (Ingrid Solano, Skeptic) / sonnet (Marcus Ilori, lead, ruling). Turns consumed: 3 + lead (draft, objections, answer, ruling) — matches the 3-turns-plus-lead budget in `PLAN.md` §3.
