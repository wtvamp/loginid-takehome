package api

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
)

func writeTestKeyFile(t *testing.T, der []byte, pemType string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "key.pem")
	block := &pem.Block{Type: pemType, Bytes: der}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("writing test key file: %v", err)
	}
	return path
}

func TestLoadSigningKey_PKCS1(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	path := writeTestKeyFile(t, x509.MarshalPKCS1PrivateKey(priv), "RSA PRIVATE KEY")

	key, err := LoadSigningKey(path)
	if err != nil {
		t.Fatalf("LoadSigningKey: %v", err)
	}
	if key.Private.N.Cmp(priv.N) != 0 {
		t.Errorf("loaded key's modulus doesn't match the original")
	}
	if key.Kid == "" {
		t.Errorf("Kid must be non-empty")
	}
}

func TestLoadSigningKey_PKCS8(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshaling PKCS8: %v", err)
	}
	path := writeTestKeyFile(t, der, "PRIVATE KEY")

	key, err := LoadSigningKey(path)
	if err != nil {
		t.Fatalf("LoadSigningKey: %v", err)
	}
	if key.Private.N.Cmp(priv.N) != 0 {
		t.Errorf("loaded key's modulus doesn't match the original")
	}
}

func TestLoadSigningKey_KidStableAcrossReloads(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	path := writeTestKeyFile(t, x509.MarshalPKCS1PrivateKey(priv), "RSA PRIVATE KEY")

	key1, err := LoadSigningKey(path)
	if err != nil {
		t.Fatalf("first LoadSigningKey: %v", err)
	}
	key2, err := LoadSigningKey(path)
	if err != nil {
		t.Fatalf("second LoadSigningKey: %v", err)
	}
	if key1.Kid != key2.Kid {
		t.Errorf("kid changed across reloads of the same key: %q vs %q — a restart would break every cached verifier's kid lookup", key1.Kid, key2.Kid)
	}
}

func TestLoadSigningKey_NotPEM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(path, []byte("not a pem file"), 0o600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}
	if _, err := LoadSigningKey(path); err == nil {
		t.Errorf("expected an error loading a non-PEM file, got nil")
	}
}

func TestSigningKey_JWKS_RoundTripsPublicKey(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	key := &SigningKey{Private: priv, Kid: "kid-123"}

	doc := key.JWKS()
	if len(doc.Keys) != 1 {
		t.Fatalf("JWKS has %d keys, want 1", len(doc.Keys))
	}
	k := doc.Keys[0]
	if k.Kid != "kid-123" || k.Kty != "RSA" || k.Alg != "RS256" || k.Use != "sig" {
		t.Errorf("unexpected JWK fields: %+v", k)
	}

	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		t.Fatalf("decoding n: %v", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		t.Fatalf("decoding e: %v", err)
	}
	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)
	if n.Cmp(priv.N) != 0 {
		t.Errorf("JWKS modulus doesn't match the signing key's public modulus")
	}
	if int(e.Int64()) != priv.E {
		t.Errorf("JWKS exponent = %d, want %d", e.Int64(), priv.E)
	}
}
