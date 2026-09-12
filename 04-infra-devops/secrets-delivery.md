# Secrets Delivery Mechanics

Design-only. This document implements the delivery mechanism for what `../02-ai-security-architecture/handoff-04-secrets.md` v2 specifies must be a secret and why — this track doesn't decide what's a secret, only how it reaches a running process. Row numbers below refer to that hand-off's inventory table.

## Mechanism: Kubernetes-native `Secret` objects, file-mounted

Per the hand-off's Kubernetes-specific requirements: every secret in the inventory is delivered as a `Secret` resource, mounted read-only as a file into the consuming Pod, never passed as a plaintext env var except the one named transitional exception below. Manifests reference `Secret` **names** only (`secretName: api-service-db-dsn`) — values are never in a manifest, never in source, never in an image layer. Values are provisioned out-of-band by whoever applies the manifest (amber-kubernetes, per the deployment-target note) — this take-home describes that step, doesn't perform it.

| Row | Secret | Mount | Consuming ServiceAccount / RBAC |
|---|---|---|---|
| #1 | `DB_DSN` | File, read via `DB_DSN_FILE` (03's convention, per the hand-off's upgraded requirement). Env-var `DB_DSN` supported only as a fallback until 03 ships the `_FILE` convention. | `api-service` SA |
| #2 | JWT signing private key | File only, never env | `api-service-issuer` SA only — a second Deployment of the same `api-service` image, selected by `APP_MODE=issuer`. The verifying `api-service` Deployment's SA mounts nothing from this row (`./containerization-design.md`) |
| #3 | JWT public key / JWKS | Not a `Secret` — `ConfigMap` or fetched URL | n/a |
| #4 | API caller `client_secret`s | Not a Kubernetes `Secret` — lives in the authorization server's own datastore (hashed, Argon2id) | Datastore credentials are shaped like row #1, delivered the same way |
| #5 | `IDP_ABC_CLIENT_ID`/`SECRET` (+ XYZ pair) | File, one `Secret` per vendor so rotating one never touches the other | `idp-connector` SA |
| #6 | Vendor `access_token` | **Not deployed and not built** — per the S3 ruling, `idp-connector` does not cache vendor tokens; nothing to provision here | n/a |
| #7 | KEK (Key-Encrypting-Key) for envelope encryption — wraps a per-record Data-Encryption-Key (DEK) rather than encrypting data directly, so rotating the KEK means re-wrapping DEKs, not re-encrypting everything | File. Not a live requirement — provisioned only if/when TOTP (a supported `auth_method` value, not a committed implementation) actually materializes; the token-caching use case named in an earlier draft no longer applies, per row #6. | `api-service` SA, if/when TOTP lands |
| #8 | TLS serving private keys | `kubernetes.io/tls` `Secret`, automated issuance preferred (cert-manager or equivalent) | Ingress, or each binary if TLS terminates at the pod |
| #9 | End-user vendor password | Nothing to deliver — transient request-body data, never persisted | n/a |
| — | Migration-runner DB credential (05's R4, not in 02's inventory — flagged there as belonging to this surface) | File, same discipline as row #1. **Provisioning differs by backend:** on PostgreSQL specifically this credential needs `CREATE EXTENSION` rights for `pg_trgm` (the trigram index behind `user_profile.name` search) — usually an allow-listed extension on managed Postgres, otherwise elevated privilege; CockroachDB needs nothing extra (trigram indexing is native); SQLite has no privilege model at all. Distinct from the runtime service credential (DML-only, no DDL) per R4 — provisioning one shared credential for both would hand the running service the same extension-install rights the migration job needs, which it has no reason to hold. | Migration `Job`'s own ServiceAccount only |

## RBAC discipline

No ServiceAccount `get`s or `list`s a `Secret` it doesn't mount — enforced by a `Role`/`RoleBinding` per service, scoped to exactly the `Secret` names in the table above for that service, not a blanket namespace-read grant. Row #2 is why this is a topology decision, not an RBAC one: a `Role` restricting `get` on a `Secret` does nothing once that `Secret` is volume-mounted into a container, so keeping the verification path from reading the signing key means never mounting it into `api-service`'s Deployment at all — hence the second `api-service-issuer` Deployment in `./containerization-design.md`, not a Role scoped "more narrowly" on the same Pod.

The audit/access-log read requirement (hand-off, Kubernetes-specific requirements, last bullet) is a **separate** RBAC grant from Secret access — see `./observability.md` for how that log stream is isolated.

## Secrets-at-rest in etcd

Per the hand-off's explicit either/or: this design's answer is **enable encryption at rest for the `Secret` resource type** at the cluster level (an `EncryptionConfiguration` with a KMS or `aescbc` provider), rather than sourcing every value from an external store. Reasoning: an external store (Vault, cloud secrets manager) is real infrastructure this lab cluster doesn't currently run, and standing one up is out of scope for a take-home submission whose deployment is design-only anyway; cluster-level encryption-at-rest is a one-time `kube-apiserver` flag, not a new service to operate. If this became a real deployment with an existing Vault install, that would be the better answer — named here as the alternative, not designed.

## CI-side secret handling

No secret in an image layer, build arg, or CI log — enforced by the pipeline's own gitleaks stage (`./ci-pipeline.md` §4) plus the container design never `COPY`ing anything but the compiled binary. CI's own credentials to reach the registry and (in the describe-only deploy stage) the cluster are themselves managed as CI-platform secrets (GitHub Actions encrypted secrets or equivalent) — same discipline, different delivery surface, not re-specified here since it's a standard CI-platform mechanism rather than something this design invents.

---
*AI tooling note: drafted directly by Theo (Sonnet, this session) from `handoff-04-secrets.md` v2. Bree's (capability-case) and Callum's (scope-cut) per-deliverable notes reviewed and folded in inline above, per PLAN.md §3.*
