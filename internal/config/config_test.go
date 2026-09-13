package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoad_DSNFilePrecedence asserts DB_DSN_FILE wins over DB_DSN when both
// are set, per PLANNING.md's Service boundaries and handoff-04-secrets.md's
// S7 revision — this is the enforcement site LT-34's acceptance criteria
// names for that precedence rule.
func TestLoad_DSNFilePrecedence(t *testing.T) {
	dir := t.TempDir()
	dsnFilePath := filepath.Join(dir, "dsn")
	if err := os.WriteFile(dsnFilePath, []byte("from-file\n"), 0o600); err != nil {
		t.Fatalf("writing dsn file: %v", err)
	}

	t.Setenv("DB_DSN", "from-env")
	t.Setenv("DB_DSN_FILE", dsnFilePath)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got, want := cfg.DBDSN, "from-file"; got != want {
		t.Errorf("DBDSN = %q, want %q (DB_DSN_FILE must take precedence over DB_DSN)", got, want)
	}
}

func TestLoad_DSNEnvFallback(t *testing.T) {
	t.Setenv("DB_DSN", "from-env")
	t.Setenv("DB_DSN_FILE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got, want := cfg.DBDSN, "from-env"; got != want {
		t.Errorf("DBDSN = %q, want %q (DB_DSN must be used when DB_DSN_FILE is unset)", got, want)
	}
}

func TestLoad_AppModeDefault(t *testing.T) {
	t.Setenv("APP_MODE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got, want := cfg.AppMode, "verifier"; got != want {
		t.Errorf("AppMode = %q, want %q (default when APP_MODE is unset)", got, want)
	}
}

// TestLoad_AppModeRejectsUnknownValue is LT-32's guard against a typo'd
// APP_MODE silently falling through to verifier (or being misread as
// issuer) — a startup error here, per Load's own doc comment, rather than
// a silent default.
func TestLoad_AppModeRejectsUnknownValue(t *testing.T) {
	t.Setenv("APP_MODE", "verifyer") // deliberate typo

	if _, err := Load(); err == nil {
		t.Fatal("Load should reject an unrecognized APP_MODE value, got nil error")
	}
}

// TestLoad_IssuerModeRequiresSigningKeyFile is LT-32's core enforcement
// site for "the api-service (verifying) Deployment holds only the public
// key material needed to verify, never the private key, regardless of
// APP_MODE's runtime value": issuer mode with JWT_SIGNING_KEY_FILE unset
// must fail startup rather than run with no key at all. This is the case
// that would occur if the verifying Deployment's pod spec (which never
// sets this variable, per its manifest) had APP_MODE misconfigured to
// "issuer" — this test proves that misconfiguration crashes the process
// instead of silently serving as an unkeyed issuer.
func TestLoad_IssuerModeRequiresSigningKeyFile(t *testing.T) {
	t.Setenv("APP_MODE", "issuer")
	t.Setenv("JWT_SIGNING_KEY_FILE", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load should reject APP_MODE=issuer with JWT_SIGNING_KEY_FILE unset, got nil error")
	}
}

// TestLoad_IssuerModeRequiresSigningKeyFileToBeReadable covers the
// physical-absence half of the same criterion from the opposite
// direction: JWT_SIGNING_KEY_FILE pointing at a path that does not exist
// (the exact shape of "the Secret volume was never mounted") must also
// fail startup, not just an unset variable.
func TestLoad_IssuerModeRequiresSigningKeyFileToBeReadable(t *testing.T) {
	t.Setenv("APP_MODE", "issuer")
	t.Setenv("JWT_SIGNING_KEY_FILE", filepath.Join(t.TempDir(), "does-not-exist"))

	if _, err := Load(); err == nil {
		t.Fatal("Load should reject APP_MODE=issuer with an unreadable JWT_SIGNING_KEY_FILE, got nil error")
	}
}

// TestLoad_IssuerModeWithSigningKeyFileSucceeds is the positive case: a
// real issuer Deployment, with the Secret actually mounted, must start
// cleanly and report the path back on Config.
func TestLoad_IssuerModeWithSigningKeyFileSucceeds(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "signing.key")
	if err := os.WriteFile(keyPath, []byte("fake-key-material"), 0o600); err != nil {
		t.Fatalf("writing fake signing key: %v", err)
	}

	t.Setenv("APP_MODE", "issuer")
	t.Setenv("JWT_SIGNING_KEY_FILE", keyPath)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.JWTSigningKeyPath != keyPath {
		t.Errorf("JWTSigningKeyPath = %q, want %q", cfg.JWTSigningKeyPath, keyPath)
	}
}

// TestLoad_VerifierModeIgnoresSigningKeyFile asserts the verifying binary
// never surfaces a signing-key path on Config even if the variable
// happens to be set in its environment (e.g. a stray value left over from
// a shared env template) — Config.JWTSigningKeyPath is the only thing
// anything downstream could use to find the private key, so it must be
// empty whenever AppMode is not "issuer", unconditionally.
func TestLoad_VerifierModeIgnoresSigningKeyFile(t *testing.T) {
	t.Setenv("APP_MODE", "verifier")
	t.Setenv("JWT_SIGNING_KEY_FILE", "/should/never/be/read")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.JWTSigningKeyPath != "" {
		t.Errorf("JWTSigningKeyPath = %q, want empty in verifier mode regardless of the env var", cfg.JWTSigningKeyPath)
	}
}
