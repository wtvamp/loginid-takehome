package api

import (
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
)

func TestHashSecret_VerifySecret_RoundTrip(t *testing.T) {
	hash, err := hashSecret("correct horse battery staple")
	if err != nil {
		t.Fatalf("hashSecret: %v", err)
	}
	if !verifySecret("correct horse battery staple", hash) {
		t.Errorf("verifySecret rejected the correct secret")
	}
	if verifySecret("wrong secret", hash) {
		t.Errorf("verifySecret accepted an incorrect secret")
	}
}

func TestHashSecret_DistinctSaltsPerCall(t *testing.T) {
	h1, err := hashSecret("same-secret")
	if err != nil {
		t.Fatalf("hashSecret: %v", err)
	}
	h2, err := hashSecret("same-secret")
	if err != nil {
		t.Fatalf("hashSecret: %v", err)
	}
	if h1 == h2 {
		t.Errorf("two hashes of the same secret are identical — salt isn't varying")
	}
	if !verifySecret("same-secret", h1) || !verifySecret("same-secret", h2) {
		t.Errorf("both independently-salted hashes must still verify the same secret")
	}
}

func TestVerifySecret_MalformedHash_RejectsWithoutPanic(t *testing.T) {
	for _, malformed := range []string{"", "no-dollar-sign", "$", "not-base64$also-not-base64"} {
		if verifySecret("anything", malformed) {
			t.Errorf("verifySecret(%q) = true, want false for a malformed hash", malformed)
		}
	}
}

// TestHashSecret_EmitsPHCFormat confirms hashSecret's output is an
// actual PHC string, not merely something verifySecret happens to accept
// — the enforcement site for "PHC, not the old bare encoding"
// (Priya Nandakumar's ruling, handoff-03-auth.md v7).
func TestHashSecret_EmitsPHCFormat(t *testing.T) {
	hash, err := hashSecret("some-secret")
	if err != nil {
		t.Fatalf("hashSecret: %v", err)
	}
	wantPrefix := "$argon2id$v=19$m=65536,t=1,p=4$"
	if !strings.HasPrefix(hash, wantPrefix) {
		t.Errorf("hashSecret() = %q, want a prefix of %q", hash, wantPrefix)
	}
}

// TestVerifySecret_ReadsEmbeddedParameters_NotPackageConstants is the
// test with actual teeth (Priya Nandakumar's review): a verifySecret
// that silently recomputed with this package's own current
// argon2Time/argon2Memory/argon2Threads constants instead of the hash's
// own embedded parameters would still pass every round-trip test above
// (hashSecret and verifySecret always agree with themselves) while
// making the PHC string's self-description pointless. This hand-builds
// a hash at DIFFERENT parameters (t=2, not this package's current t=1)
// and confirms it still verifies correctly — the only way that can
// happen is if verifySecret actually read t=2 out of the string and
// used it, not the constant.
func TestVerifySecret_ReadsEmbeddedParameters_NotPackageConstants(t *testing.T) {
	const differentTime = 2 // deliberately not argon2Time (1)
	secret := "hand-constructed-secret"
	salt := []byte("0123456789abcdef") // 16 bytes, fixed for reproducibility
	hash := argon2.IDKey([]byte(secret), salt, differentTime, argon2Memory, argon2Threads, argon2KeyLen)
	handBuilt := encodePHC(argon2Memory, differentTime, argon2Threads, salt, hash)

	if !strings.Contains(handBuilt, ",t=2,") {
		t.Fatalf("test setup bug: hand-built PHC string doesn't contain t=2: %s", handBuilt)
	}
	if !verifySecret(secret, handBuilt) {
		t.Errorf("verifySecret rejected a correctly-hashed secret at a DIFFERENT cost profile (t=2) than this package's current constant (t=1) — it must be recomputing from this package's own constants instead of the hash's embedded parameters")
	}
	if verifySecret("wrong-secret", handBuilt) {
		t.Errorf("verifySecret accepted the wrong secret against a hand-built t=2 hash")
	}
}

// TestNeedsRehash confirms the opportunistic-rehash signal fires only
// when a hash's embedded parameters actually differ from this package's
// current profile — the mechanism handoff-03-auth.md v7's
// verify-then-rehash ruling depends on.
func TestNeedsRehash(t *testing.T) {
	currentHash, err := hashSecret("whatever")
	if err != nil {
		t.Fatalf("hashSecret: %v", err)
	}
	if needsRehash(currentHash) {
		t.Errorf("needsRehash(current-profile hash) = true, want false")
	}

	salt := []byte("0123456789abcdef")
	oldHash := argon2.IDKey([]byte("whatever"), salt, 2, argon2Memory, argon2Threads, argon2KeyLen) // t=2, not current t=1
	oldEncoded := encodePHC(argon2Memory, 2, argon2Threads, salt, oldHash)
	if !needsRehash(oldEncoded) {
		t.Errorf("needsRehash(old-profile hash, t=2 vs current t=1) = false, want true")
	}

	if needsRehash("not-a-valid-phc-string") {
		t.Errorf("needsRehash(unparseable) = true, want false (can't tell, so don't rehash)")
	}
}

func TestDummyHash_IsStableAndValidArgon2idHash(t *testing.T) {
	if dummyHash == "" {
		t.Fatalf("dummyHash must not be empty")
	}
	// dummyHash must itself be verifiable (i.e. it's a real, well-formed
	// Argon2id hash at the same cost profile as every other hash in this
	// store) — the whole point of comparing against it is that it costs
	// the same as a real comparison would.
	if !verifySecret("dummy-fixed-secret-never-issued-to-any-real-client", dummyHash) {
		t.Errorf("dummyHash doesn't verify against its own known plaintext")
	}
}

func TestParsePostgresTextArray(t *testing.T) {
	cases := []struct {
		raw     string
		want    []string
		wantErr bool
	}{
		{raw: "{profile:search}", want: []string{"profile:search"}},
		{raw: "{profile:search,profile:read:any}", want: []string{"profile:search", "profile:read:any"}},
		{raw: "{}", want: nil},
		{raw: "not-an-array", wantErr: true},
		{raw: `{quoted "value"}`, wantErr: true},
		{raw: "{}extra", wantErr: true},
	}
	for _, c := range cases {
		got, err := parsePostgresTextArray(c.raw)
		if c.wantErr {
			if err == nil {
				t.Errorf("parsePostgresTextArray(%q) = %v, want an error", c.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parsePostgresTextArray(%q) unexpected error: %v", c.raw, err)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("parsePostgresTextArray(%q) = %v, want %v", c.raw, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("parsePostgresTextArray(%q) = %v, want %v", c.raw, got, c.want)
				break
			}
		}
	}
}
