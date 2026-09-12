# Planning Board

Thin cross-track coordination board — pointers and status only. The reasoning lives in each track's own files (`<track>/CLAUDE.md`, `<track>/PLAN.md`, deliverables), referenced by relative path. **One writer per row:** each lead edits only its own row; team-lead edits the header, the two Dana rows, and nothing else. Shape and rationale: `02-ai-security-architecture/planning-approach.md` §3. Org plan: `PLAN.md`. Hiring rules and registry: `CASTING.md`.

Status vocabulary: `not started` · `planning` · `plan drafted — awaiting hiring go` · `hiring go` · `hiring` · `debating` · `plan approved` · `blocked`.

| Track | Lead | Status | Blocking on | Blocked by | Planning notes | Hires (planned / registered / spawned) | Last updated |
|---|---|---|---|---|---|---|---|
| Orchestration (root) | Dana Whitfield | planning — Phase 0 scaffolding complete | — | — | `PLAN.md`, `LOG.md` | — | 2026-09-12 |
| Cross-track consistency pass (root, Phase 3) | Dana Whitfield | not started | all five `plan approved` | — | `PLAN.md` Phase 3 | — | 2026-09-12 |
| 01 Product & Industry Research (incl. Design) | Naomi Voss | not started | — | — | `01-product-industry-research-design/PLAN.md` | 3 / 0 / 0 | 2026-09-12 |
| 02 AI Architecture & Security Architecture | Marcus Ilori | not started | — | — | `02-ai-security-architecture/PLAN.md` | 4 / 0 / 0 | 2026-09-12 |
| 03 Engineering & Delivery | Renata Cole | not started | 05 contract (DAO), 02 auth hand-off (middleware) | — | `03-engineering-delivery/PLAN.md` | 4 / 0 / 0 | 2026-09-12 |
| 04 Infra & DevOps | Theo Bergman | not started | 03 service boundaries, 02 secrets inventory, 05 migration approach | — | `04-infra-devops/PLAN.md` | 3 / 0 / 0 | 2026-09-12 |
| 05 Data Ops | Priya Nandakumar | not started | — | — | `05-data-ops/PLAN.md` | 4 / 0 / 0 | 2026-09-12 |

## Service boundaries (owned by 03, read by 04)

TBD — set by 03 during planning. Must state: one binary or two (API service vs. IDP connector), Go module/package layout, configuration surface (env vars / flags), DAO driver-selection mechanism (how Postgres/CockroachDB vs. SQLite is chosen at startup), and which of these 04 may treat as stable.

## Dependency order (from `planning-approach.md` §2)

1. First wave, in parallel: 01, 02, 05 — 05 is on the critical path (it hard-blocks 03's DAO and partially blocks 04).
2. 03 split start: scaffolding may begin immediately; DAO once 05's Go-shaped contract is stable; middleware once 02's receiver-shaped auth hand-off lands.
3. 04 last, gated on the three inputs named in its row.
