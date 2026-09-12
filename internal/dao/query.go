package dao

// ProfileQuery matches multi-db-strategy.md §3 exactly. nil means "don't
// filter"; a non-nil, empty string is invalid input (ErrInvalidQuery), per
// the same "absent, never empty" convention as the domain types.
type ProfileQuery struct {
	Name    *string // case-folded substring match; caller input escaped by the DAO
	Phone   *string // EXACT match on the E.164-normalized value (not prefix)
	Region  *string // case-folded exact match
	Country *string // exact match, alpha-2, upper-cased by the DAO
	Limit   int     // 1..100; 0 means "use default 50", deliberately
	Offset  int     // 0..10_000
}
