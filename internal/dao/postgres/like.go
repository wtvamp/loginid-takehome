package postgres

import "strings"

// escapeLike escapes %, _ and \ in caller input so it's treated as a
// literal LIKE pattern, per multi-db-strategy.md §6.4. Callers of this
// package's Search always combine the result with an explicit
// `ESCAPE '\'` clause in the query text.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
