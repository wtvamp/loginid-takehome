package sqlite

import "strings"

// escapeLike escapes %, _ and \ in caller input so it's treated as a
// literal LIKE pattern (multi-db-strategy.md §6.4). SQLite has no default
// backslash escape, unlike Postgres, so callers of this package's Search
// always combine the result with an explicit `ESCAPE '\'` clause.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
