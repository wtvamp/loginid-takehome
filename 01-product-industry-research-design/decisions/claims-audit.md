# S1 — Claims audit

**Auditor:** Desmond Okafor (Spreadsheet, claims-auditor seat). **Synthesis/edits:** Naomi Voss. **Pattern:** claims audit (one-hire review, not a catalog pattern). **Inputs:** `industry-framing.md`, `personas-use-cases.md`, `api-connector-design-rationale.md`, checked against `research-loginid-company.md`, `research-oauth-history.md`, `research-oidc-address-claim.md`.

## Method

Every quantitative or evaluative claim in the three framing/rationale docs gets a row: claim, location, evidence file, status (verified / asserted / marketing-sourced / softened), and — where the claim wasn't supportable as written — the softer wording it was changed to. No outside research was commissioned; PLAN.md §3 rules that out for narrative decoration, and none of the weak claims needed it — they needed softening, not sourcing.

## Table

| Claim | Where | Evidence | Status | Resolution |
|---|---|---|---|---|
| LoginID passkeys have "no shared secret ever transmitted or stored server-side" | `industry-framing.md` §2 | `research-loginid-company.md` §1 (marketing) | Verified to source (source itself is vendor marketing) | Kept as-is |
| "This is almost certainly intentional, not an oversight in the assignment's wording" | `industry-framing.md` §2 | None — inference about LoginID's intent | Asserted | Softened → "We read this as intentional, given the gap's precision" |
| "...it's the move this exercise is actually testing for" | `industry-framing.md` §4 | None — claims to know evaluator intent | Asserted | Softened → "the move we believe this exercise is testing for" |
| Dick Hardt = OAuth 2.0 editor (RFC 6749), not OAuth 1.0 co-author | `industry-framing.md` §3 | `research-oauth-history.md` §2, RFC citations | Verified | Kept as-is |
| LoginID's 2025–26 agentic-commerce pivot | `industry-framing.md` §3 | `research-loginid-company.md` §1/§3 (marketing, already flagged company-stated) | Verified-as-marketing-claim (correctly hedged already) | Kept as-is |
| ROPC is "discouraged" by most providers | `api-connector-design-rationale.md` §3 | `research-oidc-address-claim.md` Q2 → RFC 9700 §2.4: "MUST NOT be used" | Verified but understated | Strengthened → "RFC 9700 now states MUST NOT be used" |
| Address fields match OIDC Core §5.1.1 | `api-connector-design-rationale.md` §3 | `research-oidc-address-claim.md` | Verified | Kept as-is |
| "retrieval-by-ID is the easy 80%" | `personas-use-cases.md`, Persona 2 | None — unsourced figure | Asserted / unsourced number | Softened → "retrieval-by-ID is the low-friction case" |
| "fewer form fields at signup measurably reduces drop-off" | `personas-use-cases.md`, Persona 3 | None — marketing-pattern claim, no project evidence | Marketing-sourced pattern | Softened → "is widely believed to reduce drop-off" |
| "'sign in with X' patterns dominate consumer onboarding" | `personas-use-cases.md`, Persona 3 | None | Asserted | Softened → "are common in consumer onboarding" |

## Outcome

Two unsourced quantitative-flavored claims and two evaluator-intent claims softened in place across `industry-framing.md` and `personas-use-cases.md`; one claim (ROPC status) strengthened to match its actual source. All other claims in the three docs were already properly sourced or correctly hedged as vendor marketing — no further changes needed. Nothing required deep research; S2 (the structured written debate on the password-baseline framing) can now proceed arguing over claims that have been checked.

---
Model: sonnet (Desmond Okafor, claims audit) / sonnet (Naomi Voss, lead, synthesis and edits applied). Turns consumed: 1 (Desmond's audit, returned in full in one reply) + lead — matches the 1–2-turn budget in `PLAN.md` §1.
