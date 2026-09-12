// Package model holds the domain structs shared by internal/dao,
// internal/api, and internal/connector — per PLANNING.md's Service
// boundaries and 05-data-ops/multi-db-strategy.md §1a: domain types live
// here (not in internal/dao) because handlers and the connector both
// handle them without touching the DAO.
package model

import "time"

// UserProfile matches multi-db-strategy.md §2 exactly. Pointer fields mean
// "absent", never "empty" — a non-nil, empty *string is invalid input, not
// a value (see internal/dao's validation helpers).
type UserProfile struct {
	ID            string // UUID
	Name          string // required, never empty
	Phone         *string
	StreetAddress *string
	Locality      *string
	Region        *string
	PostalCode    *string
	Country       *string // ISO 3166-1 alpha-2, upper-case
	Source        string  // "direct" | "idp_cache"
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// UserCredential matches multi-db-strategy.md §2 exactly. SecretState makes
// the three real states (no server-held secret required, not yet set,
// revoked) distinguishable — a bare nullable Secret cannot.
type UserCredential struct {
	ID          string // UUID
	UserID      string // FK -> user_profile.id
	Username    string // unique, case-folded
	MethodID    string // FK -> auth_method.id
	Secret      []byte // nil unless SecretState == "set"
	SecretState string // "none" | "set" | "revoked"
	HashAlgo    *string
	HashCost    *int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// AuthMethod matches multi-db-strategy.md §2 exactly.
type AuthMethod struct {
	ID             string
	Name           string // "password", "passkey", "otp", "oidc", ...
	RequiresSecret bool
	IsActive       bool
	CreatedAt      time.Time
}
