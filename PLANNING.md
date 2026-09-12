# Planning Board

Thin cross-track coordination board — pointers and status only. The reasoning lives in each track's own files (`<track>/CLAUDE.md`, `<track>/PLAN.md`, deliverables), referenced by relative path. **One writer per row:** each lead edits only its own row; team-lead edits the header, the two Dana rows, and nothing else. Shape and rationale: `02-ai-security-architecture/planning-approach.md` §3. Org plan: `PLAN.md`. Hiring rules and registry: `CASTING.md`.

Status vocabulary: `not started` · `planning` · `plan drafted — awaiting hiring go` · `hiring go` · `hiring` · `debating` · `plan approved` · `blocked`.

| Track | Lead | Status | Blocking on | Blocked by | Planning notes | Hires (planned / registered / spawned) | Last updated |
|---|---|---|---|---|---|---|---|
| Orchestration (root) | Dana Whitfield | Phase 2 complete — all 18 hires live in five lead sessions; Phase 3 debates under way | 02 hand-offs to 03/04; 05 S1 contract; first `decisions/` artifacts | — | `PLAN.md`, `LOG.md` | 18 / 18 (cap) | 2026-09-12 |
| Cross-track consistency pass (root, Phase 3) | Dana Whitfield | not started | all five `plan approved` | — | `PLAN.md` Phase 3 | — | 2026-09-12 |
| 01 Product & Industry Research (incl. Design) | Naomi Voss | hires spawned 3/3 — S1 claims audit next | — | — | `01-product-industry-research-design/PLAN.md`; `decisions/` created; Newcomer loan from 04 (`wesley-okonkwo`) approved by team-lead for S3/S6 cold reads, ≤4 turns, timing Theo's call (requested) | 3 / 3 / 3 | 2026-09-12 |
| 02 AI Architecture & Security Architecture | Marcus Ilori | hires spawned 4/4 — ready for S3/S4 debates | — | — | `02-ai-security-architecture/PLAN.md`; `CASTING.md` §5 ratified with 4 amendments; `planning-approach.md` App. A + B; personas + agent defs written, registry rows appended | 4 / 4 / 4 | 2026-09-12 |
| 03 Engineering & Delivery | Renata Cole | hires spawned 4/4 — awaiting inputs | 05 contract (DAO), 02 auth hand-off (middleware) | — | `03-engineering-delivery/PLAN.md` | 4 / 4 / 4 | 2026-09-12 |
| 04 Infra & DevOps | Theo Bergman | hires spawned 3/3 — awaiting inputs | 03 service boundaries, 02 secrets inventory, 05 migration approach | — | `04-infra-devops/PLAN.md` | 3 / 3 / 3 | 2026-09-12 |
| 05 Data Ops | Priya Nandakumar | hires spawned 4/4 — S2/S3 debates next | — | — | `05-data-ops/PLAN.md`; 6 stories S1–S6, S1 = Go-shaped contract in `multi-db-strategy.md` (critical path); S4 `migration-approach.md` + S5 retention table unblock 04; factory mismatch closed with 03 direct (two-party): `dao.Repository` composite exposing `Profiles()`/`Credentials()`/`Methods()`, `dao.New` signature unchanged — cited in S1 | 4 / 4 / 4 | 2026-09-12 |

## Sessions (owned by team-lead)

Each lead is its own top-level Claude Code session in its own tmux window. Use the session name with `SendMessage` (confirm with `ListAgents`); hires are teammates of their lead's session and are reached through the lead.

| Role | Session name | tmux window | Model |
|---|---|---|---|
| PM / team-lead (Dana Whitfield) | `org-planning-loginid-takehome` | `1` | fable |
| 04 lead (Theo Bergman) | `04-infra-devops-47` | `04-infra` | sonnet |
| 02 lead (Marcus Ilori) | `02-ai-security-architecture-a2` | `02-security` | sonnet |
| 03 lead (Renata Cole) | `03-engineering-delivery-b7` | `03-engineering` | sonnet |
| 01 lead (Naomi Voss) | `01-product-industry-research-design-21` | `01-product` | sonnet |
| 05 lead (Priya Nandakumar) | `data-ops-planning-phase` (auto-renamed from `05-data-ops-a3`) | `05-data` | opus |

## Service boundaries (owned by 03, read by 04)

Set by 03 during Phase 1 planning (full reasoning: `03-engineering-delivery/PLAN.md` §4).

- **Two binaries**, not one: `cmd/api-service` (Q1 DAO + Q2 REST API, client-facing, bearer-token auth) and `cmd/idp-connector` (Q3, outbound-only to vendors) — different threat models/auth surfaces per 02, independent scaling needs, mirrors the assignment's own Q2/Q3 split.
- **Package layout:**
  ```
  cmd/api-service/main.go
  cmd/idp-connector/main.go
  internal/dao/        # Repository composite interface (Profiles/Credentials/Methods) + postgres/cockroachdb/sqlite implementations
  internal/model/      # shared structs: UserProfile, UserCredential
  internal/api/         # REST handlers, router, auth middleware
  internal/connector/   # /auth, /identity client + handlers
  internal/config/      # env-var config loading, shared by both binaries
  ```
- **Config surface:** environment variables only (12-factor: config lives in the environment, not in checked-in files) — `DB_DRIVER` (postgres|cockroachdb|sqlite), `DB_DSN`, `HTTP_ADDR`, `AUTH_JWT_ISSUER`/`AUTH_JWT_AUDIENCE` (names pending 02's final scheme), `IDP_ABC_BASE_URL`, `IDP_ABC_CLIENT_ID`/`IDP_ABC_CLIENT_SECRET`. No flags, no config files.
- **DAO driver-selection mechanism:** a factory `dao.New(driver, dsn string) (dao.Repository, error)` chosen at startup from `DB_DRIVER`. `dao.Repository` is a composite interface — `Profiles() ProfileRepository`, `Credentials() CredentialRepository`, `Methods() AuthMethodRepository` — matching 05's three-way split in `multi-db-strategy.md`, kept deliberately distinct (not one flat interface) because it's a load-bearing PII/credential-separation control per `05-data-ops/pii-governance.md`: a handler has to reach for the credential accessor on purpose. Each backend package returns one struct satisfying the composite. Factory signature and `DB_DRIVER` selection are unchanged; the three sub-interfaces' method signatures finalize once 05's contract lands.
- **Stable for 04 to build against:** the two-binary split, the `cmd/` entrypoint names, the env-var config approach and variable names above, and the `dao.New(driver, dsn)` factory signature. **Not yet stable:** internal DAO implementation details, ORM/query-builder choice, handler-package internals.

## Dependency order (from `planning-approach.md` §2)

1. First wave, in parallel: 01, 02, 05 — 05 is on the critical path (it hard-blocks 03's DAO and partially blocks 04).
2. 03 split start: scaffolding may begin immediately; DAO once 05's Go-shaped contract is stable; middleware once 02's receiver-shaped auth hand-off lands.
3. 04 last, gated on the three inputs named in its row.
