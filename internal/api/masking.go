package api

import (
	"strings"

	"loginid-takehome/internal/model"
)

// SearchResultProfile is profile:search's masked response shape —
// handoff-03-auth.md v3: "masking is a profile:search-response property,
// not a per-call flag." A distinct type (and distinct serialization
// function, below) from the full read-response shape, per LT-39's own
// acceptance criteria: "the search-response serialization path is a
// distinct code path from the read-response one, not the same serializer
// with a masking flag." Unmasked detail is obtained only through a
// separate, separately-audited profile:read:own/profile:read:any call.
type SearchResultProfile struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Phone  *string `json:"phone,omitempty"`  // masked, e.g. "***-***-1234"
	Region *string `json:"region,omitempty"` // address reduced to locality/region only
}

// ProfileResponse is the full, unmasked shape returned by a
// profile:read:own/profile:read:any call.
type ProfileResponse struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Phone         *string `json:"phone,omitempty"`
	StreetAddress *string `json:"street_address,omitempty"`
	Locality      *string `json:"locality,omitempty"`
	Region        *string `json:"region,omitempty"`
	PostalCode    *string `json:"postal_code,omitempty"`
	Country       *string `json:"country,omitempty"`
}

// toSearchResult builds the masked search-response shape for one
// profile. This function, not ProfileResponse's serialization, is the
// only place a search result is ever produced — there is no shared
// helper with a masking bool, by design.
func toSearchResult(p model.UserProfile) SearchResultProfile {
	return SearchResultProfile{
		ID:     p.ID,
		Name:   p.Name,
		Phone:  maskPhone(p.Phone),
		Region: p.Region,
	}
}

// toProfileResponse builds the full, unmasked read-response shape.
func toProfileResponse(p *model.UserProfile) ProfileResponse {
	return ProfileResponse{
		ID:            p.ID,
		Name:          p.Name,
		Phone:         p.Phone,
		StreetAddress: p.StreetAddress,
		Locality:      p.Locality,
		Region:        p.Region,
		PostalCode:    p.PostalCode,
		Country:       p.Country,
	}
}

// maskPhone reduces an E.164 phone to its last 4 digits, e.g.
// "+15551234567" -> "***-***-4567" — nil stays nil (absent, not masked
// into a value that looks like data).
func maskPhone(phone *string) *string {
	if phone == nil {
		return nil
	}
	digits := *phone
	if len(digits) < 4 {
		masked := strings.Repeat("*", len(digits))
		return &masked
	}
	last4 := digits[len(digits)-4:]
	masked := "***-***-" + last4
	return &masked
}
