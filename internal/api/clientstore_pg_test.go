package api

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// setupIssuerDB connects to TEST_POSTGRES_DSN (skipping the test if
// unset, same discipline as internal/dao/postgres's own setupDB — real
// Postgres verification is this file's responsibility, not something a
// hand-written fake could ever exercise), applies the real
// migrations/issuer/00001_initial_schema.sql into a fresh, uniquely-named
// schema, and tears it down on cleanup.
func setupIssuerDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — skipping issuer-database integration test")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("opening TEST_POSTGRES_DSN: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)

	schemaName := fmt.Sprintf("test_issuer_%d", rand.Int63())
	if _, err := db.Exec("CREATE SCHEMA " + schemaName); err != nil {
		t.Fatalf("creating test schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec("DROP SCHEMA " + schemaName + " CASCADE")
	})
	if _, err := db.Exec("SET search_path TO " + schemaName); err != nil {
		t.Fatalf("setting search_path: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Join(wd, "..", "..") // this package: <repoRoot>/internal/api
	migrationPath := filepath.Join(repoRoot, "migrations", "issuer", "00001_initial_schema.sql")
	b, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("reading migration %s: %v", migrationPath, err)
	}
	content := string(b)
	upStart := strings.Index(content, "-- +goose Up")
	downStart := strings.Index(content, "-- +goose Down")
	if upStart == -1 || downStart == -1 {
		t.Fatalf("migration missing +goose Up/Down markers")
	}
	upSQL := content[upStart+len("-- +goose Up") : downStart]
	if _, err := db.Exec(upSQL); err != nil {
		t.Fatalf("applying issuer migration: %v", err)
	}

	return db
}

func insertOAuthClient(t *testing.T, db *sql.DB, clientID, name string, secretHash *string, secretState string, scopes []string, audience, status string) {
	t.Helper()
	scopeLiteral := "{" + strings.Join(scopes, ",") + "}"
	_, err := db.Exec(`
		INSERT INTO oauth_client (client_id, name, client_secret_hash, secret_state, granted_scopes, audience, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, clientID, name, secretHash, secretState, scopeLiteral, audience, status)
	if err != nil {
		t.Fatalf("inserting oauth_client row: %v", err)
	}
}

func TestPostgresClientStore_Get_Found(t *testing.T) {
	db := setupIssuerDB(t)
	hash, err := hashSecret("s3cr3t")
	if err != nil {
		t.Fatalf("hashing secret: %v", err)
	}
	insertOAuthClient(t, db, "qa-review-client", "QA Review Client", &hash, "set", []string{"profile:search"}, "loginid-api-service", "active")

	store := NewPostgresClientStore(db)
	rec, ok, err := store.Get(context.Background(), "qa-review-client")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatalf("Get returned ok=false for a row that exists")
	}
	if rec.Name != "QA Review Client" || rec.Audience != "loginid-api-service" || rec.GrantedScope != Scope("profile:search") || !rec.Active {
		t.Errorf("unexpected record: %+v", rec)
	}
	if !verifySecret("s3cr3t", rec.ClientSecretHash) {
		t.Errorf("stored hash doesn't verify against the secret it was created with")
	}
}

func TestPostgresClientStore_Get_NotFound(t *testing.T) {
	db := setupIssuerDB(t)
	store := NewPostgresClientStore(db)

	_, ok, err := store.Get(context.Background(), "does-not-exist")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ok {
		t.Errorf("Get returned ok=true for a client_id that was never inserted")
	}
}

func TestPostgresClientStore_Get_SecretStateNone_EmptyHash(t *testing.T) {
	db := setupIssuerDB(t)
	insertOAuthClient(t, db, "not-yet-secreted", "No Secret Yet", nil, "none", []string{"profile:read:any"}, "loginid-api-service", "active")

	store := NewPostgresClientStore(db)
	rec, ok, err := store.Get(context.Background(), "not-yet-secreted")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatalf("Get returned ok=false for a row that exists")
	}
	if rec.ClientSecretHash != "" {
		t.Errorf("ClientSecretHash = %q, want empty for secret_state=none", rec.ClientSecretHash)
	}
}

func TestPostgresClientStore_Get_DisabledStatus(t *testing.T) {
	db := setupIssuerDB(t)
	hash, err := hashSecret("s3cr3t")
	if err != nil {
		t.Fatalf("hashing secret: %v", err)
	}
	insertOAuthClient(t, db, "disabled-client", "Disabled", &hash, "set", []string{"profile:search"}, "loginid-api-service", "disabled")

	store := NewPostgresClientStore(db)
	rec, ok, err := store.Get(context.Background(), "disabled-client")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatalf("Get returned ok=false for a row that exists")
	}
	if rec.Active {
		t.Errorf("Active = true for status=disabled")
	}
}

// TestOAuthClient_SchemaConstraints exercises the migration's own CHECK
// constraints directly against a real Postgres — the DDL is the actual
// enforcement site for these invariants, not application code, so the
// test belongs here rather than only asserted in prose.
func TestOAuthClient_SchemaConstraints(t *testing.T) {
	db := setupIssuerDB(t)

	cases := []struct {
		name string
		sql  string
		args []any
	}{
		{
			name: "client_id too short fails the charset CHECK",
			sql:  `INSERT INTO oauth_client (client_id, name, secret_state, granted_scopes, audience, status) VALUES ($1, 'x', 'none', '{profile:search}', 'aud', 'active')`,
			args: []any{"short"},
		},
		{
			name: "secret_state=set with NULL hash violates the tying CHECK",
			sql:  `INSERT INTO oauth_client (client_id, name, client_secret_hash, secret_state, granted_scopes, audience, status) VALUES ('valid-client-id', 'x', NULL, 'set', '{profile:search}', 'aud', 'active')`,
		},
		{
			name: "secret_state=none with a non-NULL hash violates the tying CHECK",
			sql:  `INSERT INTO oauth_client (client_id, name, client_secret_hash, secret_state, granted_scopes, audience, status) VALUES ('valid-client-id', 'x', 'somehash', 'none', '{profile:search}', 'aud', 'active')`,
		},
		{
			name: "multiple granted_scopes violates the single-scope CHECK",
			sql:  `INSERT INTO oauth_client (client_id, name, secret_state, granted_scopes, audience, status) VALUES ('valid-client-id', 'x', 'none', '{profile:search,profile:read:any}', 'aud', 'active')`,
		},
		{
			// array_length(x, 1) returns NULL (not 0) for an empty array,
			// and a CHECK passes on NULL — this insert is exactly what
			// the original (broken) CHECK text would have silently
			// admitted (Priya Nandakumar's review, PR #34).
			name: "empty granted_scopes violates the single-scope CHECK",
			sql:  `INSERT INTO oauth_client (client_id, name, secret_state, granted_scopes, audience, status) VALUES ('valid-client-id', 'x', 'none', '{}', 'aud', 'active')`,
		},
		{
			name: "a NULL granted_scopes element violates the single-scope CHECK",
			sql:  `INSERT INTO oauth_client (client_id, name, secret_state, granted_scopes, audience, status) VALUES ('valid-client-id', 'x', 'none', ARRAY[NULL]::text[], 'aud', 'active')`,
		},
		{
			name: "an empty-string granted_scopes element violates the single-scope CHECK",
			sql:  `INSERT INTO oauth_client (client_id, name, secret_state, granted_scopes, audience, status) VALUES ('valid-client-id', 'x', 'none', ARRAY['']::text[], 'aud', 'active')`,
		},
		{
			name: "unrecognized status violates the status enum CHECK",
			sql:  `INSERT INTO oauth_client (client_id, name, secret_state, granted_scopes, audience, status) VALUES ('valid-client-id', 'x', 'none', '{profile:search}', 'aud', 'pending')`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := db.Exec(c.sql, c.args...)
			if err == nil {
				t.Errorf("expected a CHECK-constraint violation, got no error")
			}
		})
	}
}

// TestOAuthClient_UpdatedAtTrigger proves the BEFORE UPDATE trigger
// actually advances updated_at — the enforcement site is the trigger
// firing on a real UPDATE, not the migration text alone (Priya
// Nandakumar's review, PR #34: this table has no application-code
// writing layer to rely on for setting updated_at, unlike the main DAO
// schema, so the trigger is the only thing that can't be forgotten).
func TestOAuthClient_UpdatedAtTrigger(t *testing.T) {
	db := setupIssuerDB(t)
	insertOAuthClient(t, db, "trigger-test-client", "Trigger Test", nil, "none", []string{"profile:search"}, "aud", "active")

	var createdAt, updatedAtBefore time.Time
	if err := db.QueryRow(`SELECT created_at, updated_at FROM oauth_client WHERE client_id = $1`, "trigger-test-client").Scan(&createdAt, &updatedAtBefore); err != nil {
		t.Fatalf("querying initial row: %v", err)
	}
	if !createdAt.Equal(updatedAtBefore) {
		t.Fatalf("precondition failed: created_at (%v) and updated_at (%v) should start equal", createdAt, updatedAtBefore)
	}

	time.Sleep(10 * time.Millisecond) // ensure now() advances past the default's own now()
	if _, err := db.Exec(`UPDATE oauth_client SET status = 'disabled' WHERE client_id = $1`, "trigger-test-client"); err != nil {
		t.Fatalf("updating row: %v", err)
	}

	var updatedAtAfter time.Time
	if err := db.QueryRow(`SELECT updated_at FROM oauth_client WHERE client_id = $1`, "trigger-test-client").Scan(&updatedAtAfter); err != nil {
		t.Fatalf("querying updated row: %v", err)
	}
	if !updatedAtAfter.After(updatedAtBefore) {
		t.Errorf("updated_at did not advance after UPDATE: before=%v after=%v", updatedAtBefore, updatedAtAfter)
	}
}
