# F9 — phone search: partial-match requirement vs. 05's exact-only contract

**Finding:** `decisions/cross-track-consistency.md` F9 — `personas-use-cases.md:41` required partial-match search on both `name` and `phone`; `05-data-ops/multi-db-strategy.md` accepted phone as exact-match-only on the E.164-normalized value (a deliberate S2/S6 ruling, enforced by the index definition and already accepted by 03). Owner: Naomi (01's requirement is the one that has to move or be justified against an already-settled contract).

**Resolved directly with Priya Nandakumar (05 lead), two-party, per the precedent in `02-ai-security-architecture/planning-approach.md` §2** (a bounded, two-track decision that must still land in a durable file, not live only in chat).

**Priya's offer:** phone prefix match (`+4477…`) is cheap — the existing B-tree index on the normalized value serves it natively — and she'd add it if the persona work genuinely needs it. Infix ("contains 8891 anywhere") is not cheap on any backend and she'd push back on it. Her recommendation: keep phone exact-only, scope the UX partial-match toggle to name only, and record the phone limitation as deliberate with its reasoning stated.

**Ruling:** accepted in full, no prefix match requested. Persona 2 is explicitly described as someone working from "a phone number a customer just read aloud" — that is the exact-match case, not the partial-match case; the partial-match need is real for names (people mishear and misspell names, not digits) and E.164 normalization already absorbs the formatting variation that looks like a partial-match need on phone (different punctuation/spacing normalizing to one value). No persona in this track's work describes an analyst genuinely working from a partial or remembered fragment of a phone number.

**Applied:**
- `personas-use-cases.md`'s search-authorization appendix (line 41 as originally flagged) now states phone as exact-after-normalization, with the reasoning inline and the enforcement site named (the B-tree index on the normalized `phone` column in `multi-db-strategy.md`).
- `ux-notes.md`'s search-screen description now scopes the partial-match toggle to name only.

**Enforcement site:** `05-data-ops/multi-db-strategy.md`'s B-tree index on the E.164-normalized `phone` column (05's file, already accepted by 03) — 01 makes no independent claim about how the match is implemented, only that exact-after-normalization is the correct product requirement.

---
Model: sonnet (Priya Nandakumar, 05 lead, offer) / sonnet (Naomi Voss, 01 lead, ruling and edits applied). Turns consumed: 1 (direct two-party exchange, no hire turns — this is a lead-to-lead decision, not a pattern run).
