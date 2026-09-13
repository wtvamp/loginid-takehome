package api

import "testing"

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
