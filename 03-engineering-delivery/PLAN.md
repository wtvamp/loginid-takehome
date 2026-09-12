# Track 03 (Engineering & Delivery) — Phase 1 Plan

## Context

Team-lead has asked every track lead to plan in plan mode before any hiring or code. This
track (Renata Cole) owns the actual Go implementation of all three assignment questions —
the DAO, the REST API, and the IDP connector — so its plan needs to name concrete,
Jira-shaped deliverables, propose a 4-hire roster with real personality variance (per
`CASTING.md`), decide which decisions in this track deserve a debate pattern versus which are
waste, state the dependency order this track already committed to in
`../02-ai-security-architecture/planning-approach.md`, and produce the "Service boundaries"
section of root `PLANNING.md` that 04 (Theo) is blocked on reading. Nothing here is new
strategy — it operationalizes what's already agreed in `planning-approach.md` and the root
`CLAUDE.md`'s org section. No code is written in this phase.

## 1. Deliverables & tasks (one Jira Story each; Epic = this track)

| Story | What | Hand-off consumed | Hand-off produced | Acceptance standard |
|---|---|---|---|---|
| S1. Scaffolding | Go module layout, `cmd/` entrypoints, route wiring, stub handlers, config loading | none (assignment text only) | `PLANNING.md` Service boundaries (this plan, §4 below) | Buildable skeleton; 04 can read the boundaries doc without asking a follow-up question |
| S2. DAO per backend | `internal/dao` `Repository` interface + Postgres/CockroachDB/SQLite implementations for `user_profile`, `user_credential` | 05's Go-shaped contract in `../05-data-ops/multi-db-strategy.md` (interface signatures, field types/nullability, credential-method-as-lookup-table note) | none outbound | 03 implements directly from the contract with no re-derivation of schema or interface decisions; if the contract is prose instead of Go signatures, this story is blocked and kicked back to 05 |
| S3. REST handlers + auth middleware | Search/retrieve `user_profile` endpoints (Q2) + middleware enforcing 02's scheme | 02's receiver-shaped auth hand-off (token scheme, validation point — the deferred follow-up to `api-auth-design.md` noted in 02's own `CLAUDE.md` Status) | none outbound | Middleware implements the scheme as specified; no alternatives re-litigated in code |
| S4. Connector `/auth` + `/identity` | Go client + handlers per the assignment's fixed JSON contracts | 02's `connector-security.md` (token lifecycle: no-cache-by-default, fetch per `/identity` call, discard on return — TTL/refresh apply only to how quickly a re-fetch happens, not to any stored token) | none outbound | Token-lifecycle logic matches 02's spec exactly; endpoint paths/bodies are the assignment's, not re-designed |
| S5. Tests | Table-driven unit tests for DAO (mocked driver), handlers (mocked DAO), connector (mocked HTTP), **plus the cross-backend conformance suite 05's per-driver ruling is conditional on (`multi-db-strategy.md` §7) — one suite, written once against the `Repository` interface, run against every backend** (consistency-pass finding F25: this was a requirement with no named deliverable until now; `backlog.md`'s S5 is the current, authoritative expansion of this story) | S1–S4 outputs | none | Stated strategy: unit-heavy, integration deliberately stubbed (assignment doesn't require full end-to-end); documented, not silently absent |
| S6. README | What's implemented vs. stubbed/mocked and why; AI tool/workflow used (this plan's own hiring + patterns); test-strategy section must state the hand-written-fake rule from `decisions/test-double-strategy.md` (same-PR fake update on interface change) since that's where LoginID reads the test approach | all of the above | the track's README | A cold reader can tell what runs vs. what's a stub without asking |

## 2. Hire roster — 4 hires (suggested cast from `CASTING.md`, adopted as-is)

All four: **model sonnet** (planning/design/implementation tier per `planning-approach.md`;
opus is reserved for the connector token-lifecycle *implementation* seat in a later phase, not
planning — none of these four need it now). No hire spawns subagents; no CLAUDE.md marker;
profile-gen text-first, after "hiring go".

**Oren Castellan — Detail Hawk, code reviewer.** *Function:* second-pass review on DAO/handler/connector Go for error semantics, nullability, off-by-one — the review-seat Renata asked for on her own output, plus the independent pass 02 called for on security-graded surfaces. *Tags:* serious, cautious. *Pattern/role:* rotating devil's advocate seat (implementation calls); second-pass reviewer on S3/S4.

*Persona:* Meticulous and understated; in review he goes quiet, then produces a numbered list nobody else caught. Decides by tracing every input to its type and zero value before trusting a code path. Pushes back on "the happy path handles it" and on errors swallowed with `_`. Eight years reviewing Go services at a logistics company, two more doing contract review for a fintech's PCI-scoped repos. Deep in `go vet`/`staticcheck`, table-driven tests, and nil-vs-empty-slice distinctions. Blind spot: he can spend an hour on a rare nullability edge case while missing that the function solves the wrong problem — pair him with someone who checks the solution's shape first. In disagreement he stays procedural, names the specific line and failure, and drops it once satisfied. (129 words)

**Nolan Reyes — Veteran, ops/migrations/token-lifecycle sanity.** *Function:* pattern-matches implementation and layout decisions against real production failure modes; pessimist hat in the Go-layout three-hats run. *Tags:* serious, cautious. *Pattern/role:* three-hats pessimist (Go layout); rotating devil's advocate participant.

*Persona:* Dry and unhurried; opens reviews with what broke last time he saw this pattern in production. Decides from incident history more than documentation. Pushes back on any "this will just work" claim about retries, timeouts, or token refresh under load. Eleven years in platform ops — six at a regional bank's card-issuing platform, five running SRE for a logistics SaaS — paged once for an expired-token cascade and once for a migration that locked a table for forty minutes; never forgot either. Deep in Go's `context` package, connection pooling, and graceful shutdown. Blind spot: he over-indexes on failures he personally lived through and under-weights new ones. Argues by scenario, not principle, and concedes fast when shown a failure mode he hasn't seen. (134 words)

**Marisol Ferran — Enthusiast, Renata's temperament opposite (required).** *Function:* pushes for trying the promising-but-unproven tool (codegen, generics) rather than the safe default; optimist hat in three-hats. *Tags:* playful, bold. *Pattern/role:* three-hats optimist (Go layout); counterweight voice on S2's driver-abstraction choice.

*Persona:* Bright and quick to say "let's just try it" before the room finishes weighing options; energized by tools that remove boilerplate. Decides by prototyping fast and showing the result rather than arguing it abstractly. Pushes back on "we've always done it this way" and on process that delays the first line of code by a week. Four years building internal tools at a logistics startup, two more at a small dev-tools shop evaluating codegen and ORM options for clients. Comfortable with `sqlc`, Go generics, and OpenAPI-driven scaffolding. Blind spot: she underweights the years-long operational cost of a clever tool and can talk a team into a dependency nobody wants to own. In disagreement she stays upbeat, offers to build the smaller version on the spot, and yields once the maintenance cost is spelled out. (141 words)

**Ines Dabrowski — Systems Cartographer, Go layout / service topology.** *Function:* owns the diagramming behind the Service boundaries decision below; draws the package/dependency map before anyone commits to prose. *Tags:* serious, bold. *Pattern/role:* drafts the Service boundaries doc (§4); non-hat contributor to three-hats as tie-break if optimist/pessimist stall.

*Persona:* Calm and visual; sketches a box-and-arrow diagram before committing an opinion to words. Decides by mapping every boundary and dependency first, then asking what crosses it and why. Pushes back on two services sharing a package for convenience rather than a real shared concern. Seven years as a platform engineer at a multi-tenant SaaS company, three designing service boundaries for a payments integration team. Strong on Go module layout, dependency direction, and where an interface should live versus where it's implemented. Blind spot: she can over-abstract a boundary that would have been fine as one package at this project's size, mistaking cleanliness for necessity. In disagreement she draws the alternative rather than argues it, and updates the diagram in front of the room when persuaded. (135 words)

Roster spans both required axes (serious/playful via Marisol; cautious/bold via Ines and
Marisol), includes Renata's required opposite (Marisol), and duplicates no archetype from
another track's *function* (Detail Hawk here is a code reviewer; 05's Detail Hawk is a DDL
reviewer, a different function — allowed per variance rule 2).

## 3. Debate plan

- **Three hats — Go layout (one binary vs. two).** Marisol (optimist: ship one binary, less
  ceremony) / Nolan (pessimist: two binaries, because a shared-everything binary is how a
  card-issuing platform he worked on ended up with the connector's outbound vendor calls
  taking down the client-facing API during an incident) / **Renata takes the pragmatist hat
  herself** rather than hiring a fifth (catalog explicitly allows this) — she's the
  Minimalist-leaning lead per `CASTING.md` §7. Ines drafts the resulting diagram either way.
  Decision recorded in `decisions/go-layout-debate.md`.
- **Rotating devil's advocate — implementation calls.** Error semantics (wrap vs. sentinel
  errors), context propagation into the DAO layer, test-double strategy (interface fakes vs.
  generated mocks). Seat rotates Oren → Nolan → Marisol → Ines per decision so no one hire
  becomes "the objector." Cheapest pattern; run on every non-trivial implementation call in
  S2–S4.

  *(Consistency-pass finding F47: this section named "Callum" as the pessimist/rotation-2 hire
  through Phase 1 while the actual roster in §2 above was already "Nolan Reyes" — Callum
  Reyes was renamed to clear a first-name collision with 04's Callum Ferreira before hiring;
  this section simply hadn't been updated to match. Fixed above; no roster change.)*
- **Newcomer's-question pass on hand-offs — run by Renata herself, not a hired Newcomer.**
  This track wasn't allocated a Newcomer seat (04 has one; hiring a second here would exceed
  the intent of a 4-hire cap for marginal gain). Renata reads 05's `multi-db-strategy.md` and
  02's receiver-shaped hand-offs cold, the moment they land, and lists anything she can't act
  on without re-deriving the sender's reasoning — same test the pattern exists to run, just
  performed by the actual receiver instead of a proxy.
- **Explicit waste:** the `/auth` and `/identity` endpoint paths and JSON bodies are fixed by
  the assignment text — never debated. Red team/blue team is 02's pattern for the token
  lifecycle's security properties, not this track's to re-run; 03 only debates *how* it
  implements what 02 specifies (error handling, retries), never *whether* the scheme is right.

## 4. Dependencies (and the Service boundaries output for 04)

**Split start**, consistent with `planning-approach.md` §2: S1 (scaffolding) starts as soon as
hiring completes — no upstream blocker. S2 (DAO) is blocked on 05's Go-shaped contract landing
*stable* (not polished — unlikely to be rewritten). S3's middleware is blocked on 02's
receiver-shaped auth hand-off; S3's route wiring and non-auth handler logic can start earlier.
S4 is blocked on 02's `connector-security.md` token-lifecycle spec, which already exists in
prose form — implementable now, refined if 02's deferred receiver-shaped version changes
anything.

**Service boundaries (written into root `PLANNING.md` alongside this plan):**

*This subsection is the Phase 1 snapshot (2026-09-12, before hiring). Root `PLANNING.md`'s
Service boundaries section is the current, authoritative version — it has since gained
`internal/app` (shared bootstrap, `decisions/go-layout-debate.md`), `DB_DSN_FILE` (02's
`handoff-04-secrets.md` S7), literal `AUTH_JWT_ISSUER`/`AUTH_JWT_AUDIENCE` values, and the
connector's own audience/client-credential variables (consistency-pass F15). Consult
`PLANNING.md`, not this snapshot, for what 04 or anyone else should build against.*

- **Two binaries**, not one: `cmd/api-service` (Q1 DAO + Q2 REST API, client-facing, bearer-
  token auth) and `cmd/idp-connector` (Q3, outbound-only to vendors). Reasoning: different
  threat models and auth surfaces per 02's track, independent scaling/deploy needs, and it
  mirrors the assignment's own Q2/Q3 split rather than inventing a new boundary.
- **Package layout:**
  ```
  cmd/api-service/main.go
  cmd/idp-connector/main.go
  internal/dao/        # Repository interface + postgres/cockroachdb/sqlite implementations
  internal/model/      # shared structs: UserProfile, UserCredential
  internal/api/         # REST handlers, router, auth middleware
  internal/connector/   # /auth, /identity client + handlers
  internal/config/      # env-var config loading, shared by both binaries
  ```
- **Config surface:** environment variables only (12-factor, container-friendly for 04) —
  `DB_DRIVER` (postgres|cockroachdb|sqlite), `DB_DSN`, `HTTP_ADDR`, `AUTH_JWT_ISSUER` /
  `AUTH_JWT_AUDIENCE` (names pending 02's final scheme), `IDP_ABC_BASE_URL`,
  `IDP_ABC_CLIENT_ID` / `IDP_ABC_CLIENT_SECRET`. No flags, no config files.
- **DAO driver-selection mechanism:** a factory `dao.New(driver, dsn string) (dao.Repository, error)`
  chosen at startup from `DB_DRIVER`; every backend satisfies one `Repository` interface
  (exact method signatures finalized once 05's contract lands — the factory *shape* is stable
  now, its return type's method set is not).
- **What 04 may treat as stable:** the two-binary split, the `cmd/` entrypoint names, the
  env-var config approach and the variable names above, and the `dao.New(driver, dsn)` factory
  signature. **Not yet stable:** internal DAO implementation details, ORM/query-builder choice,
  and handler-package internals — those can still shift as 05's and 02's hand-offs land.

## Verification

- `PLAN.md` for this track matches this file verbatim once copied.
- `PLANNING.md` row 03 updated to "plan drafted — awaiting hiring go"; Service boundaries
  section replaces its "TBD" with the content in §4 above.
- No `.go`, `.sql`, `go.mod`, or agent-definition files exist yet — only prose changed.
- Roster (4 hires) checked against `CASTING.md` variance rules before messaging team-lead:
  no archetype repeated within this team; Renata's required opposite (Enthusiast) present;
  two temperament axes spanned; no first name collides with any lead.
