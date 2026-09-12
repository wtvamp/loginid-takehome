# S3 — Newcomer's-question cold read of `api-connector-design-rationale.md`

**Reader:** Wesley Okonkwo (Newcomer, track 04), on loan under the bounded two-party arrangement approved by team-lead (2–4 turns total across S3/S6). Relayed via the 04 lead session (`04-infra-devops-47`) since Wesley's environment has no cross-session messaging. **Fixed by:** Naomi Voss. **Pattern:** newcomer's question, per `PLAN.md` S3 (primary target — this is the file 03 has to *act* on, per the hand-off standard in `planning-approach.md` §2).

## Method

Wesley read `api-connector-design-rationale.md` cold — as 03 would, no other project context — right after the "Contract summary for 03" section was added, and reported anything he could not act on without asking a question first.

## Findings (turn 1 of the loan)

- **Undefined terms on first use:** "discriminator" (as a column concept), "blast radius", "resource-owner-password-credentials grant" / "OAuth2 grant family" (assumed named-grant-type knowledge), "mTLS", and the OIDC Core `address` claim referenced without inlining its shape.
- **Unresolvable cross-references:** "Persona 2" / "Persona 3" named only by label, not defined in this file; "S2" used as unglossed shorthand for a decision record.
- **One real ambiguity, not just a missing gloss:** the `expand` parameter was called a "toggle" in §2 and a "flag" in the Contract Summary, with neither saying whether it's top-level in the `POST /profiles/search` body or nested — he said he'd have had to ask before writing a Go struct around it.
- Everything else in the Contract Summary (field names, cardinality, out-of-scope list) read as directly actionable.

## Fixes applied

- **`expand` shape resolved**, the one true ambiguity: it's a top-level `expand: bool` field in the search request body, sibling to the query fields — e.g. `{"name": "...", "phone": "...", "expand": false}`. Stated identically in both places it's mentioned (§2 and the Contract Summary), with the exact JSON shape given once and cross-referenced the second time so the two mentions can't drift again.
- **Persona 2 / Persona 3** now get a one-clause inline gloss at first use in each section ("the internal user who looks up a customer record...", "the onboarding flow that pre-fills a new user's profile at signup...") pointing to `personas-use-cases.md` for the full definition, rather than assuming the reader has read that file first.
- **ROPC** spelled out and given a one-clause definition ("one of several standard ways an OAuth2 client obtains a token, this one taking a raw username/password directly") rather than assumed.
- **"S2"** given a one-clause gloss ("the second story in `PLAN.md`'s task list, not a spec term") at its point of use in the out-of-scope list, since that's the reference most likely to confuse a reader who hasn't seen this track's plan.
- Left "discriminator," "blast radius," and "mTLS" as-is: each already carries an inline explanation or a named alternative at first use (mTLS appears in a list of three named auth mechanisms, not standalone), and 03 is an engineering track — over-glossing standard terms for an implementer reader is the opposite failure mode from the one this pattern exists to catch.

## Outcome

The one blocking ambiguity (the `expand` field's shape) is closed with a concrete JSON example; the cross-reference gaps that would have cost 03 a round-trip question are closed inline. No further changes to the Contract Summary's substance — field names, cardinality, and the out-of-scope list were already directly actionable per Wesley's read. Turn 1 of 2–4 used.

## S6 — secondary cold read of `industry-framing.md`

**Purpose:** evaluator-legibility test, not an implementer's read — can a reader with no other project context state the thesis and the post-S2 "before/after, not author-intent" distinction in their own words after one pass?

**Reader's restatement (turn 2 of the loan, relayed via `04-infra-devops-47`, Wesley's environment has no cross-session tool):** the submission builds a correctly-done, industry-standard password/legacy-connector baseline — not a mistake to silently "fix" with FIDO2 — but deliberately leaves visible extension seams (`user_credential.method`, the connector's provider interface) toward what LoginID actually sells (passkeys, OIDC federation, eventually agent-identity delegation), without a rewrite. Naming that gap is itself the signal of market fluency for evaluators. He further correctly identified that the document's current version walked back an earlier claim that LoginID deliberately chose this gap (dropped as unfalsifiable per the S2 debate), and now frames password + static connector as the conventional "before" any CIAM backend starts from, with FIDO2/OIDC/agent-identity as the "after" it evolves toward — regardless of what the assignment author meant. His one caveat: he inferred the S2 resolution from this document's own summary of it, not from reading `decisions/password-baseline-debate.md` directly.

**Outcome:** thesis and the before/after-vs-author-intent distinction both restated correctly and completely — no fix needed. His caveat is itself evidence the doc's summary of S2 is legible enough to stand in for the full debate record for a reader who only needs the thesis, which is exactly what `industry-framing.md` is for (evaluators read this file; they don't need to open `decisions/`). Loan closed at 2 of 2–4 turns used — no further turns needed for this track.

## Addendum — F2 was over-corrected, then reverted (consistency-pass re-run)

The original F2 fix (this track's response to `decisions/cross-track-consistency.md` F2) rewrote `api-connector-design-rationale.md:40,72` to hide the vendor token entirely inside a single internal `LookupIdentity` adapter call, on the reasoning that 02's `connector-security.md` forbade the token crossing the adapter boundary. That fix was wrong: the assignment fixes `/auth` as returning `access_token` to its own caller, so the vendor token has to cross our connector's *exposed* boundary at least once — it can't be hidden inside one internal call. The consistency-pass re-run caught the resulting contradiction between this track's F2 fix and Marcus's separately-supplied F3 resolution (`api-service` holds the token briefly between `/auth` and `/identity`, presenting it as a header on the second call; the connector itself never caches or persists it). Marcus's F3 model is correct and stands; this track's F2 over-correction has been reverted to match it, with his exact replacement sentences applied verbatim at `api-connector-design-rationale.md:40` and `:72`. Recorded here rather than silently re-edited, since the record should show the fix was wrong once, not just that it's right now.

---
Model: sonnet (Wesley Okonkwo, track 04's Newcomer hire, cold reader — both turns) / sonnet (Naomi Voss, lead, fixes applied) / sonnet (Marcus Ilori, 02 lead, F2/F3 reconciliation sentences). Turns consumed: 2 of the 2–4-turn loan (S3 primary read + fixes; S6 secondary read, no fix needed) + 1 direct two-party exchange with 02 for the F2/F3 reconciliation — matches the 1–2-turns-each budget in `PLAN.md` §3.
