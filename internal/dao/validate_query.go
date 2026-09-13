package dao

import "strings"

// ValidateProfileQuery enforces the ErrInvalidQuery triggers enumerated in
// multi-db-strategy.md §4: a non-nil filter that is empty after trimming;
// Limit < 0 or > 100; Offset < 0 or > 10_000; Country not exactly two
// characters. An all-nil query is legal. Both backends call this before
// building any SQL, so the rule is identical across engines rather than
// each package re-deriving it.
func ValidateProfileQuery(q ProfileQuery) error {
	for _, f := range []*string{q.Name, q.Phone, q.Region, q.Country} {
		if f != nil && strings.TrimSpace(*f) == "" {
			return ErrInvalidQuery
		}
	}
	if q.Limit < 0 || q.Limit > 100 {
		return ErrInvalidQuery
	}
	if q.Offset < 0 || q.Offset > 10_000 {
		return ErrInvalidQuery
	}
	if q.Country != nil && len(*q.Country) != 2 {
		return ErrInvalidQuery
	}
	return nil
}

// EffectiveLimit returns 50 when Limit is 0 (deliberately, per §3a note 2),
// otherwise Limit unchanged. Call only after ValidateProfileQuery passes.
func EffectiveLimit(q ProfileQuery) int {
	if q.Limit == 0 {
		return 50
	}
	return q.Limit
}
