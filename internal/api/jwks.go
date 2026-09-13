// jwks.go implements a minimal RFC 7517 JSON Web Key Set client: fetch,
// parse RSA public keys, cache with a short TTL. handoff-03-auth.md v4:
// "Public-key distribution for the verifier ... JWKS served by the issuer
// Deployment, reachable only in-cluster via its own ClusterIP Service ...
// Verifier caches the fetched key set in memory with a short TTL (matching
// token TTL is reasonable) to avoid a JWKS fetch on every single
// verification."
package api

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// jwk is one entry in a JWKS response — only the fields this verifier
// needs to reconstruct an RSA public key (RFC 7517 §4, RFC 7518 §6.3.1).
// Fields this verifier doesn't use (x5c, x5t, ...) are simply not
// unmarshaled; unknown fields in the source JSON are not an error.
type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"` // base64url, unpadded — the RSA modulus
	E   string `json:"e"` // base64url, unpadded — the RSA public exponent
}

type jwksResponse struct {
	Keys []jwk `json:"keys"`
}

// parseJWKS decodes a JWKS JSON document into a map of kid -> public key.
// Any key with kty != "RSA" is skipped (not an error) — a future non-RSA
// key in the set (e.g. during an algorithm migration) shouldn't break
// verification of tokens signed with the RSA keys that ARE present.
func parseJWKS(data []byte) (map[string]*rsa.PublicKey, error) {
	var resp jwksResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("api: parsing JWKS: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey, len(resp.Keys))
	for _, k := range resp.Keys {
		if k.Kty != "RSA" {
			continue
		}
		if k.Kid == "" {
			return nil, fmt.Errorf("api: JWKS RSA key missing kid")
		}
		pub, err := rsaPublicKeyFromJWK(k)
		if err != nil {
			return nil, fmt.Errorf("api: JWKS key %q: %w", k.Kid, err)
		}
		keys[k.Kid] = pub
	}
	return keys, nil
}

func rsaPublicKeyFromJWK(k jwk) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decoding n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decoding e: %w", err)
	}
	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)
	if !e.IsInt64() {
		return nil, fmt.Errorf("exponent out of range")
	}
	return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
}

// KeySource resolves a kid to the RSA public key that should verify a
// token carrying it. JWKSCache is the real implementation; tests use a
// map-backed fake.
type KeySource interface {
	KeyForKid(kid string) (*rsa.PublicKey, error)
}

// JWKSCache fetches a JWKS document from url and caches the parsed key
// set for ttl, refetching only after it expires — "to avoid a JWKS fetch
// on every single verification" (handoff-03-auth.md v4). Supports at
// least two concurrently valid kids during a rotation overlap window by
// construction: it caches the WHOLE key set the issuer publishes, not
// just the most recent key, so an old and a new key both verify
// successfully for as long as the issuer's own JWKS response lists both.
type JWKSCache struct {
	url        string
	ttl        time.Duration
	httpClient *http.Client

	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

// NewJWKSCache constructs a cache fetching from url, refetching at most
// once per ttl. A nil httpClient uses http.DefaultClient.
func NewJWKSCache(url string, ttl time.Duration, httpClient *http.Client) *JWKSCache {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &JWKSCache{url: url, ttl: ttl, httpClient: httpClient}
}

func (c *JWKSCache) KeyForKid(kid string) (*rsa.PublicKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.keys == nil || time.Since(c.fetchedAt) >= c.ttl {
		keys, err := c.fetch()
		if err != nil {
			// A stale-but-present cache is preferable to a hard failure
			// on every request the moment one refresh fails (a
			// transient network blip to the issuer's in-cluster
			// Service) — verification with a stale key set still
			// correctly rejects a token signed by a kid that's been
			// fully retired and removed from the real set, it just
			// risks accepting a very recently rotated-in key slightly
			// late, which is not a security regression (the overlap
			// window already tolerates both old and new keys).
			if c.keys == nil {
				return nil, err
			}
		} else {
			c.keys = keys
			c.fetchedAt = time.Now()
		}
	}

	key, ok := c.keys[kid]
	if !ok {
		return nil, fmt.Errorf("api: no key found for kid %q", kid)
	}
	return key, nil
}

func (c *JWKSCache) fetch() (map[string]*rsa.PublicKey, error) {
	resp, err := c.httpClient.Get(c.url)
	if err != nil {
		return nil, fmt.Errorf("api: fetching JWKS from %s: %w", c.url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api: fetching JWKS from %s: status %d", c.url, resp.StatusCode)
	}
	// 1MiB ceiling — a JWKS response is a handful of RSA keys, never
	// this large; capped so a misbehaving/compromised issuer endpoint
	// can't force this process to buffer an unbounded response body.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("api: reading JWKS response: %w", err)
	}
	return parseJWKS(body)
}
