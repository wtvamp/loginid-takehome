package connector

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
)

// ErrVendorAuthFailed is returned by StubVendorClient.Authenticate for
// any unrecognized username/password pair — deliberately a single
// sentinel, not distinguishing "unknown username" from "wrong password",
// matching connector-security.md §4's own rule for a real vendor
// integration (this stub follows the same discipline it will eventually
// front for).
var ErrVendorAuthFailed = errors.New("connector: vendor authentication failed")

// ErrVendorIdentityNotFound is returned by StubVendorClient.FetchIdentity
// for an unrecognized or expired token, or a phone/name pair with no
// matching stub record.
var ErrVendorIdentityNotFound = errors.New("connector: vendor identity lookup failed")

// StubVendorClient is a self-contained, in-process fake vendor —
// LT-41's own non-goal list explicitly excludes real ABC/XYZ
// integration, so this is what cmd/idp-connector wires by default
// (IDP_ABC_BASE_URL unset) to demonstrate the connector's full /auth ->
// /identity flow without a real third party. Deterministic and
// data-free beyond what's seeded via Seed — no real end-user data ever
// lives in this type.
type StubVendorClient struct {
	mu          sync.Mutex
	credentials map[string]string   // username -> password
	identities  map[string]Identity // username -> identity
	tokens      map[string]string   // token -> username
}

// NewStubVendorClient returns an empty stub — call Seed to add a
// demo/test credential.
func NewStubVendorClient() *StubVendorClient {
	return &StubVendorClient{
		credentials: make(map[string]string),
		identities:  make(map[string]Identity),
		tokens:      make(map[string]string),
	}
}

// Seed registers one fake vendor end-user: username/password
// authenticate successfully, and the resulting token resolves to
// identity via FetchIdentity — regardless of what phone/name the caller
// actually requested, matching a real vendor's own binding-to-the-token
// behavior (connector-security.md §6's stated assumption).
func (s *StubVendorClient) Seed(username, password string, identity Identity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.credentials[username] = password
	s.identities[username] = identity
}

func (s *StubVendorClient) Authenticate(_ context.Context, username, password string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	want, ok := s.credentials[username]
	if !ok || want != password {
		return "", ErrVendorAuthFailed
	}
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	s.tokens[token] = username
	return token, nil
}

func (s *StubVendorClient) FetchIdentity(_ context.Context, accessToken, _, _ string) (Identity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	username, ok := s.tokens[accessToken]
	if !ok {
		return Identity{}, ErrVendorIdentityNotFound
	}
	identity, ok := s.identities[username]
	if !ok {
		return Identity{}, ErrVendorIdentityNotFound
	}
	return identity, nil
}

func randomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
