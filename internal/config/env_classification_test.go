package config_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"loginid-takehome/internal/config"
)

// envVarLiteral matches an env-var-shaped Go string literal — all
// uppercase, digits, and underscores, with at least one underscore, so
// it doesn't also match unrelated all-caps literals like mode strings
// (which config.go doesn't have any of, but a future edit could add
// one).
var envVarLiteral = regexp.MustCompile(`"([A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+)"`)

// envVarNamesInConfigGo scans config.go's own source text for every
// env-var-shaped string literal — not just the ones passed straight to
// os.Getenv, since resolveDSNFrom's fileEnv/plainEnv parameters are
// literal strings at their call sites, not inside os.Getenv itself.
// Scanning the source text directly (this project's established style
// for migrationSQL/manifest_test.go) catches both shapes without having
// to special-case call syntax.
func envVarNamesInConfigGo(t *testing.T) []string {
	t.Helper()
	root := repoRoot(t)
	src, err := os.ReadFile(filepath.Join(root, "internal", "config", "config.go"))
	if err != nil {
		t.Fatalf("reading config.go: %v", err)
	}

	seen := make(map[string]bool)
	for _, m := range envVarLiteral.FindAllStringSubmatch(string(src), -1) {
		seen[m[1]] = true
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// knownRequiredAuthEnvVars is the union of every env var
// RequiredAuthEnvVars returns across every (service, mode) pair this
// package currently recognizes.
func knownRequiredAuthEnvVars() map[string]bool {
	union := make(map[string]bool)
	pairs := [][2]string{
		{"api-service", "verifier"},
		{"api-service", "issuer"},
		{"idp-connector", ""},
	}
	for _, p := range pairs {
		required, recognized := config.RequiredAuthEnvVars(p[0], p[1])
		if !recognized {
			continue
		}
		for _, name := range required {
			union[name] = true
		}
	}
	return union
}

// TestConfigGoEnvVarsAreClassified is Marcus Ilori's (02) ruling closing
// Ingrid Solano's second PR #53 objection: RequiredAuthEnvVars is a
// hand-maintained list with nothing checking it against config.go's own
// env reads, so a new AUTH_*-prefixed field added straight to Load()
// (or read directly elsewhere) could silently bypass both the startup
// checks and the manifest test — the exact enforcement-site gap this
// whole change exists to close, one level further up. Every env var
// name found in config.go's source must appear either in some
// RequiredAuthEnvVars (service, mode) result, or in config.OptionalEnvVars
// with a stated reason — an unclassified new read fails this test.
func TestConfigGoEnvVarsAreClassified(t *testing.T) {
	required := knownRequiredAuthEnvVars()

	for _, name := range envVarNamesInConfigGo(t) {
		if required[name] {
			continue
		}
		if _, ok := config.OptionalEnvVars[name]; ok {
			continue
		}
		t.Errorf("config.go reads env var %q, which is neither in any RequiredAuthEnvVars(service, mode) result nor in config.OptionalEnvVars with a stated reason — classify it as one or the other", name)
	}
}
