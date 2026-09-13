package onboarding

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"loginid-takehome/internal/connector"
)

// fakeTokenClient is a minimal TokenClient fake, per
// decisions/test-double-strategy.md — satisfies only what this
// package's tests actually exercise, not a general-purpose mock.
type fakeTokenClient struct {
	mu        sync.Mutex
	calls     int
	token     string
	expiresIn time.Duration
	err       error
}

func (f *fakeTokenClient) FetchToken(_ context.Context, _, _ string) (string, time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return "", 0, f.err
	}
	return f.token, f.expiresIn, nil
}

func (f *fakeTokenClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestTokenSource_CachesAcrossCalls(t *testing.T) {
	fake := &fakeTokenClient{token: "tok-1", expiresIn: 10 * time.Minute}
	ts, err := NewTokenSource(fake, "client-id", "client-secret")
	if err != nil {
		t.Fatalf("NewTokenSource: %v", err)
	}

	for i := 0; i < 3; i++ {
		tok, err := ts.Token(context.Background())
		if err != nil {
			t.Fatalf("Token() call %d: %v", i, err)
		}
		if tok != "tok-1" {
			t.Errorf("Token() = %q, want %q", tok, "tok-1")
		}
	}
	if fake.callCount() != 1 {
		t.Errorf("FetchToken called %d times, want 1 — a cached token must be reused across calls, not re-minted per call (this story's own caching criterion)", fake.callCount())
	}
}

func TestTokenSource_RefreshesNearExpiry(t *testing.T) {
	fake := &fakeTokenClient{token: "tok-1", expiresIn: refreshMargin - time.Millisecond}
	ts, err := NewTokenSource(fake, "client-id", "client-secret")
	if err != nil {
		t.Fatalf("NewTokenSource: %v", err)
	}

	if _, err := ts.Token(context.Background()); err != nil {
		t.Fatalf("Token() first call: %v", err)
	}
	fake.token = "tok-2"
	tok, err := ts.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() second call: %v", err)
	}
	if tok != "tok-2" {
		t.Errorf("Token() = %q, want %q — a token within refreshMargin of its stated expiry must be refreshed proactively", tok, "tok-2")
	}
	if fake.callCount() != 2 {
		t.Errorf("FetchToken called %d times, want 2", fake.callCount())
	}
}

func TestTokenSource_InvalidateForcesRefresh(t *testing.T) {
	fake := &fakeTokenClient{token: "tok-1", expiresIn: 10 * time.Minute}
	ts, err := NewTokenSource(fake, "client-id", "client-secret")
	if err != nil {
		t.Fatalf("NewTokenSource: %v", err)
	}
	if _, err := ts.Token(context.Background()); err != nil {
		t.Fatalf("Token() first call: %v", err)
	}

	ts.Invalidate()
	fake.token = "tok-2"
	tok, err := ts.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() after Invalidate: %v", err)
	}
	if tok != "tok-2" {
		t.Errorf("Token() after Invalidate = %q, want a freshly-fetched %q", tok, "tok-2")
	}
	if fake.callCount() != 2 {
		t.Errorf("FetchToken called %d times after Invalidate, want 2", fake.callCount())
	}
}

func TestNewTokenSource_RequiresCredentials(t *testing.T) {
	fake := &fakeTokenClient{}
	cases := []struct {
		name, id, secret string
	}{
		{name: "empty id", id: "", secret: "secret"},
		{name: "empty secret", id: "id", secret: ""},
		{name: "both empty", id: "", secret: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewTokenSource(fake, c.id, c.secret); err == nil {
				t.Errorf("NewTokenSource(%q, %q) = nil error, want an error", c.id, c.secret)
			}
		})
	}
}

// fakeConnectorClient is a minimal ConnectorClient fake recording every
// call's arguments, so tests can assert exactly which token/credential
// values reached it and in what order — including whether a vendor
// token was ever passed to the wrong call.
type fakeConnectorClient struct {
	mu sync.Mutex

	authCalls     []authCall
	identityCalls []identityCall

	authResult     string
	authErr        error
	identityResult connector.Identity
	identityErr    error
}

type authCall struct {
	ourToken, vendorUsername, vendorPassword string
}

type identityCall struct {
	ourToken, vendorAccessToken, phone, name string
}

func (f *fakeConnectorClient) Auth(_ context.Context, ourToken, vendorUsername, vendorPassword string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.authCalls = append(f.authCalls, authCall{ourToken, vendorUsername, vendorPassword})
	return f.authResult, f.authErr
}

func (f *fakeConnectorClient) Identity(_ context.Context, ourToken, vendorAccessToken, phone, name string) (connector.Identity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.identityCalls = append(f.identityCalls, identityCall{ourToken, vendorAccessToken, phone, name})
	return f.identityResult, f.identityErr
}

func newTestService(t *testing.T, tokenClient TokenClient, connClient ConnectorClient) *Service {
	t.Helper()
	tokens, err := NewTokenSource(tokenClient, "client-id", "client-secret")
	if err != nil {
		t.Fatalf("NewTokenSource: %v", err)
	}
	return &Service{Tokens: tokens, Connector: connClient}
}

func TestService_VerifyIdentity_HappyPath(t *testing.T) {
	fakeTok := &fakeTokenClient{token: "our-token", expiresIn: 10 * time.Minute}
	fakeConn := &fakeConnectorClient{
		authResult: "vendor-token",
		identityResult: connector.Identity{
			Name: "Jane Doe", Phone: "+15551234567", StreetAddress: "1 Main St",
			Locality: "Springfield", Region: "IL", PostalCode: "62704", Country: "US",
		},
	}
	svc := newTestService(t, fakeTok, fakeConn)

	identity, err := svc.VerifyIdentity(context.Background(), "vendor-user", "vendor-pass", "+15551234567", "Jane Doe")
	if err != nil {
		t.Fatalf("VerifyIdentity: %v", err)
	}
	if identity != fakeConn.identityResult {
		t.Errorf("identity = %+v, want %+v", identity, fakeConn.identityResult)
	}

	if len(fakeConn.authCalls) != 1 {
		t.Fatalf("Auth called %d times, want 1", len(fakeConn.authCalls))
	}
	if fakeConn.authCalls[0].ourToken != "our-token" || fakeConn.authCalls[0].vendorUsername != "vendor-user" || fakeConn.authCalls[0].vendorPassword != "vendor-pass" {
		t.Errorf("Auth called with %+v, want ourToken=our-token vendorUsername=vendor-user vendorPassword=vendor-pass", fakeConn.authCalls[0])
	}

	if len(fakeConn.identityCalls) != 1 {
		t.Fatalf("Identity called %d times, want 1", len(fakeConn.identityCalls))
	}
	// The vendor access token Auth returned must reach Identity — and
	// ONLY Identity, never anywhere else (attack-tree leaf 4's control,
	// applied to api-service's own side of this call per this story's
	// own criterion).
	if fakeConn.identityCalls[0].vendorAccessToken != "vendor-token" {
		t.Errorf("Identity called with vendorAccessToken=%q, want %q", fakeConn.identityCalls[0].vendorAccessToken, "vendor-token")
	}
	if fakeConn.identityCalls[0].ourToken != "our-token" {
		t.Errorf("Identity called with ourToken=%q, want %q", fakeConn.identityCalls[0].ourToken, "our-token")
	}
}

func TestService_VerifyIdentity_OwnTokenAcquisitionFails(t *testing.T) {
	fakeTok := &fakeTokenClient{err: errors.New("issuer rejected our client credential")}
	fakeConn := &fakeConnectorClient{}
	svc := newTestService(t, fakeTok, fakeConn)

	_, err := svc.VerifyIdentity(context.Background(), "u", "p", "+1", "n")
	if !errors.Is(err, ErrIdentityVerificationUnavailable) {
		t.Errorf("VerifyIdentity error = %v, want ErrIdentityVerificationUnavailable", err)
	}
	if len(fakeConn.authCalls) != 0 {
		t.Errorf("Auth called %d times, want 0 — must never call the connector at all if our own token couldn't be obtained", len(fakeConn.authCalls))
	}
}

func TestService_VerifyIdentity_ConnectorUnreachable(t *testing.T) {
	fakeTok := &fakeTokenClient{token: "our-token", expiresIn: 10 * time.Minute}
	fakeConn := &fakeConnectorClient{authErr: errors.New("dial tcp: connection refused")}
	svc := newTestService(t, fakeTok, fakeConn)

	_, err := svc.VerifyIdentity(context.Background(), "u", "p", "+1", "n")
	if !errors.Is(err, ErrIdentityVerificationUnavailable) {
		t.Errorf("VerifyIdentity error = %v, want ErrIdentityVerificationUnavailable — never the raw connector error", err)
	}
	// connectorMaxRetries+1 attempts, no more, no fewer — bounded, not
	// an open-ended retry loop.
	if len(fakeConn.authCalls) != connectorMaxRetries+1 {
		t.Errorf("Auth called %d times, want %d", len(fakeConn.authCalls), connectorMaxRetries+1)
	}
}

func TestService_VerifyIdentity_VendorCredentialRejected(t *testing.T) {
	fakeTok := &fakeTokenClient{token: "our-token", expiresIn: 10 * time.Minute}
	fakeConn := &fakeConnectorClient{authErr: ErrVendorCredentialRejected}
	svc := newTestService(t, fakeTok, fakeConn)

	_, err := svc.VerifyIdentity(context.Background(), "u", "wrong-pass", "+1", "n")
	if !errors.Is(err, ErrVendorCredentialRejected) {
		t.Errorf("VerifyIdentity error = %v, want ErrVendorCredentialRejected", err)
	}
	// A 401 is ambiguous at this seam (our own token vs. the vendor's —
	// see VerifyIdentity's own doc comment), so VerifyIdentity always
	// invalidates and retries the whole flow exactly once with a fresh
	// own-token before concluding the vendor credential itself was
	// rejected — even a genuinely-wrong vendor credential costs exactly
	// 2 connector calls, never more (callAuth's own inner retry loop
	// does NOT retry a 401 on its own, which is what bounds this at 2
	// rather than connectorMaxRetries+1 per attempt).
	if len(fakeConn.authCalls) != 2 {
		t.Errorf("Auth called %d times, want 2 (initial + one ambiguous-401 retry, never more)", len(fakeConn.authCalls))
	}
}

func TestService_VerifyIdentity_RetriesOnceAfterAmbiguous401(t *testing.T) {
	fakeTok := &fakeTokenClient{token: "our-token-1", expiresIn: 10 * time.Minute}
	fakeConn := &fakeConnectorClient{authResult: "vendor-token"}
	svc := newTestService(t, fakeTok, fakeConn)

	// First Auth call rejected (ambiguous: could be OUR token or the
	// vendor's) — the fake token client then returns a different token
	// on its second FetchToken call, simulating a real refresh.
	callNum := 0
	svc2 := &Service{
		Tokens: svc.Tokens,
		Connector: &sequencedConnectorClient{
			fake: fakeConn,
			onAuthCall: func(n int) error {
				callNum = n
				if n == 1 {
					return ErrVendorCredentialRejected
				}
				return nil
			},
		},
	}
	fakeTok.token = "our-token-2"

	identity, err := svc2.VerifyIdentity(context.Background(), "u", "p", "+1", "n")
	if err != nil {
		t.Fatalf("VerifyIdentity: %v", err)
	}
	_ = identity
	if callNum != 2 {
		t.Fatalf("expected exactly 2 Auth attempts (initial + one retry after invalidation), got callNum=%d", callNum)
	}
}

// sequencedConnectorClient wraps fakeConnectorClient's Identity behavior
// while letting Auth's per-call outcome be scripted by call number —
// needed only by TestService_VerifyIdentity_RetriesOnceAfterAmbiguous401,
// which must return different results on the 1st vs. 2nd Auth call.
type sequencedConnectorClient struct {
	fake       *fakeConnectorClient
	calls      int
	onAuthCall func(callNumber int) error
}

func (s *sequencedConnectorClient) Auth(ctx context.Context, ourToken, vendorUsername, vendorPassword string) (string, error) {
	s.calls++
	if err := s.onAuthCall(s.calls); err != nil {
		return "", err
	}
	return s.fake.Auth(ctx, ourToken, vendorUsername, vendorPassword)
}

func (s *sequencedConnectorClient) Identity(ctx context.Context, ourToken, vendorAccessToken, phone, name string) (connector.Identity, error) {
	return s.fake.Identity(ctx, ourToken, vendorAccessToken, phone, name)
}
