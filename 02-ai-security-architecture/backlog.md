# Backlog — Track 02: AI Architecture & Security Architecture

Lead: Marcus Ilori. Epic: **02 AI Architecture & Security Architecture** (Jira key: LT-2). Format per `../PLAN.md` Phase 4. All eleven stories from `PLAN.md` are listed in dependency order; ten are done (marked below) and shown as-run, not rewritten as if planned in hindsight — an honest backlog shows the work. S11 is the one open story, gated on Warren's implementation go-ahead.

---

## S1 — Ratify the orchestration-pattern catalog and add the attack-tree template (LT-21)

**Status: done.**

**Description:** Reviewed `../CASTING.md` §5's six orchestration patterns against this track's own planning reasoning and ratified them with four amendments (red/blue ranking discipline, adversarial-pair "what would change my mind" lines, structured-debate synthesis discipline, universal model/turn-count footers), then authored the attack-tree table template as Appendix A of `planning-approach.md`. Every later pattern run in this track (S3, S4, S5, S8) executed under this ratified catalog. See `PLAN.md` §0 for the full reasoning.

**Acceptance criteria:**
- [x] `../CASTING.md` §5 carries the four ⟨02⟩ amendments, changes scoped only between the `## 5.` and `## 6.` headings.
- [x] `planning-approach.md` Appendix A exists with the table shape used by every later red/blue run.
- [x] Every decision artifact produced after this story carries a model/turn-count footer (verified in `decisions/*.md`).

**Depends on:** —

**Model/effort:** sonnet, high (lead-authored, no hire turns).

**Type:** Story

**Labels:** `track-02`

---

## S2 — Harness-constraints addendum to `planning-approach.md` (LT-22)

**Status: done.**

**Description:** Checked `PLAN.md`'s "Verified mechanics" claims about model/effort tiers against `research-claude-architecture-best-practices.md` and recorded the harness facts that modify the org's model/effort plan (no per-agent effort key; hires don't spawn subagents; a hire's agent-definition file is part of its prompt-cache prefix). Written as Appendix B of `planning-approach.md` so every lead building a research fan-out or a hire roster works from the same corrected assumptions.

**Acceptance criteria:**
- [x] `planning-approach.md` Appendix B exists and states the three harness facts and their consequence for §1's tiers.
- [x] No contradiction found against the research doc (stated explicitly, not silently assumed).

**Depends on:** —

**Model/effort:** sonnet, high (lead-authored).

**Type:** Story

**Labels:** `track-02`

---

## S3 — Red/blue: connector token lifecycle (LT-23)

**Status: done.**

**Description:** Ran the ratified red/blue pattern (S1) against the IDP connector's vendor-token handling in `connector-security.md` §1: Tomasz Wrede's ranked top-five attack leaves, Felix Adebayo's alternatives turn (deleting two leaves by design change — no-cache-by-default rather than an encrypted, TTL-bound cache), Helena Marsh's blue response, Tomasz's rebuttal turn (closing a connection-reuse variant and tightening the accepted-replay-risk boundary). Full record: `decisions/connector-token-lifecycle-redblue.md`; attack-tree table appended to `connector-security.md`. This is the requirement set 03's connector implementation builds against directly.

**Acceptance criteria:**
- [x] The receiving track (03) can implement the connector's token handling without re-deriving this run's reasoning — confirmed by Renata Cole directly.
- [x] "No persistent token cache by default" — enforcement site: the connector's HTTP client construction, one fetch-per-`/identity`-call code path, no cache read/write call in the default path.
- [x] "Fresh, isolated `Authorization` header per outbound request" — enforcement site: the outbound HTTP client factory used for vendor calls, explicitly not sharing a request-context object across calls.
- [x] "Fetch → use → zeroize, retries re-fetch" — enforcement site: the token variable's scope inside the single vendor-call function; a retry path that calls `/auth` again rather than reusing a held token.
- [x] Residual risk (bearer-token replay, accepted) is named in the artifact, not left implicit.

**Depends on:** S1

**Model/effort:** opus, high for the connector's token-lifecycle implementation specifically (per `planning-approach.md` §1 — the one implementation surface warranting opus); sonnet, high for this track's design/debate turns.

**Type:** Story

**Labels:** `track-02`, `security-graded`

---

## S4 — Adversarial pair: search-API authorization scoping (LT-24)

**Status: done.**

**Description:** Ran the adversarial pair pattern against `api-auth-design.md`'s scope sketch: Helena Marsh (Proposer) drafted the final scope vocabulary, `profile:search` provisioning bar, object-level policy hook, and pagination/rate-limit/lifecycle rules; Ingrid Solano (Skeptic) raised five numbered objections, each with a "what would change my mind" line; Helena revised all five. Full record: `decisions/search-authz-scoping.md`; finalized design folded into `api-auth-design.md`. This is the scope model 03's middleware and object-level policy checks implement directly.

**Acceptance criteria:**
- [x] `profile:read:own` referral constraint — enforcement site: `authorize(sub, scope, target)` checking the `caller_referral` mapping, called in-handler before the DAO call (per Renata's confirmed placement).
- [x] `profile:read:any` requires a `reason_code` from a closed enum on every call — enforcement site: request validation inside the same `authorize()` call, rejecting a missing or out-of-enum value.
- [x] `profile:search` result-cap rejection (not truncation) — enforcement site: query-construction code in the search handler, checked before the DB call executes.
- [x] Cursor-based pagination, page ceiling 50 — enforcement site: pagination-parameter validation in the handler layer.
- [x] Per-client rate limits (60/10 req/min) and the cumulative distinct-record-touch counter, applying to `read:any` regardless of `reason_code` presence — enforcement site: rate-limiting middleware plus a counting store keyed per client credential, incremented per distinct record id returned.
- [x] Scope-grant lifecycle (90-day backstop, immediate revocation on offboarding/idle/function-change) — enforcement site: whatever identity-lifecycle signal 03/04 wire the grant-issuing component to consume.

**Depends on:** S1

**Model/effort:** sonnet, high (middleware and policy-check implementation); sonnet, high for this track's debate turns.

**Type:** Story

**Labels:** `track-02`, `security-graded`

---

## S5 — Structured written debate: `ai-workflow-narrative.md`'s core claim (LT-25)

**Status: done.**

**Description:** Debated whether this project's multi-agent, persona-driven workflow produces better design than a single strong session, or is cost without evidence. Felix Adebayo (Position A) argued from two concrete instances (S3's design-deleting alternatives turn, 04's Newcomer cold-read catches); Ingrid Solano (Position B) rebutted that n=2 from roles built to find exactly this isn't evidence of a general law, and that null results (a clean concession in S3) must be counted alongside hits. Ruled per point in `decisions/ai-workflow-claim-debate.md`: against a general empirical claim, for a narrower mechanism-level one. Result folded into `ai-workflow-narrative.md` (S9).

**Acceptance criteria:**
- [x] Both positions preserved in the record, not just the winning one.
- [x] The ruling states which position won on which point and why (per the `CASTING.md` §5 ⟨02⟩ amendment this story itself ratified in S1).
- [x] The narrative (S9) states the mechanism-level claim, not the broader statistical one Position B defeated.

**Depends on:** —

**Model/effort:** sonnet, high.

**Type:** Story

**Labels:** `track-02`

---

## S6 — Hand-off 02→03: auth scheme, validation point, authorization requirements (LT-26)

**Status: done** (`handoff-03-auth.md`, now v3).

**Description:** Synthesized S4's ruling and the token-shape decisions in `api-auth-design.md` into a receiver-shaped hand-off Renata Cole can build against without re-deriving this track's reasoning: token scheme, where it's validated, the final scope vocabulary and object-level policy hook, pagination/rate-limit numbers, the audit-log field list, and an explicit "not yours to decide" list. Revised to v2 after S8's independent review surfaced two implementation-relevant fixes (the decomposition counter's scope-coverage gap; the connector's own inbound-auth requirement), then to v3 after the cross-track consistency pass resolved six further findings (F3, F5, F6, F8, F10, F-pag, F52) that touched this hand-off. Accepted by 03 with a confirmed `authorize()` placement (in-handler, not middleware) and `DB_DSN_FILE` folded into the config surface.

**Acceptance criteria:**
- [x] Receiving track (03) can act on this without re-deriving the reasoning in `api-auth-design.md`/`connector-security.md` — confirmed by Renata directly, cold read "clean."
- [x] Token TTL (5–15 min), no refresh, no denylist — enforcement site: `exp` claim check in the verification middleware; absence of a refresh-grant or denylist-lookup code path.
- [x] Verifier pins expected algorithm, rejects `alg: none`/downgrade — enforcement site: explicit algorithm allow-list parameter on the JWT verification call.
- [x] Public-key-only verification in `api-service`; private key isolated to the issuing code path — enforcement site: Kubernetes RBAC Role (per `handoff-04-secrets.md` row #2) plus code-level separation between issuing and verifying functions.
- [x] `idp-connector`'s own `/auth`/`/identity` require authentication with a distinct audience/scope — enforcement site: the same JWT middleware pattern applied to `idp-connector`'s router, configured with `aud: idp-connector-service`.
- [x] Token-endpoint brute-force protection — enforcement site: rate limiter on the token-issuing endpoint, keyed by `client_id` + source IP.
- [x] Audit-log field list (`sub`, scope, record id, `reason_code`, policy decision, timestamp, outcome) and nothing beyond it — enforcement site: a single structured-logging call at the end of each search/retrieve handler, not scattered `log.Printf` calls a reviewer has to hunt for.

**Depends on:** S4

**Model/effort:** sonnet, high (lead synthesis, no new hire turns); sonnet, high for 03's implementation against it.

**Type:** Story

**Labels:** `track-02`, `security-graded`

---

## S7 — Hand-off 02→04: secrets inventory and never-log list (LT-27)

**Status: done** (`handoff-04-secrets.md`, now v2).

**Description:** Authored the secrets inventory and never-log list Theo Bergman's track needs to design secrets delivery against, then ran Ingrid Solano's single objection turn (S7's assigned pattern — one turn, not a full debate) and folded in all four accepted objections: the authorization-server's ownership was mis-scoped by omission, a KMS-access credential row was missing, the `DB_DSN` env-var exception was too weak, and audit-log read access needed its own RBAC line distinct from Secret access. Cold-read by 04's Newcomer (Wesley Okonkwo) caught two further gating gaps (TOTP/token-cache feature-status ambiguity; a non-self-contained trust-boundary reference), both fixed.

**Acceptance criteria:**
- [x] Receiving track (04) can act on this without re-deriving the reasoning in `threat-model.md`/`api-auth-design.md`/`connector-security.md` — confirmed by 04's Newcomer cold read, two gating items fixed before acceptance.
- [x] `DB_DSN_FILE` required as the primary delivery mechanism, `DB_DSN` a transitional fallback — enforcement site: the config-loading code in `internal/config`, preferring the file path when both are set (confirmed folded into 03's Service boundaries).
- [x] JWT signing private key file-mount only, RBAC-restricted to the issuing code path's service account — enforcement site: the Kubernetes Role scoping `get`/`list` on that Secret.
- [x] Client secrets stored hashed (Argon2id), never plaintext at rest — enforcement site: the provisioning code path that writes the hash, per `threat-model.md` Asset 1.
- [x] One `Secret` per vendor registration (ABC/XYZ never share a Secret) — enforcement site: the Kubernetes manifest's per-vendor Secret object.
- [x] Never-log list (vendor password, vendor token, PII values, raw vendor error bodies, signing key, JWTs, client secrets, `DB_DSN`, full `/auth`/`/identity` bodies, PII-bearing query params) — enforcement site: a log-formatting/redaction layer that structurally cannot emit these fields, not a code-review convention alone.
- [x] Audit-log read access is a distinct, narrower grant than Secret-read access — enforcement site: log-pipeline RBAC or a separate log-sink access control, 04's mechanism.

**Depends on:** one Skeptic objection turn (no debate pattern)

**Model/effort:** sonnet, high (lead synthesis + one hire turn).

**Type:** Story

**Labels:** `track-02`, `security-graded`

---

## S8 — Independent second review of the security-graded documents (LT-28)

**Status: done.**

**Description:** Ran an independent second-review pass on `threat-model.md`, `api-auth-design.md`, and `connector-security.md`, each reviewed by a hire who neither authored nor blue-teamed it. Required a mid-run reassignment: the original static roster assigned Helena Marsh to review two documents she'd since authored content in (S4's Proposer role, S3's blue seat) — caught and corrected before running, not after. Result: Helena reviewed `threat-model.md` (4 citation/precision fixes, 1 completeness note), Tomasz Wrede reviewed `api-auth-design.md` (4 findings, including a decomposition-counter gap that made a rate limit a silent no-op for `profile:read:any`), Ingrid Solano reviewed `connector-security.md` (2 findings — no auth requirement on inbound calls to the connector; no stated assumption about what a vendor token authorizes). All ten findings accepted, zero declined. Full record: `decisions/second-review-security-docs.md`.

**Acceptance criteria:**
- [x] Every reviewer is verified independent (did not author or blue-team the document) at the moment the review runs, not just at planning time.
- [x] Every accepted finding is fixed in the source document, not just recorded in the decision file.
- [x] The two findings affecting 03's implementation (decomposition-counter scope, connector inbound-auth requirement) are propagated into `handoff-03-auth.md` (v2 at the time this story ran; now v3) and relayed directly to 03.
- [x] Null results (documents/findings that didn't surface anything) are counted in the tally alongside hits, not silently omitted.

**Depends on:** S3, S4

**Model/effort:** sonnet, high.

**Type:** Story

**Labels:** `track-02`, `security-graded`

---

## S9 — Revise `ai-workflow-narrative.md` for the org layer (LT-29)

**Status: done.**

**Description:** Rewrote the AI-tooling narrative to cover what actually ran once hiring began: the persona/pattern-catalog layer this project built on top of Anthropic's documented orchestrator-worker pattern, honestly measured evidence from S3/S8 (including null results, not just hits), S5's ruling stated plainly with the losing position preserved, the S8 independence-reassignment lesson generalized ("independence is checked at review time, not assigned once"), and the real external operator agent named as deliberately out of scope and never contacted.

**Acceptance criteria:**
- [x] Distinguishes what's Anthropic-documented from what this project composed on its own (unchanged discipline from the original draft).
- [x] States the S5 ruling including the position that lost, not just the one that won.
- [x] Leads with the single most concrete example (Tomasz's `profile:read:any` decomposition-counter finding) rather than an abstract summary.
- [x] Names the external operator agent and states plainly it was never contacted.

**Depends on:** S3, S4, S5, S8

**Model/effort:** sonnet, high.

**Type:** Story

**Labels:** `track-02`

---

## S10 — `threat-model.md` maintenance (LT-30)

**Status: done — verified already satisfied, no edit required.**

**Description:** Two required checks from the original plan: (a) repoint the stale reference to a wiped `.sql` file at the prose schema in `../05-data-ops/multi-db-strategy.md`; (b) add a trust-boundary line naming the external deployment operator as a privileged actor. Both were already present in `threat-model.md` from earlier work (verified by `grep` for any remaining `.sql` reference, and by re-reading Assumption 6) — recorded here rather than silently skipped, since an honest backlog notes verification, not just edits.

**Acceptance criteria:**
- [x] No reference to a wiped `.sql` file remains anywhere in `threat-model.md` (verified by grep).
- [x] The operator trust boundary is named explicitly in Assumption 6, with the consequence for `handoff-04-secrets.md` stated.

**Depends on:** —

**Model/effort:** sonnet, high (verification only, no hire turns).

**Type:** Task

**Labels:** `track-02`

---

## S11 — Post-implementation independent review of 03's auth middleware and connector token code (LT-31)

**Status: open — gated on Warren's implementation go-ahead.**

**Description:** Once 03 implements the auth middleware (S6) and the connector's token-lifecycle code (S3), this track runs an independent code-level review of the security-graded surfaces only: token validation, the `authorize()` object-level policy hook, rate-limiting/decomposition-counter enforcement, and the connector's fetch-use-zeroize discipline. Reviewer assignment will be checked for independence at run time per the S8 lesson (whoever reviews must not have authored the code or the design doc it implements) rather than assumed from the current roster, since roles may have accumulated further authorship by then.

**Acceptance criteria:**
- [ ] Reviewer independence verified at run time, not assumed from this backlog's authorship snapshot.
- [ ] Every enforcement site named in S3/S4/S6/S7's acceptance criteria above is checked against the actual implementation, not just the design doc.
- [ ] Findings recorded in `decisions/second-review-implementation.md`, following the same accept/decline-with-reason discipline as S8.
- [ ] Any Residual **H** finding (per the S1 attack-tree convention) produces a follow-up story or a stated accepted risk, not a silent gap.

**Depends on:** Warren's code go-ahead; 03's implemented auth middleware and connector token-lifecycle stories.

**Model/effort:** sonnet, high.

**Type:** Story

**Labels:** `track-02`, `security-graded`, `no-code-yet`

---

## Room for pass-driven stories

Any story arising from the cross-track consistency pass that touches this track's surfaces (auth scheme, secrets inventory, connector security, threat model) will be added below this line with the `from-consistency-pass` label, in the same format as above, once the pass reports.

---

## S12 — Split the token issuer into its own Deployment with isolated signing-key custody (LT-32)

**Status: in review** (from `../decisions/cross-track-consistency.md` F13; PR #18 open, Helena Marsh's independent review posted — approve).

**Description:** The pass caught a real contradiction: `handoff-04-secrets.md` said the JWT signing private key "never touches `api-service`" while also defining the issuer as "a mode of `api-service`," and 04's manifest resolved that by mounting the signing key into every replica of a single `api-service` Deployment — RBAC on a Secret's `get` verb does nothing to stop a volume already mounted into a running container. Fixed at the design level in `handoff-04-secrets.md` Assumption 2 and row 2: the issuer runs as a second Deployment of the same `api-service` image, selected by an environment variable (`APP_MODE=issuer` vs. default), with its own ServiceAccount that alone mounts the signing-key Secret. This story is the implementation and manifest work that design change requires — it did not exist as a story before the pass.

**Acceptance criteria:**
- [ ] `api-service` supports an `APP_MODE` environment variable selecting issuer vs. verifying behavior from one binary/image.
- [x] Two Kubernetes Deployments exist, each with its own ServiceAccount; only the issuer Deployment's pod spec mounts the JWT signing-key Secret. **Corrected enforcement sites (Theo Bergman caught that an SA-scoped RBAC Role on the Secret is inert — a volume mount is kubelet-resolved at admission, not gated by the mounting pod's own ServiceAccount RBAC):** (a) the verifying Deployment's pod spec physically contains no reference to the Secret, confirmed by dumping the manifest whole; (b) the CI deployer's Role has no `secrets` verbs at all; (c) the Secret is created out-of-band, never through the CI-managed manifest set. No RBAC Role naming this Secret is required on either Deployment's ServiceAccount.
- [x] The verifying Deployment's pod spec contains no reference to the signing-key Secret at all (not just RBAC-denied — physically absent from its mounts). Verified by Helena's PR review (reading the manifest/config, not just the claim).
- [x] **Accepted residual risk, named explicitly (found by Naomi Voss/Desmond Okafor during LT-32 refinement, mitigation proposed by Theo Bergman):** the CI deployer's Role grants `create`/`update`/`patch` on `deployments` (required for the pipeline to function at all), and a Deployment volume-mount reference doesn't require the applying identity to hold RBAC on the referenced Secret — so a manifest PR could add a `volumeMount` for the signing-key Secret directly into the verifying Deployment, and the CI runner's existing deploy-write access would be sufficient to apply it, bypassing the Secrets-RBAC lockdown entirely without ever touching the Secrets API. No admission controller (Gatekeeper/Kyverno/OPA) exists on this cluster, and standing one up is disproportionate for this take-home. **Accepted, not a gap left silent, because:** (1) this is a structural property of Kubernetes RBAC without a policy engine — it applies to every Secret in the namespace, not uniquely to this one, and is a known, widely-accepted limitation of RBAC-only clusters; (2) the CI deploy identity is already this track's named privileged trust boundary (`threat-model.md` Assumption 7) — this is a concrete instance of a boundary already named, not a new one; (3) a compensating control exists: a CI check greps the manifest set for the signing-key Secret's name and fails the build if it appears outside the issuer Deployment's own block — a detective control that raises the bar (catches an accidental or careless PR before merge) without claiming to be a structural guarantee (a deliberately obfuscated reference could still evade a grep). This is stated as an accepted risk with a named, if imperfect, mitigation — the honest posture for a take-home, not a claim of a stronger guarantee than what's actually in place.
- [ ] `containerization-design.md`'s mount at the lines the pass named is removed and replaced with the two-Deployment shape.

**Depends on:** `handoff-04-secrets.md` v2 (this track); 03's `APP_MODE` config surface addition; 04's manifest change.

**Model/effort:** sonnet, high (03 implementation); sonnet, high (04 manifest).

**Type:** Story

**Labels:** `track-02`, `security-graded`, `from-consistency-pass`, `no-code-yet`

---

## S13 — Wire `api-service`'s client credential to call the connector, and the vendor-token header pass-through (LT-33)

**Status: open** (from `../decisions/cross-track-consistency.md` F3 and F15).

**Description:** Two related gaps the pass found: (1) nothing said what our own `/auth` returns to its caller or how `/identity` obtains vendor authority, given the assignment's fixed `/identity` body has no credential field (F3) — resolved in `connector-security.md` §1: `/auth` returns the vendor token to the authenticated internal caller, which presents it as a request header (not body field) on the subsequent `/identity` call; the connector itself still never caches it. (2) `api-service` needs its own client credential to authenticate to the connector's now-required inbound auth (`connector-security.md` §5) in the first place, and no inventory row covered it — added as row 10 in `handoff-04-secrets.md` (F15). This story is the implementation work both design fixes require.

**Acceptance criteria:**
- [ ] `api-service` obtains and presents a `connector:identity-lookup`-scoped token (via its own client credential, row 10 of `handoff-04-secrets.md`) when calling `idp-connector`'s `/auth` and `/identity`.
- [ ] The onboarding flow's vendor-auth step receives the vendor `access_token` from our `/auth` response and holds it only for the span between that call and the following `/identity` call — enforcement site: the handler/service layer that mediates the onboarding flow, not the connector itself (which remains stateless per S3).
- [ ] `/identity` accepts the vendor token as a request header, not a body field, and fails closed with a generic error if it is missing or rejected by the vendor.
- [ ] `01-product-industry-research-design/ux-notes.md`'s two-screen flow and this mechanism agree on what the frontend/onboarding-flow layer actually holds between screens (coordinate directly with Naomi if the UX description needs a one-sentence update to match).

**Depends on:** `connector-security.md` (this track); `handoff-04-secrets.md` row 10 (this track); 03's connector-client implementation.

**Model/effort:** sonnet, high.

**Type:** Story

**Labels:** `track-02`, `security-graded`, `from-consistency-pass`, `no-code-yet`
