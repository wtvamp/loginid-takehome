package dao

import (
	"strings"

	"loginid-takehome/internal/model"
)

// NormalizeProfileForWrite upper-cases Country before a write, per
// multi-db-strategy.md §6.5: "Country normalization at write, not only at
// read... Upper-casing the query without upper-casing the table makes a
// stored 'us' silently invisible to a search for 'US'." The CHECK
// constraint alone only rejects already-wrong input; it doesn't normalize
// it, so this is required in addition to it, in both backend
// implementations, called from Create/Update/Upsert before the SQL is
// built.
func NormalizeProfileForWrite(p *model.UserProfile) {
	if p.Country != nil {
		upper := strings.ToUpper(*p.Country)
		p.Country = &upper
	}
}
