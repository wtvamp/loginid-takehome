// signingkey.go loads the issuer's RSA private key (LT-32's
// JWT_SIGNING_KEY_FILE) and derives its kid — the one piece both the
// token-minting handler and the JWKS-serving handler need. Nothing in
// this package outside this file (and the two handlers that hold a
// *SigningKey by value) ever touches private-key material — the
// verifying binary never imports this file's actual key bytes at all
// (LT-40's own structural guarantee is unaffected: KeySource/JWKSCache
// there only ever handle public keys).
package api

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
)

// SigningKey bundles the parsed private key with its derived kid, so
// every caller (the token handler, the JWKS handler) uses the exact same
// kid a verifier fetching this key's JWKS entry will look up.
type SigningKey struct {
	Private *rsa.PrivateKey
	Kid     string
}

// LoadSigningKey reads and parses an RSA private key from path (PEM,
// PKCS#1 or PKCS#8 — either is accepted since key-generation tooling
// varies in which it emits, and this isn't a place to add friction over).
//
// kid is derived deterministically from the public key's own bytes
// (SHA-256 of its PKIX DER encoding, base64url, first 16 characters) —
// not a separately configured value. This means a key rotation (a new
// JWT_SIGNING_KEY_FILE, LT-32's existing mechanism) automatically
// produces a new kid with no separate "bump the kid" step to forget, and
// two different key files can never silently collide on the same kid.
func LoadSigningKey(path string) (*SigningKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("api: reading signing key %q: %w", path, err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("api: signing key %q is not valid PEM", path)
	}

	priv, err := parseRSAPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("api: parsing signing key %q: %w", path, err)
	}

	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("api: marshaling public key for kid derivation: %w", err)
	}
	sum := sha256.Sum256(pubDER)
	kid := base64.RawURLEncoding.EncodeToString(sum[:])[:16]

	return &SigningKey{Private: priv, Kid: kid}, nil
}

func parseRSAPrivateKey(der []byte) (*rsa.PrivateKey, error) {
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	keyAny, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("not a PKCS#1 or PKCS#8 RSA private key: %w", err)
	}
	rsaKey, ok := keyAny.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("PKCS#8 key is not an RSA private key")
	}
	return rsaKey, nil
}

// JWKS renders this key's public component as a single-key RFC 7517
// JSON Web Key Set document — the exact shape internal/api/jwks.go's
// client-side parser (parseJWKS) expects, since that's the contract
// this handler must satisfy for LT-40's verifier to consume it.
func (k *SigningKey) JWKS() jwksResponse {
	pub := k.Private.PublicKey
	return jwksResponse{
		Keys: []jwk{{
			Kty: "RSA",
			Kid: k.Kid,
			Alg: jwtAlgorithm,
			Use: "sig",
			N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			E:   base64.RawURLEncoding.EncodeToString(bigEndianExponentBytes(pub.E)),
		}},
	}
}

// bigEndianExponentBytes encodes e as the minimal big-endian byte string
// a JWK's "e" member expects (RFC 7518 §6.3.1) — mirrors the same
// encoding this package's own tests already use to build a JWK from an
// *rsa.PublicKey (internal/api/jwks_test.go's bigEndianExponent), kept as
// its own small function here since production code (unlike a test
// helper) shouldn't reach across a _test.go file.
func bigEndianExponentBytes(e int) []byte {
	b := []byte{byte(e >> 16), byte(e >> 8), byte(e)}
	i := 0
	for i < len(b)-1 && b[i] == 0 {
		i++
	}
	return b[i:]
}
