//go:build ignore

// Standalone runbook helper, never built into any image or run by any
// application code path — generates an Argon2id hash as a standard PHC
// string ($argon2id$v=19$m=...,t=...,p=...$<salt>$<hash>), the same
// self-describing encoding internal/api/clientstore.go's verifySecret
// reads its cost parameters back from — not the app's own compile-time
// constants (Priya Nandakumar's ruling, handoff-03-auth.md v7: a bare
// "<base64-salt>$<base64-hash>" with no embedded parameters would make a
// future cost-profile bump break every existing hash at once with no way
// to tell an old row from a new one). Cost profile here
// (time=1, memory=64MiB, threads=4, keyLen=32, saltLen=16) matches
// clientstore.go's CURRENT profile, but the hash this program emits
// verifies correctly even after that profile changes, since the
// parameters travel with the hash. Kept as source, not a checked-in
// binary, and excluded from normal builds via the "ignore" build tag
// above — `go run` compiles and discards it each time.
//
// Usage (never let the plaintext or the hash reach a log, a commit, or a
// chat message — pipe or redirect only):
//
//   read -s -p "QA client secret: " QA_SECRET; echo
//   go run deploy/gen-argon2-hash.go "$QA_SECRET" > /tmp/qa-hash.txt
//   unset QA_SECRET
//
// /tmp/qa-hash.txt then holds the PHC-format string to embed in the
// INSERT in deploy/demo-data/qa-client-bootstrap.md's runbook step;
// delete the temp file after.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"

	"golang.org/x/crypto/argon2"
)

const (
	argon2Time    = 1
	argon2Memory  = 64 * 1024
	argon2Threads = 4
	argon2KeyLen  = 32
	argon2SaltLen = 16
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run deploy/gen-argon2-hash.go <secret>")
		os.Exit(1)
	}
	secret := os.Args[1]

	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		fmt.Fprintf(os.Stderr, "generating salt: %v\n", err)
		os.Exit(1)
	}
	hash := argon2.IDKey([]byte(secret), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)

	fmt.Printf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s\n",
		argon2.Version, argon2Memory, argon2Time, argon2Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}
