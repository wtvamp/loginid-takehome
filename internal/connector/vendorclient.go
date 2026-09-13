// Package connector implements Q3 — cmd/idp-connector's outbound calls
// to a generic third-party identity provider ("ABC"/"XYZ" in the
// assignment's own generic framing), per
// 02-ai-security-architecture/connector-security.md and
// refinement/LT-41.md. No real vendor integration exists or is asked
// for (LT-41's own non-goals) — VendorClient is the seam a real vendor
// adapter would implement; this package ships a stub (stubvendorclient.go)
// good enough to demonstrate the connector's own contract and security
// properties end to end, and an HTTP-based implementation
// (httpvendorclient.go) that demonstrates the outbound security
// discipline connector-security.md requires, callable against any vendor
// that happens to share this project's own JSON shape (a stated
// simplifying assumption, not a claim about any real vendor's actual
// API).
package connector

import "context"

// Identity is the PII a vendor's /identity endpoint returns, matching
// the assignment's own field list verbatim — name, phone, and address
// (street_address, locality, region, postal_code, country).
type Identity struct {
	Name          string
	Phone         string
	StreetAddress string
	Locality      string
	Region        string
	PostalCode    string
	Country       string
}

// VendorClient is this connector's seam to a third-party IDP. Both
// methods take the vendor bearer token as an explicit parameter, never a
// client-level field — connector-security.md §1's fetch-use-zeroize
// boundary requires the token's scope to run from the point it's read
// off the request to the end of that single call, which a client-level
// field would silently violate by keeping it alive for the client's own
// lifetime instead.
type VendorClient interface {
	// Authenticate exchanges an end-user's vendor username/password for
	// a vendor access token, per the assignment's own POST /auth
	// contract. Returns a generic error on any failure — the specific
	// reason (unknown username vs. wrong password vs. vendor
	// unreachable) is for this package's own internal handling only,
	// never surfaced to our caller (connector-security.md §4:
	// "must not leak which part of the credential was wrong").
	Authenticate(ctx context.Context, username, password string) (accessToken string, err error)

	// FetchIdentity looks up PII by phone/name using an already-obtained
	// vendor accessToken, per the assignment's own POST /identity
	// contract. accessToken is used for this one call only and is never
	// retained by the implementation past this call returning.
	FetchIdentity(ctx context.Context, accessToken, phone, name string) (Identity, error)
}
