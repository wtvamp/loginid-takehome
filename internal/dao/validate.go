package dao

import "loginid-takehome/internal/model"

// ValidateCreateID enforces multi-db-strategy.md §2's rule: on Create, the
// caller leaves ID as "" and the DAO generates it. A non-empty ID is a
// caller error, not a request to use that ID.
func ValidateCreateID(id string) error {
	if id != "" {
		return ErrInvalidArgument
	}
	return nil
}

// ValidateProfilePointers enforces "absent, never empty" for every
// *string field on UserProfile: a non-nil, empty pointer is invalid input.
// Backend implementations (S3/S4) call this before any write.
func ValidateProfilePointers(p *model.UserProfile) error {
	for _, f := range []*string{p.Phone, p.StreetAddress, p.Locality, p.Region, p.PostalCode, p.Country} {
		if f != nil && *f == "" {
			return ErrInvalidArgument
		}
	}
	return nil
}

// ValidateCredentialPointers enforces "absent, never empty" for
// UserCredential's *string field(s).
func ValidateCredentialPointers(c *model.UserCredential) error {
	if c.HashAlgo != nil && *c.HashAlgo == "" {
		return ErrInvalidArgument
	}
	return nil
}

// PrepareCreateProfileWithCredential clears any caller-supplied UserID on
// c, per multi-db-strategy.md §3b: the DAO always sets UserID from the
// profile it just created, never trusting a caller-supplied value. Backend
// implementations call this before creating the credential row, then set
// the real UserID from the newly created profile's ID.
func PrepareCreateProfileWithCredential(c *model.UserCredential) {
	c.UserID = ""
}
