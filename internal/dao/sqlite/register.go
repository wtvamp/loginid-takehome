package sqlite

import "loginid-takehome/internal/dao"

// init registers this package under the "sqlite" driver string, per
// multi-db-strategy.md §1.
func init() {
	dao.Register("sqlite", New)
}
