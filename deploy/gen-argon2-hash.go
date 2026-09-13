//go:build ignore

// Standalone runbook helper, never built into any image or run by any
// application code path — generates an Argon2id hash in exactly the
// encoding internal/api/clientstore.go's hashSecret/verifySecret expect
// ("<base64-salt>$<base64-hash>", RawStdEncoding), with the same fixed
// cost profile (time=1, memory=64MiB, threads=4, keyLen=32, saltLen=16).
// Kept as source, not a checked-in binary, and excluded from normal
// builds via the "ignore" build tag above — `go run` compiles and
// discards it each time.
//
// Usage (never let the plaintext or the hash reach a log, a commit, or a
// chat message — pipe or redirect only):
//
//   read -s -p "QA client secret: " QA_SECRET; echo
//   go run deploy/gen-argon2-hash.go "$QA_SECRET" > /tmp/qa-hash.txt
//   unset QA_SECRET
//
// /tmp/qa-hash.txt then holds the PHC-shaped string to embed in the
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

	fmt.Println(base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(hash))
}
