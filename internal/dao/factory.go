package dao

import "fmt"

// New selects a backend implementation by driver name, matching
// multi-db-strategy.md §1's factory signature exactly. "postgres" and
// "cockroachdb" are distinct driver strings rather than one because the
// engine-aware retry seam (§4) needs to know which engine it is talking to.
//
// No backend is registered yet — S3 (postgres/cockroachdb) and S4 (sqlite)
// register themselves via Register in their own package init(). Until
// then, New always returns an error for every driver name, which is
// expected: this story's non-goal is real I/O, not a callable factory.
func New(driver, dsn string) (Repository, error) {
	ctor, ok := drivers[driver]
	if !ok {
		return nil, fmt.Errorf("dao: unknown driver %q", driver)
	}
	return ctor(dsn)
}

type driverCtor func(dsn string) (Repository, error)

var drivers = map[string]driverCtor{}

// Register is called by a backend package's init() to register itself
// under a driver name (e.g. "postgres", "cockroachdb", "sqlite"). Not part
// of 05's Go-shaped contract — an internal wiring mechanism this track owns
// so New's signature never has to change as backends are added.
func Register(name string, ctor driverCtor) {
	drivers[name] = ctor
}
