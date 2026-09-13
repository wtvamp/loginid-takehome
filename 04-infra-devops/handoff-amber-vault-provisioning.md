# Hand-off: Vault provisioning for `loginid-takehome` (LT-46)

**From:** Amber (OpenClaw Kubernetes agent on the Mac mini, model claude-opus-4-7), on Warren's authorization, 2026-09-13 10:06–10:25 PDT. **Requested by:** Dana Whitfield (PM). **Receiver:** Theo Bergman (04) for the manifest side; Helena Marsh (02) for policy review; Naomi Voss (01) for the refinement record.

This is Amber's report, verbatim from her session output, lightly framed. It contains names and paths only; no secret value, token, or key appears here or appeared in her session. The request she answered is summarised at the end.

---

## 1. Kubernetes auth — already enabled, no change needed

`auth/kubernetes` was already mounted and correctly configured for **this** cluster: `kubernetes_host = https://192.168.0.44:6443` (uplift-kub-ctrl-plane-1), `token_reviewer_jwt_set = true`, `disable_iss_validation = true`. I enabled nothing and re-configured nothing.

## 2–3. KV mount and the copy

**Mount `secret/`** — pre-existing and already KV **v2** (`options.version=2`), so I did not enable a new one. Prefix **`secret/loginid-takehome/`** (it joins existing `fmn/` and `mail/`).

All six copied, key names preserved exactly, each at version 1:

| Vault path | keys |
|---|---|
| `secret/loginid-takehome/db-runtime-credential` | `dsn`, `password` |
| `secret/loginid-takehome/db-migrator-credential` | `dsn`, `password` |
| `secret/loginid-takehome/issuer-db-runtime-credential` | `dsn`, `password` |
| `secret/loginid-takehome/issuer-db-migrator-credential` | `dsn`, `password` |
| `secret/loginid-takehome/api-service-jwt-signing-key` | `signing-key` |
| `secret/loginid-takehome/postgres-superuser` | `password` |

Verified by SHA-256 per key, source vs. read-back — all identical. `postgres-server-tls`, `loginid-takehome-tls`, `qa-client-credential` untouched. **The Kubernetes Secrets were copied, not deleted** — nothing is removed until Theo's side is live and you say so.

## 4–5. Policies and roles

Five policies, `read` only, on exact `secret/data/loginid-takehome/<name>` paths; five `auth/kubernetes` roles of the same names, `token_ttl=1h`, `token_max_ttl=4h`, namespace `loginid-takehome`.

| Role / policy name | bound ServiceAccount | read paths |
|---|---|---|
| `loginid-takehome-api-service` | `api-service` | db-runtime-credential, connector-client-credential |
| `loginid-takehome-api-service-issuer` | `api-service-issuer` | issuer-db-runtime-credential, api-service-jwt-signing-key |
| `loginid-takehome-db-migrate` | **`db-migrate`** | db-migrator-credential, issuer-db-migrator-credential |
| `loginid-takehome-postgres` | **`postgres`** | postgres-superuser + all four db credentials |
| `loginid-takehome-idp-connector` | **`idp-connector`** | none (policy exists, grants nothing) |

`connector-client-credential` returns **404, not 403** for api-service — permitted by policy, just not populated yet. LT-33 can write it with no Vault change.

## ⚠️ The blocker: three workloads have no ServiceAccount

Your brief assumed each workload had its own `serviceAccountName`. It doesn't. Across the entire repo only `api-service` and `api-service-issuer` are defined. **`postgres` (StatefulSet), `db-migrate` (Job), and `idp-connector` (Deployment) declare no `serviceAccountName` and run as `default`** — confirmed against both the repo manifests and the live pods after Theo's 17:15Z redeploy.

I did not bind three roles to `default`. Doing so would mean any pod on the `default` SA could assume **any** of those roles — `idp-connector`, which you ruled gets nothing, could have assumed `loginid-takehome-postgres` and read `postgres-superuser`. That would have inverted the least-privilege requirement while appearing to satisfy it.

Instead I created dedicated ServiceAccounts `postgres`, `db-migrate`, `idp-connector` in the namespace and bound the roles to those. They're inert until used. Verified: `default` is denied all five roles (403), `idp-connector` cannot assume the postgres role (403), and the same SA name in another namespace is denied (403).

**So Theo must add `serviceAccountName: postgres` / `db-migrate` / `idp-connector` to those three workloads.** Until he does, those three cannot authenticate to Vault at all — this is a hard failure, not a degradation. Ideally the SA objects go into `manifests.yaml` too so it's declarative; his `loginid-takehome-deployer` Role already has `serviceaccounts` create/update.

## 6. Vault address for the injector

**`https://vault.vault.svc:8200`** — note **https**, not the `http://` in your brief. The listener has `tls_disable = 0`. Theo does **not** need to set an address annotation: that exact value is already the injector's built-in default (`AGENT_INJECT_VAULT_ADDR`).

For TLS, two options:
- **House pattern** (what ns `mail` uses): `vault.hashicorp.com/tls-skip-verify: "true"`.
- **Proper verification**: I staged Secret **`vault-ca-cert`** (key `ca.crt`) into `loginid-takehome`, fingerprint-identical to the one in ns `vault`. Mount via `vault.hashicorp.com/agent-extra-secret: "vault-ca-cert"` + `vault.hashicorp.com/ca-cert`. The cert's SANs include `vault.vault.svc`, so this validates cleanly.

**Flag:** that CA expires **2026-12-14**, about three months out. Not urgent, not mine to renew unilaterally, but it shouldn't be a surprise.

## End-to-end proof, and one trap for Theo

I ran a throwaway pod through the real injector on the `api-service` SA and deleted it afterwards. Full chain works: SA → k8s auth → role → policy → KV v2 → agent → file.

It also caught something worth having: the obvious multi-line template renders a **trailing newline** into the file — 49 bytes for a 48-byte password. A `\n` on a DB password fails auth looking exactly like a wrong password. The single-line form produces an exact byte match, and I verified this rather than just recommending it:

```
'{{- with secret "secret/data/loginid-takehome/db-runtime-credential" -}}{{ .Data.data.password }}{{- end -}}'
```

## What I changed beyond the brief

- Created ServiceAccounts `postgres`, `db-migrate`, `idp-connector` (reason above).
- Created Secret `vault-ca-cert` in `loginid-takehome`.
- Nothing enabled, nothing deleted, no existing Vault policy or role touched. The four `db-*`/`issuer-*` credentials, the JWT key, and the superuser remain live as Kubernetes Secrets.

No secret value, token, or key appears above or in any file I wrote; the local Vault token cache was shredded and the port-forward is closed.

---

## The request Amber answered (summary)

Copy six named Kubernetes Secrets from `loginid-takehome` into KV v2 at `secret/loginid-takehome/<secret-name>` preserving key names; create five read-only policies and five Kubernetes-auth roles (`loginid-takehome-{api-service, api-service-issuer, db-migrate, postgres, idp-connector}`) bound to the matching ServiceAccounts in the namespace, `token_ttl=1h`, `token_max_ttl=4h`; report names only. Leave `postgres-server-tls`, `loginid-takehome-tls`, and `qa-client-credential` as Kubernetes Secrets by ruling (`../02-ai-security-architecture/handoff-04-secrets.md` v3.1).

## PM notes for the receivers

- **Theo:** the three `serviceAccountName` additions (`postgres`, `db-migrate`, `idp-connector`) are a hard prerequisite; add the ServiceAccount objects to `deploy/manifests.yaml` so they are declarative. Use the single-line template form Amber verified (trailing-newline trap). Prefer the `vault-ca-cert` verification path over `tls-skip-verify`; the CA expiry (2026-12-14) is Warren's platform item, recorded in `teardown.md`'s neighbourhood, not this project's.
- **Helena:** five roles; `loginid-takehome-postgres` is the widest. Amber's denial matrix (default SA denied all five, idp-connector denied postgres, same SA name in another namespace denied) is the evidence to re-run, not re-derive.
- **Deletion of the six Kubernetes Secrets** happens only after Theo's PR is deployed and Naomi's live review passes, on the PM's word, via runbook.

---

## Addendum (10:55 PDT): sixth role, and Amber's independent read of the #56 outage

Amber provisioned `loginid-takehome-retention-sweep` (policy and Kubernetes-auth role, read on `secret/data/loginid-takehome/db-runtime-credential` only), bound to ServiceAccount `retention-sweep` in `loginid-takehome`, which she created in-cluster to mint a test JWT; PR #57's declaration adopts it. Verified: the sweep SA reads that one path and is denied the other five; `default` is denied the role. Nothing else changed.

While there she found the live outage from #56 and diagnosed it independently, matching the PM's and Theo's findings: (1) the injector's default 250m CPU request per agent container pushed the namespace past its `requests.cpu` quota (effective pod request is max(sum of containers, largest init container), so `db-migrate`'s init agent alone charged 250m while it waited for a Postgres that could not be scheduled); (2) `agent-extra-secret` mounts the Secret's keys directly at `/vault/custom/`, so the CA path is `/vault/custom/ca.crt`, which she confirmed with a throwaway pod rather than by reasoning. She deliberately applied neither fix live, because both live in `deploy/manifests.yaml` and a live patch would drift from git and be reverted on the next apply. Both landed in PR #58.
