package onboarding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"loginid-takehome/internal/connector"
)

// httpConnectorCallTimeout is this client's own belt-and-braces
// http.Client.Timeout, independent of connectorCallTimeout's
// context.WithTimeout in onboarding.go — the same defense-in-depth
// pattern internal/connector's HTTPVendorClient uses.
const httpConnectorCallTimeout = 10 * time.Second

var defaultConnectorHTTPClient = &http.Client{
	Timeout: httpConnectorCallTimeout,
	Transport: &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
		MaxConnsPerHost:     10,
		IdleConnTimeout:     30 * time.Second,
	},
}

// HTTPConnectorClient calls cmd/idp-connector's real /auth and /identity
// endpoints. Mirrors internal/connector.HTTPVendorClient's own
// discipline: a bounded connection pool, and every request built fresh
// inside each call — baseURL/httpClient are the only fields this type
// holds, so a call's own bearer/vendor tokens are always per-call
// parameters, never client-level state.
type HTTPConnectorClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPConnectorClient requires a non-empty baseURL.
func NewHTTPConnectorClient(baseURL string, httpClient *http.Client) (*HTTPConnectorClient, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("onboarding: idp-connector base URL must not be empty")
	}
	if _, err := url.Parse(baseURL); err != nil {
		return nil, fmt.Errorf("onboarding: parsing idp-connector base URL: %w", err)
	}
	if httpClient == nil {
		httpClient = defaultConnectorHTTPClient
	}
	return &HTTPConnectorClient{baseURL: baseURL, httpClient: httpClient}, nil
}

type connectorAuthRequestBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type connectorAuthResponseBody struct {
	AccessToken string `json:"access_token"`
}

func (c *HTTPConnectorClient) Auth(ctx context.Context, ourToken, vendorUsername, vendorPassword string) (string, error) {
	reqBody, err := json.Marshal(connectorAuthRequestBody{Username: vendorUsername, Password: vendorPassword})
	if err != nil {
		return "", fmt.Errorf("onboarding: encoding connector auth request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/auth", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("onboarding: building connector auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// ourToken set on this request only — never a client-level default
	// header, never retained past this function returning.
	req.Header.Set("Authorization", "Bearer "+ourToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("onboarding: idp-connector /auth unreachable: %w", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return "", ErrVendorCredentialRejected
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("onboarding: idp-connector /auth returned status %d", resp.StatusCode)
	}

	var body connectorAuthResponseBody
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&body); err != nil || body.AccessToken == "" {
		return "", fmt.Errorf("onboarding: malformed connector /auth response")
	}
	return body.AccessToken, nil
}

func (c *HTTPConnectorClient) Identity(ctx context.Context, ourToken, vendorAccessToken, phone, name string) (connector.Identity, error) {
	reqBody, err := json.Marshal(struct {
		Phone string `json:"phone"`
		Name  string `json:"name"`
	}{Phone: phone, Name: name})
	if err != nil {
		return connector.Identity{}, fmt.Errorf("onboarding: encoding connector identity request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/identity", bytes.NewReader(reqBody))
	if err != nil {
		return connector.Identity{}, fmt.Errorf("onboarding: building connector identity request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ourToken)
	// vendorAccessToken travels as a header, matching idp-connector's
	// own fixed {"phone","name"} body contract (LT-41) — never a body
	// field, and this is the one place this function ever reads it.
	req.Header.Set("X-Vendor-Access-Token", vendorAccessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return connector.Identity{}, fmt.Errorf("onboarding: idp-connector /identity unreachable: %w", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return connector.Identity{}, ErrVendorCredentialRejected
	}
	if resp.StatusCode != http.StatusOK {
		return connector.Identity{}, fmt.Errorf("onboarding: idp-connector /identity returned status %d", resp.StatusCode)
	}

	// connectorIdentityResponseBody's fields are identical in
	// name/order/type to connector.Identity (only the json tags
	// differ) — decoding into this shape then converting is required
	// since connector.Identity itself carries no json tags at all
	// (StreetAddress/PostalCode would otherwise silently fail to match
	// idp-connector's snake_case wire fields).
	var body connectorIdentityResponseBody
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&body); err != nil {
		return connector.Identity{}, fmt.Errorf("onboarding: malformed connector /identity response")
	}
	return connector.Identity(body), nil
}

type connectorIdentityResponseBody struct {
	Name          string `json:"name"`
	Phone         string `json:"phone"`
	StreetAddress string `json:"street_address"`
	Locality      string `json:"locality"`
	Region        string `json:"region"`
	PostalCode    string `json:"postal_code"`
	Country       string `json:"country"`
}
