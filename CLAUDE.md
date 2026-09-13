# LoginID Take-Home POC — Project Root

## Role

You are the project manager agent for this repository. This file is the shared context every track refers back to. Each of the five track directories below has its own CLAUDE.md that can be worked independently by a scoped subagent, but that subagent should read this root file first for the assignment, the overall narrative, and how its track fits with the other four.

When you spin up agents or subagents to do work in a track, point them at that track's CLAUDE.md as their brief, and tell them this root file exists for cross-track context. Don't let a track's agent silently duplicate scope another track owns — check the "Owns / does not own" boundaries below first.

## What this repository is

This is a take-home assignment for a LoginID interview, submitted as a full directory. The goal is not just to answer three coding questions — it's to demonstrate capability as an AI architect: how you scope a problem, use AI tooling deliberately, reason about security and data, and produce something that reads as an intentional, well-run engineering effort rather than a quick script.

Code does not need to work or be fully complete. LoginID has said explicitly they care more about the design and the reasoning behind it, and about which AI tools/frameworks were used and how. Every track should document its reasoning, not just its output.

## The assignment (verbatim from LoginID)

Provide code solutions for the following questions in Go (preferably) or other languages — describe any tool, framework, or AI used.

1. Code of a database DAO that can store and retrieve `user_profile` (name, address, phone) and `user_credential` (username, method, password), with support for multiple databases (PostgreSQL or CockroachDB, SQLite, etc).
2. Design a RESTful API web service to search and retrieve the above user profile data, plus security API authentication.
3. Design a service connector to third-party identity providers (referred to generically as ABC or XYZ) to retrieve personal data from vendors, with the following endpoints:
   - `/auth` — retrieves an `access_token` based on a POST JSON body `{"username": "<string>", "password": "<string>"}`
   - `/identity` — retrieves user PII (name, phone, address: `street_address`, `locality`, `region`, `postal_code`, `country`) based on a POST JSON body `{"phone": "<string>", "name": "<string>"}`

Go is the preferred language. Other languages are acceptable if justified.

## Track structure

Five directories, each independently workable, each owning a distinct slice of the problem:

| Directory | Track | Owns |
|---|---|---|
| `01-product-industry-research-design/` | Product & Industry Research (incl. Design) | Market/industry framing (IAM, passwordless auth, IDP aggregation space), personas and use cases, API and data contract design rationale, UX of the surfaces that would sit on top of this |
| `02-ai-security-architecture/` | AI Architecture & Security Architecture | The AI tooling/workflow used to produce this submission, threat modeling, auth/authz design for the API, credential and PII handling, secrets management, third-party connector security |
| `03-engineering-delivery/` | Engineering & Delivery | The actual Go code for all three questions, project structure, test strategy, delivery documentation |
| `04-infra-devops/` | Infra & DevOps | How this would be built, containerized, deployed, and operated — CI/CD, environments, multi-database configuration, observability |
| `05-data-ops/` | Data Ops | Data modeling for `user_profile` / `user_credential`, multi-database abstraction strategy, migrations, PII data governance and retention |

Directories are numbered for reading order in the final submission, not for sequencing of work — tracks can be worked in any order or in parallel.

## Working principles for every track

- State your reasoning, not just your conclusion. LoginID is grading the "why" as much as the "what."
- Name the AI tool/workflow you used for that track's output (this project is being built with Claude Code; say so, and say how — e.g., subagents scoped per track, a given skill, a given model).
- Keep each track's CLAUDE.md self-contained enough that a fresh agent with no other context can pick it up and do correct work from it alone, plus this root file.
- Cross-reference other tracks by relative path (e.g., `../05-data-ops/CLAUDE.md`) rather than restating their content.
- Implementation is gated per story, not project-wide: Warren gave the implementation go on 2026-09-12 (evening) as an agile loop. Code (`.go`, `.sql`, `go.mod`, Dockerfiles, CI config) may be written only for a story the Product Owner has pulled, refined with the research team, and handed to engineering — recorded in `<track>/refinement/<LT-key>.md`. Work happens on a `feature/<LT-key>` branch, never directly on `main`. A story is done only when it is deployed to the public URL and the Product Owner has reviewed it there.
- **Attribution rule (Warren, 2026-09-12): every commit, PR, and Jira ticket says who did the work.** *Commits:* the git author is the persona doing the work — set once per worktree with `git config --worktree user.name "<Persona Name> (<track> <role>, <model>)"` and `git config --worktree user.email "<slug>@agents.loginid-takehome.invalid"` (`extensions.worktreeConfig` is enabled repo-wide) — and every commit message ends with trailers: `Story: LT-nn`, `Work-By: <persona> (<seat>); <persona> (<seat>)…` naming each hire who contributed and what they did (implementation, tests, review, objection seat), then `Co-Authored-By: Claude <model> <noreply@anthropic.com>`. *PRs:* title `LT-nn: <summary>`; body has a **Who did the work** section (lead, hires and seats, reviewers incl. 02's independent reviewer where applicable, models), plus links to the refinement doc and the Jira issue. *Jira:* every transition and every PR link is a comment beginning `Work-By: …` with the same names; the story's final comment before Done names who deployed and who reviewed. The PM applies the same rule to its own commits. Persona names, not "Claude" or "the team".
- **Shared working tree rule.** Every lead session shares this one checkout, so `git checkout`/`git switch` here changes everyone's tree — **never run them in this directory.** Feature branches live in worktrees outside the repo: `git worktree add /Users/warrenthompson/Source/LoginID-worktrees/<LT-key> -b feature/<LT-key> main`; work, commit, and push from that directory; the root tree stays on `main`. Remote: `origin` = `git@github.com:wtvamp/loginid-takehome.git` (public). Go code lives at the repo root (`cmd/`, `internal/`, `go.mod`); track directories hold documents.

## Org, hiring, and orchestration

The project is run as an org: this PM agent (Dana Whitfield) orchestrates five standing track-lead agents, and each lead hires a small sub-team of specialist agents with deliberately varied personalities. **Each track lead runs as its own top-level Claude Code session**, launched from its track directory in its own tmux window — so it loads its own track `CLAUDE.md` and persona natively, owns and spawns its hires as named teammates in that window, and talks to the PM session through cross-session messaging. Hires belong to their lead's session, not the PM's. The governing files, all at the root:

- `PLAN.md` — the approved org plan for the planning phase (phases, gates, verification). `~/.claude/plans/` is scratch; this is the record.
- `PLANNING.md` — the thin cross-track board: one row per track, one writer per row, plus the service-boundaries section 03 owns and 04 reads.
- `CASTING.md` — archetype palette, org-wide variance rules, hire sizing, the persona-writing standard, the orchestration-pattern catalog (adversarial pair, red/blue, three hats, rotating devil's advocate, structured written debate, newcomer's question), the deep-research rule, the hire-creation procedure, and the registry of every hire.
- `LOG.md` — dated, append-only journal of what happened and why (team-lead writes it; it is part of the submission).
- `.claude/agents/` — one agent definition per hire (model pinned; a once-only per-agent `PreToolUse` hook displays its own persona on its first tool call). `.claude/AGENT_TEMPLATE.md` is the starting point (kept outside the agents directory so it is not registered as an agent type itself).
- `02-ai-security-architecture/planning-approach.md` — the agreed Claude-architecture approach: model/effort by role, coordination rules, hand-off standard, dependency order.

Rules that hold across every track:
- Every agent — lead or hire — has a profile-gen persona (name, temperament, professional history, technical background, stated blind spot). Hires are created text-first with `--output file`; portraits arrive later from `scripts/portrait-queue.sh`. Hires are **never** added as `profile-gen` markers in any `CLAUDE.md` — only the six markers that exist today.
- Hires do not spawn subagents. Only track leads (and team-lead) authorize or run deep research (patents, whitepapers, standards, primary sources); hires request it from their lead.
- Tracks never modify root-level state (this file, the root persona, root config); flag it to team-lead instead.
- **Decisions already made are not re-asked.** Warren's rulings reach tracks through the PM and are recorded in `LOG.md` and this file. A lead must not raise an `AskUserQuestion` to Warren for a decision that is already on the record (e.g., the deployment target, the public repo, the code go) — it asks the PM by message, which answers from the record or escalates. A dialog left open in a lead's session blocks that lead and everything queued behind it (this cost DevOps an hour on 2026-09-12). Genuinely new decisions that only Warren can make still go to Warren — through the PM.
- Hand-offs are files by relative path, written so the receiver can act without re-deriving the sender's reasoning; chat messages signal readiness, they do not carry content.
- Edit `CLAUDE.md` files at turn boundaries only (prompt-cache stability). Milestone `git` commits at the end of each phase; no mid-task commits.

## Status

Planning Phases 0–3 are complete (2026-09-12): five track plans approved, 18 hires live in five lead sessions, every track's Phase 3 debates and hand-offs delivered, and the cross-track consistency pass reported (`decisions/cross-track-consistency.md`: 52 findings, six bounded fixes required before planning is declared done — in progress, owned per finding). Phase 4 is under way: Jira Epics `LT-1`…`LT-5` exist, tracks 01 and 05 are transcribed, 02/03/04 follow the pass fixes. All implementation code was deliberately wiped earlier in the day and none has been written since; **no code until Warren's explicit go-ahead**, which he gives after the PM reports the backlog implementation-ready.

<!-- profile-gen:start slug=dana-whitfield -->
@profiles/dana-whitfield/dana-whitfield.md
<!-- profile-gen:end slug=dana-whitfield -->
