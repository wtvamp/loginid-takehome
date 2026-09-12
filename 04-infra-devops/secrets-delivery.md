# Secrets Delivery Mechanics

Design-only. This document implements the delivery mechanism for what `../02-ai-security-architecture/handoff-04-secrets.md` v2 specifies must be a secret and why — this track doesn't decide what's a secret, only how it reaches a running process. Row numbers below refer to that hand-off's inventory table.

## Mechanism: Kubernetes-native `Secret` objects, file-mounted

Per the hand-off's Kubernetes-specific requirements: every secret in the inventory is delivered as a `Secret` resource, mounted read-only as a file into the consuming Pod, never passed as a plaintext env var except the one named transitional exception below. Manifests reference `Secret` **names** only (`secretName: api-service-db-dsn`) — values are never in a manifest, never in source, never in an image layer. Values are provisioned out-of-band by whoever applies the manifest (amber-kubernetes, per the deployment-target note) — this take-home describes that step, doesn't perform it.

| Row | Secret | Mount | Consuming ServiceAccount / RBAC |
|---|---|---|---|
| #1 | `DB_DSN` | File, read via `DB_DSN_FILE` (03's convention, per the hand-off's upgraded requirement). Env-var `DB_DSN` supported only as a fallback until 03 ships the `_FILE` convention. | `api-service` SA |
| #2 | JWT signing private key | File only, never env | Restricted Role scoped to the issuing code path within `api-service`'s own SA — not the verification path, per the hand-off's assumption 2 |
| #3 | JWT public key / JWKS | Not a `Secret` — `ConfigMap` or fetched URL | n/a |
| #4 | API caller `client_secret`s | Not a Kubernetes `Secret` — lives in the authorization server's own datastore (hashed, Argon2id) | Datastore credentials are shaped like row #1, delivered the same way |
| #5 | `IDP_ABC_CLIENT_ID`/`SECRET` (+ XYZ pair) | File, one `Secret` per vendor so rotating one never touches the other | `idp-connector` SA |
| #6 | Vendor `access_token` | Not deployed — transient, in-memory only. If cached, encrypted with #7's KEK; no `Secret` object | n/a |
| #7 | KEK for envelope encryption | File. Per the hand-off's resolved v2: not a live requirement until TOTP or token-caching actually materializes — provisioned only if/when 05 or 02 confirms one of those features is real. If a KMS is used instead of a mounted key (row #7a), its access credential follows the same delivery discipline as row #2. | `api-service` and/or `idp-connector` SA, whichever feature lands |
| #8 | TLS serving private keys | `kubernetes.io/tls` `Secret`, automated issuance preferred (cert-manager or equivalent) | Ingress, or each binary if TLS terminates at the pod |
| #9 | End-user vendor password | Nothing to deliver — transient request-body data, never persisted | n/a |
| — | Migration-runner DB credential (05's R4, not in 02's inventory — flagged there as belonging to this surface) | File, same discipline as row #1. **Provisioning differs by backend:** on PostgreSQL specifically this credential needs `CREATE EXTENSION` rights for `pg_trgm` (the trigram index behind `user_profile.name` search) — usually an allow-listed extension on managed Postgres, otherwise elevated privilege; CockroachDB needs nothing extra (trigram indexing is native); SQLite has no privilege model at all. Distinct from the runtime service credential (DML-only, no DDL) per R4 — provisioning one shared credential for both would hand the running service the same extension-install rights the migration job needs, which it has no reason to hold. | Migration `Job`'s own ServiceAccount only |

## RBAC discipline

No ServiceAccount `get`s or `list`s a `Secret` it doesn't mount — enforced by a `Role`/`RoleBinding` per service, scoped to exactly the `Secret` names in the table above for that service, not a blanket namespace-read grant. Per the hand-off, this is narrower still for row #2: `api-service`'s own SA needs a distinct Role restricting the signing-key `Secret` to the issuing code path, which in practice means either a second SA for the issuer mode or a sidecar/init-container pattern that reads the key and the main container process never does — the exact split is an implementation detail for 03, not a change to this delivery mechanism.

The audit/access-log read requirement (hand-off, Kubernetes-specific requirements, last bullet) is a **separate** RBAC grant from Secret access — see `./observability.md` for how that log stream is isolated.

## Secrets-at-rest in etcd

Per the hand-off's explicit either/or: this design's answer is **enable encryption at rest for the `Secret` resource type** at the cluster level (an `EncryptionConfiguration` with a KMS or `aescbc` provider), rather than sourcing every value from an external store. Reasoning: an external store (Vault, cloud secrets manager) is real infrastructure this lab cluster doesn't currently run, and standing one up is out of scope for a take-home submission whose deployment is design-only anyway; cluster-level encryption-at-rest is a one-time `kube-apiserver` flag, not a new service to operate. If this became a real deployment with an existing Vault install, that would be the better answer — named here as the alternative, not designed.

## CI-side secret handling

No secret in an image layer, build arg, or CI log — enforced by the pipeline's own gitleaks stage (`./ci-pipeline.md` §4) plus the container design never `COPY`ing anything but the compiled binary. CI's own credentials to reach the registry and (in the describe-only deploy stage) the cluster are themselves managed as CI-platform secrets (GitHub Actions encrypted secrets or equivalent) — same discipline, different delivery surface, not re-specified here since it's a standard CI-platform mechanism rather than something this design invents.

---
*AI tooling note: drafted directly by Theo (Sonnet, this session) from `handoff-04-secrets.md` v2. Bree's (capability-case) and Callum's (scope-cut) per-deliverable notes reviewed and folded in inline above, per PLAN.md §3.*
