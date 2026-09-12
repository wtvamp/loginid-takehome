# Newcomer's-question pass: 03 service boundaries + 02 secrets hand-off

Pattern: Newcomer's question (`CASTING.md` §5). Reader: Wesley Okonkwo, cold read, 1 turn.

## Targets

1. `../PLANNING.md` — Service boundaries section (owned by 03/Renata Cole).
2. `../02-ai-security-architecture/handoff-04-secrets.md` v1 (owned by 02/Marcus Ilori).

## Findings (verbatim list, Wesley's turn)

**Service boundaries (`PLANNING.md`):**
1. `Repository` interface method set marked "not yet stable" — Wesley can design against the factory signature only, not the interface shape. Not a defect, just confirms 04 can't start DAO-adjacent work yet.
2. "12-factor" used undefined.

**`handoff-04-secrets.md`:**
3. Row 2/7 — "envelope encryption", KEK vs. DEK relationship; DEK itself never defined (only "re-wrapping DEKs" in the rotation column).
4. Row 2 — `kid` claim undefined.
5. Row 2/3 — RS256/ES256, JWKS assumed familiar.
6. Row 4 — Argon2id, OAuth2 client-credentials grant assumed familiar.
7. Row 7 — "if either feature exists" reads as TOTP support being undecided, not confirmed. Wesley can't tell if he's building around a maybe or a yes.
8. Row 7a — "workload-identity-based" assumed K8s/cloud vocabulary undefined in-document.
9. Assumption 3 — "external operator agent" never named in the hand-off itself; only resolvable from context outside the document (04's own `PLAN.md` names it as `amber-kubernetes`).
10. Kubernetes-specific requirements, last bullet — "incident-review role" named but who/what holds it or how it's provisioned is unstated.

## Disposition

Items 1-2 belong to 03's document; relayed to Renata (03) to gloss or leave as-is at her discretion — informational, not blocking 04.

Items 3-10 belong to 02's document; relayed to Marcus (02) since the Newcomer's-question pattern has the author fix, not the receiver. Item 9 is the one substantive flag: the hand-off's own trust-boundary language (assumption 3) should name the operator actor rather than relying on a cross-track reference to 04's `PLAN.md` — a hand-off shouldn't require reading a second track's file to parse its own assumptions. Item 7 (TOTP: maybe vs. yes) is the second-most substantive — 04 needs to know whether to design KEK delivery around a real feature or a hypothetical before finalizing the secrets-delivery deliverable.

Everything else (jargon glosses: KEK/DEK, kid, RS256/ES256, JWKS, Argon2id, client-credentials, workload identity) is cosmetic — the underlying requirement is clear to Theo and doesn't block drafting; flagged to Marcus as optional polish, not gating.

No fix applied in this track — hand-off content isn't 04's to edit.

**Resolved by Marcus (02), v2 of `handoff-04-secrets.md`:**
- Item 7 (TOTP): neither TOTP nor connector token-caching is a committed feature. TOTP is a supported *value* in 05's `user_credential.method` lookup, not a planned implementation. Token-caching is under live debate in 02's S3 (connector token-lifecycle red/blue); current direction (Felix's alternatives turn) is "no cache by default." Row 7 is now an explicit named contingency, not a live requirement — 04 provisions it only if either feature materializes. 02 will follow up once S3 lands.
- Item 9 (operator naming): assumption 3 now names the operator inline in the hand-off itself rather than requiring a read of 04's `PLAN.md`.
- Items 3-6, 8 (jargon glosses): left as-is, deliberately — those terms are already defined/sourced in `api-auth-design.md`/`threat-model.md`, which the hand-off already points to; re-glossing here would duplicate content per the project's own no-duplication rule. Accepted by 04 — not gating.

03's items (1-2) also resolved: "12-factor" glossed in `PLANNING.md`; the Repository-interface note was intentional (genuinely unstable, blocked on 05) and needed no fix.

04 proceeds on `handoff-04-secrets.md` v2, still gated on 05's migration approach before drafting any of the five deliverables.

---
Model: sonnet (Theo, ruling) / sonnet (Wesley Okonkwo, cold read). Turns consumed: 1.
