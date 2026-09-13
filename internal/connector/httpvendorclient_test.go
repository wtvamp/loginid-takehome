package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestNewHTTPVendorClient_RejectsNonHTTPS(t *testing.T) {
	if _, err := NewHTTPVendorClient("http://vendor.example.com", nil); err == nil {
		t.Errorf("NewHTTPVendorClient accepted a plain http:// base URL, want an error (connector-security.md §2: TLS only)")
	}
	if _, err := NewHTTPVendorClient("not-a-url", nil); err == nil {
		t.Errorf("NewHTTPVendorClient accepted a garbage base URL, want an error")
	}
}

// TestHTTPVendorClient_FetchIdentity_FreshAuthorizationHeaderPerRequest
// is attack-tree leaf 1's own named enforcement site (refinement/LT-41.md):
// two sequential calls through the SAME pooled *http.Client, each
// presenting a DIFFERENT vendor token, must each produce an outbound
// Authorization header matching ITS OWN token — never the previous
// call's, which a client that held a shared/default header (instead of
// building it fresh per call) could otherwise leak across requests via
// connection reuse.
func TestHTTPVendorClient_FetchIdentity_FreshAuthorizationHeaderPerRequest(t *testing.T) {
	var mu sync.Mutex
	var observedHeaders []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		observedHeaders = append(observedHeaders, r.Header.Get("Authorization"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(vendorIdentityResponse{Name: "ok"})
	}))
	defer srv.Close()

	// Construct directly (bypassing NewHTTPVendorClient's https-only
	// check, which httptest.Server's plain-http URL would otherwise
	// fail) — this file is in the same package, so it can set the
	// unexported fields directly rather than needing a second,
	// test-only constructor in non-test code.
	client := &HTTPVendorClient{baseURL: srv.URL, httpClient: srv.Client()}

	if _, err := client.FetchIdentity(context.Background(), "token-A", "+15555550100", "Jane"); err != nil {
		t.Fatalf("first FetchIdentity call: %v", err)
	}
	if _, err := client.FetchIdentity(context.Background(), "token-B", "+15555550100", "Jane"); err != nil {
		t.Fatalf("second FetchIdentity call: %v", err)
	}

	if len(observedHeaders) != 2 {
		t.Fatalf("server observed %d requests, want 2", len(observedHeaders))
	}
	if observedHeaders[0] != "Bearer token-A" {
		t.Errorf("first request's Authorization = %q, want %q", observedHeaders[0], "Bearer token-A")
	}
	if observedHeaders[1] != "Bearer token-B" {
		t.Errorf("second request's Authorization = %q, want %q — must not inherit the first call's token via connection reuse", observedHeaders[1], "Bearer token-B")
	}
}

func TestHTTPVendorClient_Authenticate_SendsCredentialsAndReturnsToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body vendorAuthRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("server: decoding request: %v", err)
		}
		if body.Username != "u" || body.Password != "p" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(vendorAuthResponse{AccessToken: "minted-token"})
	}))
	defer srv.Close()

	client := &HTTPVendorClient{baseURL: srv.URL, httpClient: srv.Client()}
	token, err := client.Authenticate(context.Background(), "u", "p")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if token != "minted-token" {
		t.Errorf("token = %q, want minted-token", token)
	}
}

func TestHTTPVendorClient_Authenticate_VendorRejects_GenericError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","hint":"password field was 'hunter2'"}`))
	}))
	defer srv.Close()

	client := &HTTPVendorClient{baseURL: srv.URL, httpClient: srv.Client()}
	_, err := client.Authenticate(context.Background(), "u", "wrong")
	if err != ErrVendorAuthFailed {
		t.Errorf("err = %v, want ErrVendorAuthFailed — the raw vendor response body must never surface (connector-security.md §3)", err)
	}
}

func TestHTTPVendorClient_RespectsContextTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &HTTPVendorClient{baseURL: srv.URL, httpClient: srv.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := client.Authenticate(ctx, "u", "p")
	if err == nil {
		t.Fatalf("Authenticate did not return an error despite a 20ms context timeout against a 200ms-slow server")
	}
}
