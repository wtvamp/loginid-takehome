# Containerization Design

Design-only. No Dockerfile, manifest, or config file is being committed yet — per root `CLAUDE.md`, code/config artifacts wait for Warren's project-wide go-ahead. This document describes what would be built, in enough detail that building it is a transcription exercise, not a design exercise.

Inputs this depends on: `../PLANNING.md` Service boundaries (03, stable), `../02-ai-security-architecture/handoff-04-secrets.md` v2 (02, stable), `../05-data-ops/migration-approach.md` (05, provisional — noted where it matters).

Deployment target: the real Kubernetes dev/lab cluster on Warren's LAN, operated by amber-kubernetes (an external agent, not contacted during this take-home — see `./PLAN.md`). Manifests below are sketched against that cluster's shape, not a generic "some Kubernetes somewhere."

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
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-service
  namespace: loginid-poc          # amber-kubernetes' convention, confirmed at apply time — not queried during this take-home
spec:
  replicas: 2
  selector:
    matchLabels: {app: api-service}
  template:
    metadata:
      labels: {app: api-service}
    spec:
      serviceAccountName: api-service       # scoped per handoff-04-secrets.md row #2 — this SA's Role can `get` the JWT signing key only from the issuing code path, not the verification path
      securityContext:
        runAsNonRoot: true
        readOnlyRootFilesystem: true
      containers:
        - name: api-service
          image: registry.internal/loginid-poc/api-service:<tag>
          resources:
            requests: {cpu: "100m", memory: "128Mi"}
            limits: {cpu: "500m", memory: "256Mi"}
          envFrom:
            - configMapRef: {name: api-service-config}   # DB_DRIVER, HTTP_ADDR, AUTH_JWT_ISSUER/AUDIENCE — non-secret
          env:
            - {name: DB_DSN_FILE, value: /secrets/db/dsn}
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
metadata: {name: api-service}
spec:
  selector: {app: api-service}
  ports: [{port: 443, targetPort: 8080}]
```

`idp-connector`'s manifest is the same shape with its own two `Secret` volume mounts (per-vendor client credentials, one `Secret` per vendor per handoff row #5), its own `resources` block (`idp-connector` is I/O-bound waiting on vendor calls, not CPU-heavy — requests `{cpu: 50m, memory: 64Mi}`, limits `{cpu: 250m, memory: 128Mi}`), and no inbound `Service` — it is called by `api-service` or invoked as a job, never exposed to anything outside the cluster. Exact invocation shape (long-running service vs. on-demand job) is undecided; either way the container/image design above doesn't change.

**On process vs. host isolation (per 03/Renata, `../PLANNING.md`):** the two-binary split is a process boundary, not a placement guarantee — the `resources.limits` above are what actually enforce it if both Pods land on the same node, on top of (not instead of) 03's own connector-side backpressure (`context.WithTimeout`, bounded retry/backoff, their S4).

## 4. Migration job

Per 05's requirement R1/R2 (`../05-data-ops/migration-approach.md` — **provisional, pending Priya's objection-turn ruling; treat this whole section as subject to revision until she marks it final**, though the tool name is the only likely change, not the mechanism): a `Job`, not an init container, with `parallelism: 1` and `backoffLimit` set so it doesn't retry indefinitely against a database it's already partially migrated. Runs `goose` (or whatever 05 finalizes) against the migration-runner credential (DDL-only, distinct from the runtime service credential per R4) before the Deployment's rollout proceeds — enforced by a CI/CD pipeline gate (`./ci-pipeline.md` §4), not by Kubernetes ordering alone, because R2's single-runner requirement has to hold across concurrent deploys too, not just within one.

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

## 6. What's deliberately not here

No service mesh, no Istio/Linkerd sidecar, no HorizontalPodAutoscaler, no PodDisruptionBudget. This is a two-service take-home submission, not a platform. If any of these earn their place later, they're additive to this design, not a rewrite of it.

---
*AI tooling note: drafted directly by Theo (Sonnet, this session) from 03/02/05's hand-offs. Bree's and Callum's per-deliverable notes are pending — folded in per `./PLAN.md` §3 once their turns land, rather than as a separate decision artifact.*
