# Containerization Design

Implementation-phase update: Warren gave the implementation go-ahead as an agile loop (`../PLANNING.md`). The Dockerfiles this document describes are now real and committed (LT-45, PRs #4/#5); the manifests below are still the design reference the real deploy job builds from. Deployment target confirmed: Warren's real, shared cluster, not an isolated lab sandbox — see `decisions/deploy-path.md` for the discovery that corrected this and the direct confirmation obtained before proceeding.

Inputs this depends on: `../PLANNING.md` Service boundaries (03, stable), `../02-ai-security-architecture/handoff-04-secrets.md` v2 (02, stable), `../05-data-ops/migration-approach.md` (05, **final**), `decisions/deploy-path.md` (namespace, image registry, public URL).

## 1. Two images, not one

Mirrors 03's two-binary split: `cmd/api-service` and `cmd/idp-connector` are two separate images with independent Dockerfiles, tags, and Deployments. Reasons this isn't one image with two entrypoints: different threat models (per 02 — `api-service` is client-facing with bearer-token auth, `idp-connector` is outbound-only to vendors), different scaling needs, and a vulnerability in one image's dependency tree doesn't force a rebuild/redeploy of the other.

## 2. Dockerfile shape (both images, same pattern)

Multi-stage Go build, three stages:

1. **`builder`** — `golang:1.2x-alpine` (or `-bookworm` if cgo is needed by a driver), `go mod download` cached as its own layer before `COPY . .`, then `CGO_ENABLED=0 go build -o /out/<binary> ./cmd/<binary>`. Static binary, no libc dependency to drag into the final image.
2. **`(none)`** — no separate test stage in the image build itself; tests run in CI (see `./ci-pipeline.md`), not as a Docker build step. A test failure should fail the pipeline stage, not silently produce a passing image build with a skipped test.
3. **`final`** — `gcr.io/distroless/static` (or `scratch` if no CA certs are needed — `idp-connector` needs them for outbound TLS to vendors, `api-service` doesn't unless its DB driver requires them). Non-root user (distroless's default `nonroot:nonroot`, UID 65532). Only the compiled binary and, for `idp-connector`, `/etc/ssl/certs` get copied in. No shell, no package manager, nothing an attacker can `exec` into even with a foothold.

Both images run as a read-only root filesystem (`securityContext.readOnlyRootFilesystem: true` at the Pod spec level) — neither binary writes to disk at runtime; migrations are a separate job (see §4), not something either service does to itself on boot.

## 3. Deployment/Service manifest sketch — `api-service`

```yaml
apiVersion: v1
kind: Namespace
metadata: {name: loginid-takehome}
---
apiVersion: v1
kind: ResourceQuota
metadata: {name: loginid-takehome-quota, namespace: loginid-takehome}
spec:
  hard:
    requests.cpu: "1"
    requests.memory: 1Gi
    limits.cpu: "2"
    limits.memory: 2Gi
    pods: "10"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-service
  namespace: loginid-takehome     # a real, isolated namespace on Warren's shared cluster — see decisions/deploy-path.md §3
spec:
  replicas: 2
  selector:
    matchLabels: {app: api-service}
  template:
    metadata:
      labels: {app: api-service}
    spec:
      serviceAccountName: api-service       # this SA mounts nothing from the signing-key row — see the issuer Deployment below
      securityContext:
        runAsNonRoot: true
        readOnlyRootFilesystem: true
      containers:
        - name: api-service
          image: ghcr.io/wtvamp/loginid-takehome/api-service:<tag>
          resources:
            requests: {cpu: "100m", memory: "128Mi"}
            limits: {cpu: "500m", memory: "256Mi"}
          envFrom:
            - configMapRef: {name: api-service-config}   # DB_DRIVER, HTTP_ADDR, AUTH_JWT_ISSUER/AUDIENCE — non-secret
          env:
            - {name: DB_DSN_FILE, value: /secrets/db/dsn}
          volumeMounts:
            - {name: db-dsn, mountPath: /secrets/db, readOnly: true}
          readinessProbe: {httpGet: {path: /healthz, port: 8080}, initialDelaySeconds: 5}
          livenessProbe: {httpGet: {path: /healthz, port: 8080}, initialDelaySeconds: 10}
      volumes:
        - name: db-dsn
          secret: {secretName: api-service-db-dsn}
---
apiVersion: v1
kind: Service
metadata: {name: api-service}
spec:
  selector: {app: api-service}
  ports: [{port: 443, targetPort: 8080}]
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-service-issuer
  namespace: loginid-takehome
spec:
  replicas: 2
  selector:
    matchLabels: {app: api-service-issuer}
  template:
    metadata:
      labels: {app: api-service-issuer}
    spec:
      serviceAccountName: api-service-issuer   # the ONLY SA that mounts the JWT signing key, per handoff-04-secrets.md row #2 / F13's fix
      securityContext:
        runAsNonRoot: true
        readOnlyRootFilesystem: true
      containers:
        - name: api-service
          image: ghcr.io/wtvamp/loginid-takehome/api-service:<tag>   # same image as api-service — mode selected below, not a separate build
          resources:
            requests: {cpu: "100m", memory: "128Mi"}
            limits: {cpu: "500m", memory: "256Mi"}
          envFrom:
            - configMapRef: {name: api-service-config}
          env:
            - {name: DB_DSN_FILE, value: /secrets/db/dsn}
            - {name: APP_MODE, value: issuer}   # 02's resolution to F13: same image as the verifying Deployment, a second Deployment/SA, selected by this env var — not a third binary, not a flag (the config surface permits none)
          volumeMounts:
            - {name: db-dsn, mountPath: /secrets/db, readOnly: true}
            - {name: jwt-signing-key, mountPath: /secrets/jwt, readOnly: true}
          readinessProbe: {httpGet: {path: /healthz, port: 8080}, initialDelaySeconds: 5}
          livenessProbe: {httpGet: {path: /healthz, port: 8080}, initialDelaySeconds: 10}
      volumes:
        - name: db-dsn
          secret: {secretName: api-service-db-dsn}
        - name: jwt-signing-key
          secret: {secretName: api-service-jwt-signing-key}
---
apiVersion: v1
kind: Service
metadata: {name: api-service-issuer}
spec:
  selector: {app: api-service-issuer}
  ports: [{port: 443, targetPort: 8080}]
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: loginid-takehome
  namespace: loginid-takehome
  annotations: {cert-manager.io/cluster-issuer: letsencrypt-prod}
spec:
  ingressClassName: nginx
  tls:
    - hosts: [loginid-takehome.uplifttech.org]
      secretName: loginid-takehome-tls
  rules:
    - host: loginid-takehome.uplifttech.org
      http:
        paths:
          - {path: /healthz, pathType: Exact, backend: {service: {name: api-service, port: {number: 443}}}}
          # remaining API path prefix pending 03's final route naming; issuer-specific
          # routes (e.g. /.well-known/jwks.json, a token endpoint) go to api-service-issuer.
          # idp-connector gets no route here — never publicly reachable, by design.
```

No `NetworkPolicy` on `api-service` restricts inbound traffic from the `Ingress` — the default-deny `NetworkPolicy` in §5 is scoped to `idp-connector` only, deliberately, since `api-service` needs to accept traffic from the ingress controller and that path isn't the one this design is defending.

**Reachability:** the verifying `api-service` Deployment fetches the JWKS from `api-service-issuer`'s in-cluster `Service` (`https://api-service-issuer.loginid-takehome.svc/jwks`) at startup and on rotation, per row #3's "fetched from the issuer" option — no config variable or NetworkPolicy rule exists yet for this path, both are 04's to add once 03 wires the JWKS fetch client (flagged in the vocabulary sweep, receivers Theo/Renata). Nothing blocks this by default: the `NetworkPolicy` in §5 above is a default-deny scoped to `idp-connector`'s ingress only, so egress from `api-service` (and from `idp-connector`, if it ever needs a token from the issuer directly) to `api-service-issuer`'s `Service` is unrestricted namespace-default traffic, not something this design has to open a hole for.

**Why two Deployments of one image, not RBAC on one:** a `Role` restricting `get` on the signing-key `Secret` does nothing once that `Secret` is already volume-mounted into a container — every replica of that Deployment can read the file regardless of who's allowed to `kubectl get` it. The only way to keep the verification code path (every `api-service` replica) from being able to read the private key is to never mount it there at all. `APP_MODE=issuer` on the second Deployment is 03's addition to the config surface (env-var only, no flags, per `PLANNING.md`); the same binary handles both modes, wired by that variable, not two separate images. This was F13 in the consistency pass — the earlier version of this manifest mounted the key into `api-service` directly, which the RBAC comment here used to (incorrectly) imply was sufficient.

`idp-connector`'s manifest is the same shape with its own two `Secret` volume mounts (per-vendor client credentials, one `Secret` per vendor per handoff row #5), its own `resources` block (`idp-connector` is I/O-bound waiting on vendor calls, not CPU-heavy — requests `{cpu: 50m, memory: 64Mi}`, limits `{cpu: 250m, memory: 128Mi}`), and no inbound `Service` — it is called by `api-service` or invoked as a job, never exposed to anything outside the cluster. Exact invocation shape (long-running service vs. on-demand job) is undecided; either way the container/image design above doesn't change.

**On process vs. host isolation (per 03/Renata, `../PLANNING.md`):** the two-binary split is a process boundary, not a placement guarantee — the `resources.limits` above are what actually enforce it if both Pods land on the same node, on top of (not instead of) 03's own connector-side backpressure (`context.WithTimeout`, bounded retry/backoff, their S4).

## 4. Migration job

Per 05's requirement R1/R2 (`../05-data-ops/migration-approach.md`, **final**): a `Job`, not an init container, with `parallelism: 1` and `backoffLimit` set so it doesn't retry indefinitely against a database it's already partially migrated. Runs `goose` against the migration-runner credential (DDL-only, distinct from the runtime service credential per R4 — and, on PostgreSQL specifically, needing `CREATE EXTENSION` rights for `pg_trgm` that the runtime credential must never hold, per `./secrets-delivery.md`) before the Deployment's rollout proceeds — enforced by a CI/CD pipeline gate (`./ci-pipeline.md` §4), not by Kubernetes ordering alone, because R2's single-runner requirement has to hold across concurrent deploys too, not just within one.

R2's mechanism is orchestration-layer exclusivity (a concurrency-gated pipeline stage, `./ci-pipeline.md` §4), not a database lock — final reasoning: a transaction-scoped lock (`pg_advisory_xact_lock`, CockroachDB-compatible) releases between `goose`'s per-file transactions, so a second runner could interleave mid-run even with one held; a session-scoped lock would span the whole run but CockroachDB doesn't support that scope. No single database mechanism covers both the right scope and the right engine, so the exclusivity has to live outside the database entirely, on every backend, not just CockroachDB. 05 also names a cheap backstop — a unique constraint on `goose`'s version-table `version_id` column, added in the first migration regardless of whether goose's default schema already has one — so a second runner that does slip through fails loudly on insert instead of interleaving silently. That's a migration-file change (03/05's to make), not something 04 builds, but it's worth naming here since it's the backstop behind this job's own exclusivity design, not a replacement for it.

**Fail-closed schema check (R3, final):** the readiness probe below isn't just a liveness ping — `api-service` must refuse to serve if either of goose's two version tables (`shared/` and its backend-specific directory, per `migration-approach.md` §2's two-invocation mechanism) is behind the version the binary was built against. A check against only one table can pass while the other is stale — e.g. `shared/` current but the Postgres-only `pg_trgm` index migration unapplied — which is a passing check that proves nothing. `/healthz` above is liveness only; a separate `/readyz` (or the same endpoint gated by both checks) is where this lives, wired to whatever version-embedding mechanism 03 picks (build-time constant vs. reading the shipped migration files) — the check comparing *both* tables is 04's requirement to build against; the embedding mechanism is 03's call.

## 5. NetworkPolicy: adopted, not cut

Considered and dropped from an earlier pass as scope creep, then reconsidered: §3's claim that `idp-connector` is isolated because it never gets an inbound `Service` is a paper boundary without one. Anything else on the namespace's default-allow network can still reach it directly and skip `api-service`'s auth path entirely — which undermines the exact "different threat models" reasoning 02 gave for splitting the images in the first place. A minimal default-deny `NetworkPolicy` for the `idp-connector` Pod, with one explicit `ingress` rule allowing traffic only from `api-service`'s pod selector, is ~15 lines and one more manifest to keep in sync — cheap relative to what it closes. Included below; everything else in this section stays cut.

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: idp-connector-default-deny}
spec:
  podSelector: {matchLabels: {app: idp-connector}}
  policyTypes: [Ingress]
  ingress:
    - from: [{podSelector: {matchLabels: {app: api-service}}}]
```

## 5a. Standing checklist: adding any new component to `loginid-takehome`

Per Tobias's (01) flagged pattern — this got caught by hand twice (F13's signing-key RBAC gap in LT-32, then the JWKS/`/auth/token` NetworkPolicy conflict this same track found itself) before it became a rule rather than a one-off review catch. Every new Deployment/StatefulSet/Service added to this namespace answers, explicitly, before it ships:

1. **NetworkPolicy scope** — the namespace default is allow, not deny (per `decisions/deploy-path.md`'s "shared production, isolate aggressively" framing). Name exactly which pods/namespaces may reach this component's ingress; if the answer is "namespace default," say so on purpose, don't leave it unstated.
2. **Secret access, if any** — is a Secret mounted into this component? If so, is CI's RBAC (`deploy/rbac.yaml`) capable of reading it (it shouldn't be, for anything sensitive), and is there a CI-side guard (like `manifest-secret-guard`) preventing that Secret's name from appearing in any other component's manifest?
3. **ResourceQuota headroom** — does the namespace quota (`deploy/manifests.yaml`'s `ResourceQuota`) actually have room for this component's `resources.limits`? Check before shipping, not after a `FailedCreate`.
4. **Public exposure** — does this component get an `Ingress` route, or is it in-cluster only? State which, and if in-cluster only, confirm no route was added for it by accident.

## 6. What's deliberately not here

No service mesh, no Istio/Linkerd sidecar, no HorizontalPodAutoscaler, no PodDisruptionBudget. This is a two-service take-home submission, not a platform. If any of these earn their place later, they're additive to this design, not a rewrite of it.

---
*AI tooling note: drafted directly by Theo (Sonnet, this session) from 03/02/05's hand-offs. Bree's and Callum's per-deliverable notes are pending — folded in per `./PLAN.md` §3 once their turns land, rather than as a separate decision artifact.*
