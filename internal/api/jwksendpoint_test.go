package api

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewJWKSHandler_ServesKeyJWKS(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	key := &SigningKey{Private: priv, Kid: "kid-abc"}

	srv := httptest.NewServer(NewJWKSHandler(key))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/.well-known/jwks.json")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var doc jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(doc.Keys) != 1 || doc.Keys[0].Kid != "kid-abc" {
		t.Errorf("unexpected JWKS document: %+v", doc)
	}

	// The verifier side (internal/api/jwks.go's parseJWKS) must be able
	// to actually consume this response — proving the two sides agree on
	// shape, not just that this handler emits *some* JSON.
	keys, err := parseJWKS(mustMarshal(t, doc))
	if err != nil {
		t.Fatalf("parseJWKS rejected NewJWKSHandler's own output: %v", err)
	}
	if _, ok := keys["kid-abc"]; !ok {
		t.Errorf("parseJWKS didn't find kid-abc in the round-tripped document")
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshaling: %v", err)
	}
	return b
}
