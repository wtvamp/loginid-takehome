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
