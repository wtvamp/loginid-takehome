# Decision: Go layout — one binary vs. two

Pattern: three hats (Marisol Ferran, optimist · Nolan Reyes, pessimist · Renata Cole, pragmatist). Diagram: Ines Dabrowski (pending, will be appended when delivered).

## Optimist (Marisol Ferran)

Argued for one binary on take-home-appropriate grounds: every extra binary is authoring/wiring/README ceremony that doesn't teach the reader anything, for a submission graded on demonstrated reasoning. Conceded the isolation argument is real and already published as stable in `PLANNING.md` with 04 building against it, so declined to relitigate the boundary itself. Actual proposal: **keep two binaries, merge the ceremony, not the process boundary** — both `cmd/api-service/main.go` and `cmd/idp-connector/main.go` become thin wrappers around one shared `internal/app` package (route registration, config loading), so the deployables stay two but the authored code stays close to one. Would yield even that if a shared-bootstrap bug risked taking both down at once.

## Pessimist (Nolan Reyes)

Pattern-matched to a production incident: a shared process/connection pool let a slowly-timing-out downstream partner exhaust the pool and take down an unrelated inbound API — 40 minutes of a partner's problem becoming the platform's outage. Confirmed two binaries closes the *shared-fate* version of that failure (a stalled vendor can't starve `api-service`'s own pool). Flagged what it does **not** close: host/deploy colocation (same node, same FD/CPU budget, same on-call page) is a 04 question, not implied by the binary split; and a self-inflicted retry storm in the connector's token-refresh logic (no backoff) is a failure mode independent of process boundaries entirely. Explicitly not arguing against two binaries — arguing that the boundary alone doesn't buy safety without `context.WithTimeout` on every outbound call and a bounded worker pool.

## Ruling (Renata Cole, pragmatist)

**Two binaries stands** — this was already published in `PLANNING.md`'s Service boundaries section as stable, 04 is building against it, and neither hat argued for reopening it. Confirmed rather than relitigated.

**Adopting Marisol's shared-`internal/app` proposal.** Both `cmd/*/main.go` become thin wrappers over `internal/app`: route registration and config loading live once, not twice. This gets the authoring-cost win she argued for without touching the process boundary 04 depends on. Package layout in `PLANNING.md` gets a note added (not a structural change — `internal/app` slots alongside the existing `internal/` packages).

**Adopting Nolan's requirement, not just noting it.** Carrying into S4's spec (connector `/auth` + `/identity`): every outbound HTTP call to a vendor must set a `context.WithTimeout` (no unbounded outbound calls), and the connector's HTTP client uses a bounded connection pool. Token-refresh/retry logic must back off, never tight-loop against a failing vendor endpoint. This is now a stated acceptance criterion for S4, not an assumption riding on "it's a separate binary."

**Flagging to 04, not deciding here (Nolan's other point):** whether `api-service` and `idp-connector` are deployed to the same host/node is 04's call, and the isolation this decision buys is *process*-level, not necessarily *host*-level, unless 04 also separates deployment. Will note this explicitly when the Service boundaries section is next touched, so 04 doesn't read "two binaries" as "two hosts" by default.

## Diagram (Ines Dabrowski)

```
================================================================================
GO LAYOUT — TWO BINARIES, DEPENDENCY DIRECTION
================================================================================

External callers
  │
  │ HTTPS + bearer token (JWT, iss=https://auth.loginid-takehome.internal,
  │                             aud=loginid-api-service)
  ▼
┌──────────────────────────────┐        ┌──────────────────────────────┐
│   cmd/api-service            │        │   cmd/idp-connector          │
│   (client-facing, inbound)   │        │   (outbound-only to vendors) │
│                               │        │                               │
│  main.go                     │        │  main.go                     │
│    │                         │        │    │                         │
│    ▼                         │        │    ▼                         │
│  internal/api                │        │  internal/connector          │
│   - router                   │        │   - POST /auth  (client)     │
│   - auth middleware ─────────┼──┐     │   - POST /identity (client)  │
│   - handlers (search/get     │  │     │   - token lifecycle          │
│     user_profile)            │  │     │     (TTL/refresh/no-        │
│    │                         │  │     │      plaintext-persist,      │
│    ▼                         │  │     │      per 02 connector-       │
│  internal/dao                │  │     │      security.md)            │
│   - Repository (composite):  │  │     │    │                         │
│     Profiles()               │  │     └────┼─────────────────────────┘
│     Credentials()            │       vendor XYZ/ABC (POST /auth,     │
│     Methods()                │       /identity — assignment-fixed)   │
│   - postgres / cockroachdb /  │
│     sqlite implementations   │
│    │                         │
│    ▼                         │
│  internal/model               │  ◄── shared by BOTH binaries
│   UserProfile, UserCredential │      (no behavior, structs only)
└──────────────┬────────────────┘
               │
               ▼
       internal/config   ◄── shared by BOTH binaries
        env-var loading only: DB_DRIVER, DB_DSN, DB_DSN_FILE,
        HTTP_ADDR, AUTH_JWT_ISSUER, AUTH_JWT_AUDIENCE,
        IDP_ABC_BASE_URL, IDP_ABC_CLIENT_ID/SECRET

--------------------------------------------------------------------------------
DEPENDENCY DIRECTION (arrows = "depends on", never reversed)
--------------------------------------------------------------------------------
  cmd/api-service   → internal/api → internal/dao → internal/model
  cmd/idp-connector → internal/connector → internal/model
  both cmd/*        → internal/config

  internal/model and internal/config are the ONLY packages both binaries import.
  Neither is a dumping ground: model = data shape only, config = env parsing only.
  internal/api and internal/connector never import each other.
  internal/dao is imported ONLY by cmd/api-service — idp-connector has no DB access,
  by design: it never touches user_profile/user_credential storage, only vendor PII
  in flight.

--------------------------------------------------------------------------------
WHY TWO BINARIES, NOT ONE (the boundary this diagram backs)
--------------------------------------------------------------------------------
- Different threat models: api-service holds the DB and validates inbound bearer
  tokens; idp-connector holds vendor credentials and calls out. A shared process
  means a vendor-call bug (slow vendor, panic, goroutine leak) can degrade the
  client-facing API — the exact incident shape Nolan's pessimist hat is naming.
- Independent scaling: connector traffic is bursty/vendor-latency-bound; api-service
  traffic is client-request-bound. One binary couples their resource profiles for
  no shared concern.
- Mirrors the assignment's own Q2/Q3 split — not an invented boundary.

--------------------------------------------------------------------------------
WHERE I'D CALL "TOO CLEVER" ON MYSELF (blind spot, named up front)
--------------------------------------------------------------------------------
- internal/api and internal/connector are NOT further split into sub-packages
  (no internal/api/handlers vs internal/api/middleware). At this project's size
  that split would be abstraction for its own sake — one package per binary-
  facing concern is enough. If Marisol's optimist case is "one binary, less
  ceremony," the honest counter isn't "more packages," it's: the two-binary
  split earns its keep on threat model + scaling, not on package count. If that
  reasoning doesn't hold, the diagram — not my opinion — changes.
================================================================================
```

Note: the diagram predates the ruling's addition of `internal/app` (shared bootstrap) below — that package sits alongside `internal/config` as a second package both binaries import (route registration + config wiring only, still no behavior shared beyond wiring).

## Status

Ruled. Diagram delivered and appended above.

---
Model: sonnet (Marisol Ferran, optimist; Nolan Reyes, pessimist; Renata Cole, lead, pragmatist + ruling). Turns consumed: 2 hire turns + lead synthesis.
