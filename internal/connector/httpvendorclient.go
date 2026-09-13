package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// vendorCallTimeout bounds every outbound vendor call — belt-and-braces
// alongside the caller's own context.WithTimeout, the same
// defense-in-depth pattern LT-40's JWKS fetch client uses (an explicit
// http.Client.Timeout independent of context, since a context deadline
// bug elsewhere must not leave this client waiting forever).
const vendorCallTimeout = 10 * time.Second

// defaultVendorHTTPClient is a bounded-pool client — connector-security.md's
// "bounded connection pool" requirement. Deliberately module-level so
// every HTTPVendorClient constructed without an explicit client shares
// one bounded pool rather than each accidentally getting Go's unbounded
// http.DefaultTransport.
var defaultVendorHTTPClient = &http.Client{
	Timeout: vendorCallTimeout,
	Transport: &http.Transport{
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 5,
		MaxConnsPerHost:     10,
		IdleConnTimeout:     30 * time.Second,
	},
}

// HTTPVendorClient calls a real (or real-shaped) vendor over HTTPS,
// demonstrating connector-security.md's outbound discipline: TLS only,
// a bounded connection pool, and a fresh, isolated Authorization header
// built inside each call rather than held as client-level state —
// httpClient's connection pooling can reuse a TCP connection across
// calls, but never a header, since baseURL/httpClient are the only
// fields this type holds; the token is a per-call parameter, never
// stored.
//
// No real ABC/XYZ vendor exists for this take-home (LT-41's own
// non-goal) — this implementation assumes a vendor that happens to
// share this project's own JSON request/response shape, a stated
// simplifying assumption for demonstrating the connector's own outbound
// security properties, not a claim about any real vendor's actual API.
type HTTPVendorClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPVendorClient requires an https:// baseURL — connector-security.md
// §2: "Vendor calls over TLS only ... including in local/dev
// environments." httpClient defaults to defaultVendorHTTPClient (bounded
// pool + timeout) if nil.
func NewHTTPVendorClient(baseURL string, httpClient *http.Client) (*HTTPVendorClient, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("connector: parsing vendor base URL: %w", err)
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("connector: vendor base URL %q must use https, per connector-security.md's TLS-only requirement", baseURL)
	}
	if httpClient == nil {
		httpClient = defaultVendorHTTPClient
	}
	return &HTTPVendorClient{baseURL: baseURL, httpClient: httpClient}, nil
}

type vendorAuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type vendorAuthResponse struct {
	AccessToken string `json:"access_token"`
}

func (c *HTTPVendorClient) Authenticate(ctx context.Context, username, password string) (string, error) {
	reqBody, err := json.Marshal(vendorAuthRequest{Username: username, Password: password})
	if err != nil {
		return "", fmt.Errorf("connector: encoding vendor auth request: %w", err)
	}

	// A fresh *http.Request built from scratch on every call — no
	// client-level default header exists to leak across calls, and this
	// request object is never retained past this function returning.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/auth", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("connector: building vendor auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Never wrap or surface the raw error to our own caller —
		// connector-security.md §4: vendor errors must not distinguish
		// which part of the credential was wrong, and a raw dial/DNS
		// error can itself carry sensitive detail (host, IP). The
		// caller-facing generic failure is this package's caller's job
		// (internal/connector's handlers); this method's contract is
		// simply "success or ErrVendorAuthFailed", nothing richer.
		return "", ErrVendorAuthFailed
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Do not read/log the response body — connector-security.md §3:
		// "do not log raw vendor response bodies by default, since a
		// vendor error payload can itself echo back submitted PII or
		// partial credentials."
		return "", ErrVendorAuthFailed
	}

	var body vendorAuthResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil || body.AccessToken == "" {
		return "", ErrVendorAuthFailed
	}
	return body.AccessToken, nil
}

type vendorIdentityRequest struct {
	Phone string `json:"phone"`
	Name  string `json:"name"`
}

type vendorIdentityResponse struct {
	Name          string `json:"name"`
	Phone         string `json:"phone"`
	StreetAddress string `json:"street_address"`
	Locality      string `json:"locality"`
	Region        string `json:"region"`
	PostalCode    string `json:"postal_code"`
	Country       string `json:"country"`
}

func (c *HTTPVendorClient) FetchIdentity(ctx context.Context, accessToken, phone, name string) (Identity, error) {
	reqBody, err := json.Marshal(vendorIdentityRequest{Phone: phone, Name: name})
	if err != nil {
		return Identity{}, fmt.Errorf("connector: encoding vendor identity request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/identity", bytes.NewReader(reqBody))
	if err != nil {
		return Identity{}, fmt.Errorf("connector: building vendor identity request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// The one place accessToken is used — set on this request only,
	// never on a client-level default header, never retained after this
	// function returns.
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Identity{}, ErrVendorIdentityNotFound
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Identity{}, ErrVendorIdentityNotFound
	}

	var body vendorIdentityResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return Identity{}, ErrVendorIdentityNotFound
	}
	// vendorIdentityResponse's fields are identical in name/order/type to
	// Identity (only the json tags differ), so a direct type conversion
	// is both valid and clearer than restating every field (staticcheck
	// S1016).
	return Identity(body), nil
}
