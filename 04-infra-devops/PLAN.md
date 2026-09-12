# Track 04 (Infra & DevOps) — Phase 1 Planning Plan

## Context

This is the Phase 1 planning pass for track 04, per team-lead's brief and the org-wide `PLAN.md` / `PLANNING.md` / `CASTING.md` / `02-ai-security-architecture/planning-approach.md` scaffolding. No code or config artifacts yet — this file is a plan only. Track 04 is deliberately last in the dependency order: it designs how the engineering track's services get built, deployed, and operated, which only makes sense once those services have a shape. Nothing below gets executed until team-lead replies "hiring go" (for the roster) and Warren gives the separate implementation go-ahead (for any code/config).

## Hiring status (post "hiring go")

Wesley Okonkwo, Bree Sandoval, and Callum Ferreira are live, standing named teammates, spawned directly by Theo in this session — see `CASTING.md` §6 step 5.

## Deployment target note (added post-approval, Warren's direct input)

There is a real Kubernetes dev/lab cluster on Warren's LAN, operated day-to-day by an OpenClaw agent ("amber-kubernetes") running on a Mac mini reachable via SSH — this is not a hypothetical target, it's a specific, addressable cluster with an operator who already knows its conventions. Warren's explicit call: this stays a **design-only reference** for the submission. The deliverables below should name this cluster as the concrete deployment target (so the design reads as intentional rather than generic "some Kubernetes cluster somewhere"), including describing amber-kubernetes' role as the operational actor who'd apply manifests and know the cluster's existing namespace/ingress conventions — but nothing gets actually deployed, and amber-kubernetes is not contacted, during this take-home. If a real demo deployment is ever wanted, that is a separate, later decision Warren would make explicitly — not implied by this note.

## 1. Deliverables & tasks (one Jira Story each; Epic = this track)

1. **Containerization design** — Dockerfile sketch/description for both the DAO-backed API service and the IDP connector service (multi-stage Go builds), described as targeting the real k8s dev cluster above (so it should read as a Deployment/Service manifest sketch, not just a bare Dockerfile). *Gated on:* 03's service boundaries (one binary or two, package layout) in `PLANNING.md`.
2. **CI pipeline sketch** — what runs on commit/PR: build, lint, test, security scan, and (as a documented final stage) a describe-only "apply to the lab cluster via amber-kubernetes" step — named, not executed. *Gated on:* nothing structural from other tracks, but the "test" stage assumes 03's test strategy exists in outline.
3. **Local dev-loop** — docker-compose sketch with a local Postgres profile and a SQLite-only profile (compose profiles, matching whatever env-var/config mechanism 03 picks for DB driver selection); this is the local counterpart to the cluster target, not a replacement for it. *Gated on:* 03's DAO driver-selection mechanism.
4. **Secrets delivery mechanics** — how DB credentials, IDP credentials, and signing keys reach the running services at deploy time (env injection, mounted files, or a secrets-manager sketch — mechanism only, not what's a secret or why), noting how that would map onto the lab cluster's existing secrets convention once known (not yet queried — see deployment target note). *Gated on:* 02's secrets inventory.
5. **Observability + never-log enforcement** — logging/metrics/tracing approach for both services, plus how the never-log list gets enforced in practice (structured logging with a redaction/allowlist discipline, not just a policy statement). *Gated on:* 02's never-log list.

Named hand-off inputs, all as files this track reads (not re-derives):
- `../PLANNING.md` → "Service boundaries" section, owned by Renata (03).
- 02's receiver-shaped secrets inventory + never-log list (per `planning-approach.md` §2, a deferred 02→04 hand-off — file path TBD, likely an addition to `../02-ai-security-architecture/connector-security.md` or a new short file).
- `../05-data-ops/multi-db-strategy.md` → migration tool/approach only (not migration content).

## 2. Hire roster (3 hires, per Warren's 3–4/track sizing and Theo's small-surface argument for the low end of that range)

All three functions are genuinely useful here — not padding — because each does something Theo's own temperament won't: a Newcomer catches "assumed" jargon in hand-offs Theo would read past too fast; an Enthusiast is the required temperament opposite and the one voice arguing against Theo's own bias toward the minimum viable setup; a second Minimalist voice acts as an independent scope check on Theo's own designs (deadpan-minimalist reviewing deadpan-minimalist has no counterweight without a distinct persona doing it). No fourth hire — there's no fourth function this track's small deliverable set actually needs; adding one would be exactly the over-resourcing Theo argued against in `planning-approach.md`.

### Hire 1 — Newcomer (function: hand-off legibility check)
- **Proposed name:** Wesley Okonkwo
- **Archetype:** Newcomer · temperament tags: `serious, cautious`-leaning but genuinely curious (bright/earnest, not deadpan)
- **Model:** sonnet
- **Patterns & role:** runs the Newcomer's-question pass (§5 of `CASTING.md`) on 03's service-boundaries hand-off and 02's secrets-inventory hand-off once both land — reads each cold, lists anything he couldn't act on without asking Theo to re-explain it, Theo fixes or annotates. This is the one pattern this track's plan authorizes.
- **Persona (draft, 137 words):** Bright, unguarded, asks the question everyone else stopped asking two meetings ago. Decides by restating what he thinks he just read and waiting to be corrected — if nobody corrects him, he assumes it's actually clear and says so in writing. Two years as a support engineer triaging on-call tickets for a mid-size SaaS company, one year as a junior platform engineer at a logistics startup, where he learned that a runbook nobody outside its author can follow is not a runbook. Comfortable with Docker, basic CI YAML, and reading a config file, but has never designed one from scratch. Pushes back on any hand-off that uses a term without defining it once. Blind spot: doesn't know what's already settled, so he sometimes flags a deliberate simplification as a gap. Disagreement: asks a follow-up question rather than asserting he's right.

### Hire 2 — Enthusiast (function: Theo's required temperament opposite; argues for trying the promising infra option)
- **Proposed name:** Bree Sandoval
- **Archetype:** Enthusiast · temperament tags: `playful, bold`
- **Model:** sonnet
- **Patterns & role:** informal counterweight seat (not a named catalog pattern — the catalog itself says this track is too small for red/blue or three-hats) inside the containerization, CI, and dev-loop design tasks: for each, she writes the "what's the more capable/interesting option and what would it cost us" case (e.g., a managed secrets manager over a mounted file, a fuller CI matrix over the minimum) before Theo rules on it. Documented as a short "considered and declined/adopted" note per deliverable, not a separate debate artifact — proportionate to a three-task track.
- **Persona (draft, 128 words):** Bold, quick to say "let's just try it," genuinely energized by the newer option in the room. Decides fast and revises fast — she'd rather propose three things and cut two than deliberate over one. Three years building CI/CD for a Series B fintech's platform team, one year as a DevRel engineer demoing infra tools, which is where the optimism about new tooling comes from and also where she watched three demos fail under real traffic. Deep hands-on with GitHub Actions, container registries, and secrets-manager integrations. Pushes back whenever a design defaults to "the minimum" without pricing what's given up. Blind spot: discounts the operational cost of the thing she's excited about. Disagreement: stays cheerful, concedes fast when shown the failure mode, rarely re-litigates after that.

### Hire 3 — Minimalist (function: independent scope guard)
- **Proposed name:** Callum Ferreira
- **Archetype:** Minimalist · temperament tags: `serious, cautious`
- **Model:** sonnet
- **Patterns & role:** informal review seat — reads each of the five deliverables once drafted and asks "what would we cut," specifically watching for take-home scope creep this track is prone to (Kubernetes manifests, a service mesh, a full observability stack) that reads as impressive but isn't what a take-home needs. One short note per deliverable, folded into the deliverable file rather than a separate debate artifact.
- **Persona (draft, 118 words):** Plain-spoken and unimpressed by scale for its own sake, but not identical to Theo — where Theo sizes to the actual problem from experience, Callum's method is adversarial subtraction: he assumes everything in a draft is optional until proven otherwise. Four years as an SRE at a logistics company that got burned by an over-built Kubernetes migration for a service with ten users, two years freelancing infra audits for seed-stage startups telling them what to delete. Comfortable with Docker Compose, basic CI, and reading a Terraform diff for what it actually provisions. Pushes back on anything justified by "best practice" rather than a stated failure mode. Blind spot: has, in his own account, deleted something load-bearing before. Disagreement: states the cut plainly, doesn't fight if overruled.

## 3. Debate plan

- **One Newcomer's-question pass** (Wesley) on 03's service-boundaries entry in `PLANNING.md` and on 02's secrets-inventory/never-log hand-off, run once each lands. This is the pattern `CASTING.md` §5 explicitly scopes to this track, and it's the highest-ROI pattern in the whole project per Marcus's synthesis — it directly tests whether those two hand-offs actually meet the "receiver can act without re-deriving" standard this track depends on.
- **Everything else is waste, explicitly:** no adversarial pair, no red/blue, no three hats, no structured written debate, no rotating devil's advocate seat. This track has no security-property decision (that's 02's), no multi-DB abstraction choice (that's 05's), and no authz scoping (that's 03/02's) — the catalog itself names this track's decisions as exactly the kind not worth debating (endpoint shapes, a Dockerfile, a compose file). Bree and Callum's roles above are lightweight per-deliverable notes, not formal debate artifacts, because formalizing them into `decisions/*.md` files for a Dockerfile sketch would be process theater on a three-task track.

## 4. Dependencies

Stable enough to design against means, for each of the three gating inputs:

- **03 service boundaries** (`../PLANNING.md`, Service boundaries section): stable once Renata states — one binary or two, Go package layout, the config surface (env vars vs. flags vs. file), and the DAO driver-selection mechanism — and marks it as decided rather than a placeholder. It does not need to be implemented in code yet, only unlikely to be rewritten; per `planning-approach.md`, "stable" means that, not "polished."
- **02 secrets inventory + never-log list**: stable once it exists as an enumerated list (which secrets, roughly what each protects) plus an explicit never-log list, in a named file this track can cite — not as prose alternatives Theo would have to interpret. Marcus's `planning-approach.md` already commits this as a deferred 02→04 follow-up once the planning pause lifts.
- **05 migration approach** (`../05-data-ops/multi-db-strategy.md`): stable once it names the migration tool/mechanism (e.g., a specific Go migration library, embedded SQL files, or a codegen approach) — this track only needs the name and invocation shape to script a pipeline step around it, never the migration content itself, which stays 05's.

## 5. Status (updated post-drafting)

All three gating inputs landed (03 boundaries stable, 02 secrets hand-off v2, 05 migration approach provisional-but-stable-for-04-per-05's-own-note) and Warren gave the go to start against them once the usage reset. All five deliverables drafted: `./containerization-design.md`, `./ci-pipeline.md`, `./local-dev-loop.md`, `./secrets-delivery.md`, `./observability.md`. Bree's capability-case notes (NetworkPolicy adopted, CockroachDB CI job adopted, hot-reload adopted; HPA/service-mesh declined) and Callum's scope-cut notes (isolation-paragraph trim, anti-affinity dropped, KMS row folded, tracing cut to one line) are folded directly into the files, not a separate decisions artifact, per PLAN.md §3's proportionality call. `newcomer-coldread-boundaries-secrets.md` in `./decisions/` covers Wesley's earlier hand-off legibility pass. Wesley also ran a bounded two-turn loan for 01 (Naomi), closed clean.
