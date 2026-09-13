package onboarding

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// tokenCallTimeout bounds every call to the issuer's /auth/token,
// independent of whatever deadline the caller's ctx carries — same
// belt-and-braces pattern as connectorCallTimeout.
const tokenCallTimeout = 10 * time.Second

// refreshMargin is how far before a token's stated expiry TokenSource
// proactively treats it as stale and refreshes, rather than risk
// presenting an already-(or nearly-)expired token to the connector —
// standard OAuth2 client-credentials practice, per this story's own
// caching criterion (Marcus Ilori's (02) correction).
const refreshMargin = 30 * time.Second

// defaultTokenHTTPClient is a bounded-pool client, the same discipline
// internal/connector's own outbound clients use.
var defaultTokenHTTPClient = &http.Client{
	Timeout: tokenCallTimeout,
	Transport: &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
		MaxConnsPerHost:     10,
		IdleConnTimeout:     30 * time.Second,
	},
}

// HTTPTokenClient calls a real OAuth2 client-credentials token endpoint
// (this project's own POST /auth/token, LT-51) over RFC 6749 §4.4's wire
// format: HTTP Basic client auth, form-encoded body, no scope parameter
// (the client's own single granted scope is issued when scope is
// omitted, per tokenhandler.go's own doc comment).
type HTTPTokenClient struct {
	tokenURL   string
	httpClient *http.Client
}

// NewHTTPTokenClient requires a non-empty tokenURL. httpClient defaults
// to defaultTokenHTTPClient (bounded pool + timeout) if nil.
func NewHTTPTokenClient(tokenURL string, httpClient *http.Client) (*HTTPTokenClient, error) {
	if tokenURL == "" {
		return nil, fmt.Errorf("onboarding: token URL must not be empty")
	}
	if _, err := url.Parse(tokenURL); err != nil {
		return nil, fmt.Errorf("onboarding: parsing token URL: %w", err)
	}
	if httpClient == nil {
		httpClient = defaultTokenHTTPClient
	}
	return &HTTPTokenClient{tokenURL: tokenURL, httpClient: httpClient}, nil
}

type tokenSuccessBody struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

func (c *HTTPTokenClient) FetchToken(ctx context.Context, clientID, clientSecret string) (string, time.Duration, error) {
	form := strings.NewReader(url.Values{"grant_type": {"client_credentials"}}.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, form)
	if err != nil {
		return "", 0, fmt.Errorf("onboarding: building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Never wrap the raw dial/DNS error into the caller-visible
		// error — this seam's contract, matching HTTPVendorClient's own
		// convention, is "succeeded, or a generic failure," never richer
		// detail than that.
		return "", 0, fmt.Errorf("onboarding: token endpoint unreachable")
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Deliberately not logging or returning the response body —
		// same "don't echo a vendor/issuer error payload back" discipline
		// this project applies everywhere else it talks to another
		// service (connector-security.md §3).
		return "", 0, fmt.Errorf("onboarding: token endpoint returned status %d", resp.StatusCode)
	}

	var body tokenSuccessBody
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&body); err != nil || body.AccessToken == "" || body.ExpiresIn <= 0 {
		return "", 0, fmt.Errorf("onboarding: malformed token response")
	}
	return body.AccessToken, time.Duration(body.ExpiresIn) * time.Second, nil
}

// TokenSource caches api-service's own connector-scoped client-
// credentials token in memory and reuses it across calls until near
// expiry, refreshing shortly before TTL or on an explicit Invalidate
// (called when the connector itself rejects the token with a 401) —
// per-replica only, no shared store, per this story's own caching
// criterion (Marcus Ilori's (02) correction to this refinement's
// original draft: this is our own client's token, standard OAuth2
// client-credentials practice, a different risk class from the
// end-user vendor token which is correctly never cached at all).
type TokenSource struct {
	client       TokenClient
	clientID     string
	clientSecret string

	mu     sync.Mutex
	cached string
	expiry time.Time
}

// NewTokenSource requires non-empty clientID/clientSecret — this
// story's own credential-holding criterion; a construction-time error
// here is what api-service's caller (cmd/api-service/main.go) uses to
// decide whether the onboarding capability can be wired up at all,
// rather than crash-looping the whole process over a capability that,
// unlike auth verification itself, has an existing precedent in this
// codebase for staying optional (NewVerifierRouter's own nil-repo
// pattern: fail that one capability closed, not the whole binary).
func NewTokenSource(client TokenClient, clientID, clientSecret string) (*TokenSource, error) {
	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("onboarding: CONNECTOR_CLIENT_ID and CONNECTOR_CLIENT_SECRET (or their _FILE variants) must both be set")
	}
	return &TokenSource{client: client, clientID: clientID, clientSecret: clientSecret}, nil
}

// Token returns a valid connector-scoped access token, from cache when
// one exists and isn't within refreshMargin of its stated expiry,
// otherwise by fetching a fresh one. Failure here is logged as its own
// distinct class — a config/credential problem (this binary's own
// client_id/secret rejected, or the token endpoint itself unreachable)
// — separate from a downstream connector-unreachable failure, per this
// story's own criterion that the two must not be lumped together.
func (s *TokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	if s.cached != "" && time.Now().Before(s.expiry.Add(-refreshMargin)) {
		tok := s.cached
		s.mu.Unlock()
		return tok, nil
	}
	s.mu.Unlock()

	callCtx, cancel := context.WithTimeout(ctx, tokenCallTimeout)
	defer cancel()
	tok, expiresIn, err := s.client.FetchToken(callCtx, s.clientID, s.clientSecret)
	if err != nil {
		log.Printf("onboarding: failed to obtain api-service's own connector-scoped token (own-credential/config problem, not connector downtime): %v", err)
		return "", err
	}

	s.mu.Lock()
	s.cached = tok
	s.expiry = time.Now().Add(expiresIn)
	s.mu.Unlock()
	return tok, nil
}

// Invalidate drops the cached token, forcing the next Token call to
// fetch a fresh one — called when the connector rejects the cached
// token with a 401 (early/unexpected expiry), per this story's own
// "refreshing before TTL or on a 401" criterion.
func (s *TokenSource) Invalidate() {
	s.mu.Lock()
	s.cached = ""
	s.mu.Unlock()
}
