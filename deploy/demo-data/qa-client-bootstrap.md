# QA client credential — out-of-band seeding runbook

**Format change (handoff-03-auth.md v7):** `gen-argon2-hash.go` now emits a PHC-format hash instead of the earlier `<base64-salt>$<base64-hash>` encoding — the two are not interchangeable. Any row seeded before this change must be re-seeded (re-run step 1 for a new secret, then the rotation `UPDATE` in "Rotation / revocation" below) before it will verify again.

Per Priya's ruling on LT-51: "migrations carry reference data, never credentials" — the QA client row in `oauth_client` is seeded by hand, directly against the issuer database's own migrator role, never through a migration file, never through the CI-applied manifest set (`deploy/rbac.yaml` grants the deployer nothing on `secrets`, so it has no path to this credential regardless), and never with the plaintext secret or its hash committed anywhere in this repo.

## 1. Generate the secret and its Argon2id hash

```
CLIENT_SECRET=$(openssl rand -hex 24)
go run deploy/gen-argon2-hash.go "$CLIENT_SECRET" > /tmp/qa-hash.txt
HASH=$(cat /tmp/qa-hash.txt)
rm -f /tmp/qa-hash.txt
```

`deploy/gen-argon2-hash.go` produces a standard PHC-format Argon2id hash (`$argon2id$v=19$m=...,t=...,p=...$<salt>$<hash>`) — the same self-describing encoding `user_credential.secret` already uses, and what `internal/api/clientstore.go`'s `verifySecret` reads its cost parameters back from (never assumed to match the app's own current constants — Priya Nandakumar's ruling, `handoff-03-auth.md` v7). Verified by round-tripping a test hash through the app's own comparison logic before this runbook was written, not assumed from reading the source, and again by a permanent test (`internal/api/clientstore_test.go`'s hand-constructed-different-parameters case) that fails if the verifier is ever changed to ignore the embedded parameters.

**Give `$CLIENT_SECRET` to whoever will run the PO review script and nowhere else** — it is the Basic-auth password for `POST /auth/token`. It cannot be recovered from `$HASH` afterward; if lost, generate a new secret and re-run step 2 (an `UPDATE`, not a fresh `INSERT`).

## 2. Insert the row

```
MIGRATOR_DSN=$(kubectl -n loginid-takehome get secret \
  issuer-db-migrator-credential -o jsonpath='{.data.dsn}' | base64 -d)

psql "$MIGRATOR_DSN" -v ON_ERROR_STOP=1 -c "
  INSERT INTO oauth_client (client_id, name, client_secret_hash, secret_state, granted_scopes, audience, status)
  VALUES ('qa-review-client', 'PO review script', '$HASH', 'set', ARRAY['profile:read:own'], 'loginid-api-service', 'active');
"

unset MIGRATOR_DSN CLIENT_SECRET HASH
```

`granted_scopes` is a single-element array (`profile:read:own`) — the schema's own `ck_oauth_client_granted_scopes_single` CHECK requires exactly one, matching this project's one-credential-per-scope provisioning model (`decisions/search-authz-scoping.md`). Adjust the scope value if the review script needs a different one; do not add a second element, the CHECK will reject it.

**`audience` must be the literal `AUTH_JWT_AUDIENCE` value api-service actually verifies against — `loginid-api-service`, per `PLANNING.md`'s config surface — not the Kubernetes Service name `api-service`.** An earlier run of this runbook seeded rows with `audience='api-service'`; those tokens minted fine but every protected route 401'd, since the verifier's audience check compares against the config literal, not the Service name. If migrating existing rows: `UPDATE oauth_client SET audience='loginid-api-service' WHERE audience='api-service';` via the migrator DSN.

## 3. Confirm it without re-exposing the secret

```
kubectl -n loginid-takehome exec postgres-0 -- env PGPASSWORD="$(kubectl -n loginid-takehome get secret issuer-db-runtime-credential -o jsonpath='{.data.password}' | base64 -d)" \
  psql "sslmode=require host=localhost user=issuer_runtime dbname=issuer" \
  -c "SELECT client_id, secret_state, status FROM oauth_client WHERE client_id = 'qa-review-client';"
```

Expect one row, `secret_state = 'set'`, `status = 'active'` — confirms the insert landed and that the runtime role (the one `api-service-issuer` actually connects as at request time) can read it, without ever printing the hash or the plaintext secret in this check.

## Rotation / revocation

- **Rotate:** repeat step 1 for a new secret, then `UPDATE oauth_client SET client_secret_hash = '$HASH' WHERE client_id = 'qa-review-client';` via the migrator DSN.
- **Revoke:** `UPDATE oauth_client SET secret_state = 'revoked', client_secret_hash = NULL WHERE client_id = 'qa-review-client';` — the schema's tying CHECK requires `client_secret_hash IS NULL` whenever `secret_state` isn't `'set'`.
