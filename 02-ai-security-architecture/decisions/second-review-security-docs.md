# Decision: Independent Second Review of Security-Graded Docs (S8)

Pattern: second review, independence assigned per-document by authorship history rather than the original static roster in `PLAN.md` — see reassignment note below. Each reviewer covers a document they neither authored nor blue-teamed during Phase 2 execution.

## Reassignment note

`PLAN.md`'s original S8 plan assigned Helena Marsh to review both `threat-model.md` and `api-auth-design.md`, and Ingrid Solano to review `connector-security.md`, on the assumption that neither had touched `api-auth-design.md`. That assumption broke during execution: Helena became S4's Proposer, authoring the finalized scope vocabulary and policy content in `api-auth-design.md`, and was also the blue seat in S3, authoring the controls in `connector-security.md`. Reviewing either would be an author reviewing their own work, not a second reviewer. Reassigned: **Helena → `threat-model.md` only** (never authored or blue-teamed); **Tomasz Wrede → `api-auth-design.md`** (no involvement in Phase 2 — his S3 work was the connector, not the search API); **Ingrid → `connector-security.md`** (unchanged — she never touched it).

## Review: `threat-model.md` (Helena Marsh)

Four citation/precision fixes, one completeness note, all accepted:

1. NIST SP 800-63B was invoked by name for recommending Argon2id specifically, but 800-63B Rev. 3 (current) requires only "a suitable memory-hard KDF" without naming Argon2id — that specific naming is in the draft SP 800-63-4, not yet final. Fixed: claim softened to cite what 800-63B Rev. 3 actually requires, Argon2id credited to OWASP alone.
2. WebAuthn counter/credential-ID integrity was asserted with no citation. Fixed: W3C WebAuthn Level 2 §6.1.1 added.
3. The TLS 1.2+ floor was asserted with no citation. Fixed: NIST SP 800-52 Rev. 2 added.
4. Credential-stuffing mitigation presented account lockout as a peer option alongside rate limiting/backoff; NIST SP 800-63B §5.2.2 actively disfavors lockout as a self-inflicted denial-of-service vector. Fixed: lockout language removed, throttling/backoff/step-up stated as the preference with the citation.
5. **Completeness, not a defect:** the three-asset scope silently omitted this API's own issued JWTs. Fixed: one line stating that boundary explicitly as `api-auth-design.md`'s asset, not re-modeled here.

## Review: `api-auth-design.md` (Tomasz Wrede)

Four findings, ranked, all accepted:

1. **High.** The cumulative distinct-record-touch counter (S4's fix for decomposition-style enumeration) was worded to exempt any call carrying a `reason_code`. `profile:read:any`'s policy hook already requires a `reason_code` on every call, so the exemption was unconditionally true for that scope — the counter that closed the decomposition gap for `profile:search` was a no-op for `profile:read:any`, the scope with the broadest blast radius (no referral constraint). Nobody in S4's adversarial pair was looking at this angle, since the counter itself was Ingrid's fix to a different objection (about `profile:search`). Fixed: the counter now applies to `read:any` regardless of `reason_code` presence.
2. **Medium.** No stated defense against JWT algorithm confusion at the verifier (`alg: none`, RS256→HS256 downgrade). Fixed: verifier must pin the expected algorithm and reject any other, cited to RFC 8725 §3.1.
3. **Medium-high.** No brute-force/lockout protection on the client-credentials token endpoint itself — every rate limit in the design protects the resource API after a token issues, none protect the endpoint where a caller obtains one. Fixed: per-`client_id`/per-IP failed-grant rate limiting and alerting added.
4. **Minor, consistency.** This API's own bearer-token replay has the identical accepted-risk profile as the connector's vendor token (S3), but unlike the connector, it was never named as such — silently assumed safe rather than stated. Fixed: named explicitly as an accepted risk, with the same reasoning (TTL bounds exposure, no PoP lever expected or required).

## Review: `connector-security.md` (Ingrid Solano)

Two findings, both accepted, both outside the frame of the three S3 participants (who were entirely focused on custody of the vendor token, not on who may call the connector or what a vendor token authorizes):

1. **No authentication requirement on calls into the connector's own `/auth`/`/identity`.** §1–§4 protected the outbound vendor call exclusively; nothing stated who is allowed to reach the connector's own endpoints, which meant the rate-limiting control in §4 had no caller identity to bind "per calling-API-client" to. Fixed: new §5 requires the same OAuth2/JWT mechanism as the search API, with a distinct audience (`idp-connector-service`) and scope (`connector:identity-lookup`), restricted to internal service callers.
2. **Whether a vendor token authorizes a query for an arbitrary identity, or only the token-holder's own, was unaddressed.** The design's entire token-handling discipline (§1) governs custody, not authority. Fixed: new §6 states plainly, as an assumption rather than a resolved control, that this design assumes the vendor binds `/identity` queries to the authenticated principal — and names the consequence if a real vendor doesn't.

## Downstream propagation

Both `api-auth-design.md` fixes affecting implementation (finding 1's counter scope, findings 2–4) and `connector-security.md`'s new §5 (connector auth requirement) were folded into `handoff-03-auth.md` (now v2) and relayed directly to Renata Cole (03), who confirmed she will build the connector's handler auth to the new requirement from the start rather than retrofitting.

## Why this run mattered (for `ai-workflow-narrative.md`, S9)

Every one of these findings survived because the reviewer had zero authorial investment in the document's existing frame. The `api-auth-design.md` finding 1 is the clearest case: the counter's flaw was invisible to Helena (who wrote it) and Ingrid (whose objection it was answering) precisely because both were reasoning inside "does this satisfy the objection as stated," not "does this control actually bind for every scope it claims to cover." Tomasz, arriving with no stake in either the original proposal or its revision, checked the mechanism against all three scopes independently and found the gap in under one review pass.

**Tally, honestly counted (per S5's ruling that null results count too; corrected by the cross-track consistency pass, F40 — "4+4+2=10" undercounted `threat-model.md`'s own list of five items):** 11 items across three documents — 4 defects + 1 completeness note in `threat-model.md`, 4 defects in `api-auth-design.md`, 2 defects in `connector-security.md` — 10 defects plus 1 completeness note, zero declined, zero null reviews this time: every second-review seat found at least one real, accepted item. This is a different (better) hit rate than S3's red/blue, where one of three top-ranked leaves (leaf 2 in S3) was conceded clean with no new finding. Both data points belong in S9's tally, not just the hits.

---
Model: sonnet (Helena Marsh, Tomasz Wrede, Ingrid Solano — reviews) / sonnet (Marcus Ilori, lead — reassignment ruling, fixes applied). Turns consumed: 3 (one review turn per document; no rebuttal round needed since all findings were accepted outright) — matches the 2-turn budget in `PLAN.md` §3, one turn over due to the third document added by the reassignment.
