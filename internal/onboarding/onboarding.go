// Package onboarding implements LT-33: api-service's own outbound call
// to cmd/idp-connector on an onboarding flow's behalf — the one story
// that makes Q2 (api-service) and Q3 (idp-connector) one system, per
// refinement/LT-33.md.
//
// This is a service-layer mediator, not a new public HTTP endpoint: no
// assignment question asks for an onboarding endpoint, and this story's
// own non-goals rule out a new client-facing surface. Acceptance (PO
// review script item 7) is a unit/integration test exercising Service
// against a fake ConnectorClient/TokenClient, not a live-URL check.
package onboarding

import (
	"context"
	"errors"
	"log"
	"time"

	"loginid-takehome/internal/connector"
)

// ErrIdentityVerificationUnavailable is returned when the connector
// couldn't be reached at all (dial/timeout/5xx after retries) or
// api-service's own connector-scoped token couldn't be obtained — an
// availability problem, never internal detail, per Marcus Ilori's (02)
// ruling on this story's connector-unreachable failure mode.
var ErrIdentityVerificationUnavailable = errors.New("onboarding: identity verification temporarily unavailable")

// ErrVendorCredentialRejected is returned when the connector was
// reached fine but rejected the end-user's vendor username/password (a
// genuine 401 from /auth) — distinguishable from
// ErrIdentityVerificationUnavailable so an onboarding flow can tell a
// user to recheck their vendor login rather than "try again later," but
// carrying no finer detail than that: idp-connector's own /auth already
// collapses "unknown username" vs "wrong password" vs "vendor
// unreachable" into one generic rejection (connector-security.md §4),
// and this package does not attempt to recover any of that distinction.
var ErrVendorCredentialRejected = errors.New("onboarding: vendor credential rejected")

// TokenClient is the seam this package uses to obtain api-service's own
// connector-scoped client-credentials access token — satisfied by
// HTTPTokenClient (token.go), or a fake in tests.
type TokenClient interface {
	// FetchToken exchanges clientID/clientSecret for a fresh
	// client-credentials token, per RFC 6749 §4.4. Returns the token's
	// stated lifetime as a duration (not an absolute time), since
	// TokenSource is the one thing responsible for turning that into an
	// expiry deadline.
	FetchToken(ctx context.Context, clientID, clientSecret string) (accessToken string, expiresIn time.Duration, err error)
}

// ConnectorClient is api-service's outbound seam to cmd/idp-connector's
// own /auth and /identity endpoints (LT-41). ourToken authenticates
// api-service itself to the connector (aud: idp-connector-service,
// scope: connector:identity-lookup); vendorAccessToken is the end
// user's vendor token, obtained from Auth and passed straight through to
// Identity — Service.VerifyIdentity is the only place that ever holds
// it, and only for the span of one call.
type ConnectorClient interface {
	// Auth returns ErrVendorCredentialRejected on the connector's own
	// generic 401 (vendor username/password rejected), or
	// ErrIdentityVerificationUnavailable on anything else (network
	// error, non-2xx/401 status, malformed response) — never richer
	// detail than that.
	Auth(ctx context.Context, ourToken, vendorUsername, vendorPassword string) (vendorAccessToken string, err error)

	// Identity returns the same two-error contract as Auth.
	// ErrVendorCredentialRejected here covers the connector rejecting
	// EITHER our own token (early/unexpected expiry — a 401 that looks
	// identical to a rejected vendor token from this seam's point of
	// view, by idp-connector's own deliberate design) or the vendor
	// token; Service.VerifyIdentity treats a 401 here as cause to
	// invalidate the cached own-token and retry once with a fresh one
	// before concluding the vendor token itself was the problem.
	Identity(ctx context.Context, ourToken, vendorAccessToken, phone, name string) (connector.Identity, error)
}

// connectorCallTimeout bounds every outbound call to idp-connector,
// independent of whatever deadline the caller's ctx carries — the same
// belt-and-braces pattern internal/connector's own HTTPVendorClient
// uses for its calls to the vendor.
const connectorCallTimeout = 10 * time.Second

// connectorMaxRetries/connectorRetryBackoff bound the retry loop this
// story's "connector unreachable" criterion requires: a small, fixed
// number of attempts with backoff, never an open-ended retry loop a
// caller could turn into a self-inflicted flood against idp-connector.
const (
	connectorMaxRetries   = 2
	connectorRetryBackoff = 250 * time.Millisecond
)

// Service mediates one onboarding attempt end to end: obtain our own
// connector-scoped token, call the connector's /auth with the end
// user's vendor credential, then its /identity with the resulting
// vendor token — never persisting or logging the vendor password or
// vendor access token at any point.
type Service struct {
	Tokens    *TokenSource
	Connector ConnectorClient
}

// VerifyIdentity is this package's one entry point. vendorAccessToken
// never escapes attemptVerifyIdentity: it's read from callAuth's return
// value and passed straight into callIdentity as a parameter, never
// assigned to any field of Service or any wider-scoped variable, and
// never logged — attack-tree leaf 4's control (connector-security.md),
// applied symmetrically to api-service per this story's own criterion.
func (s *Service) VerifyIdentity(ctx context.Context, vendorUsername, vendorPassword, phone, name string) (connector.Identity, error) {
	identity, err := s.attemptVerifyIdentity(ctx, vendorUsername, vendorPassword, phone, name)
	if errors.Is(err, ErrVendorCredentialRejected) {
		// A 401 from the connector is ambiguous from this seam's own
		// point of view: it means either OUR token or the vendor's was
		// rejected — idp-connector's own uniform-response-content
		// design (connector-security.md §1/F3) gives no way to tell
		// which. Invalidate the cached own-token and retry the WHOLE
		// flow exactly once with a freshly-minted one before concluding
		// the vendor credential itself was actually the problem — the
		// "refreshing ... on a 401" half of this story's caching
		// criterion.
		s.Tokens.Invalidate()
		identity, err = s.attemptVerifyIdentity(ctx, vendorUsername, vendorPassword, phone, name)
	}
	return identity, err
}

func (s *Service) attemptVerifyIdentity(ctx context.Context, vendorUsername, vendorPassword, phone, name string) (connector.Identity, error) {
	ourToken, err := s.Tokens.Token(ctx)
	if err != nil {
		// TokenSource.Token already logged this as its own distinct
		// failure class (a config/credential problem, not connector
		// downtime) — nothing further to add here.
		return connector.Identity{}, ErrIdentityVerificationUnavailable
	}

	vendorAccessToken, err := s.callAuth(ctx, ourToken, vendorUsername, vendorPassword)
	if err != nil {
		return connector.Identity{}, err
	}
	return s.callIdentity(ctx, ourToken, vendorAccessToken, phone, name)
}

func (s *Service) callAuth(ctx context.Context, ourToken, vendorUsername, vendorPassword string) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, connectorCallTimeout)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt <= connectorMaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(connectorRetryBackoff):
			case <-callCtx.Done():
				return "", ErrIdentityVerificationUnavailable
			}
		}
		token, err := s.Connector.Auth(callCtx, ourToken, vendorUsername, vendorPassword)
		if err == nil {
			return token, nil
		}
		if errors.Is(err, ErrVendorCredentialRejected) {
			// Not transient — retrying an actually-rejected credential
			// against an unreachable connector wouldn't help, and
			// retrying it against a reachable one would just waste
			// idp-connector's own rate-limit budget on a doomed
			// request. Surface immediately.
			return "", err
		}
		lastErr = err
	}
	log.Printf("onboarding: idp-connector /auth unreachable after %d attempts: %v", connectorMaxRetries+1, lastErr)
	return "", ErrIdentityVerificationUnavailable
}

func (s *Service) callIdentity(ctx context.Context, ourToken, vendorAccessToken, phone, name string) (connector.Identity, error) {
	callCtx, cancel := context.WithTimeout(ctx, connectorCallTimeout)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt <= connectorMaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(connectorRetryBackoff):
			case <-callCtx.Done():
				return connector.Identity{}, ErrIdentityVerificationUnavailable
			}
		}
		identity, err := s.Connector.Identity(callCtx, ourToken, vendorAccessToken, phone, name)
		if err == nil {
			return identity, nil
		}
		if errors.Is(err, ErrVendorCredentialRejected) {
			return connector.Identity{}, err
		}
		lastErr = err
	}
	log.Printf("onboarding: idp-connector /identity unreachable after %d attempts: %v", connectorMaxRetries+1, lastErr)
	return connector.Identity{}, ErrIdentityVerificationUnavailable
}
