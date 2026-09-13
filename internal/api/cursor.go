package api

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// cursorPrefix distinguishes this package's cursors from arbitrary
// base64 a caller might send, so a malformed cursor fails with a clear
// message rather than a confusing offset.
const cursorPrefix = "v1:"

// encodeCursor produces the opaque, API-facing cursor for the next page
// starting at offset. handoff-03-auth.md v3's F-pag correction: the
// cursor may encode the DAO's own Offset directly underneath — the
// opaque-cursor requirement is that callers cannot construct or walk one
// by hand, not that the handler is forbidden from using Offset
// internally. Encoding is base64 of a versioned, prefixed string rather
// than the bare decimal offset so a caller can't trivially read or
// increment it by eye.
func encodeCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(cursorPrefix + strconv.Itoa(offset)))
}

// decodeCursor recovers the offset encodeCursor produced. An empty
// cursor (the first page) decodes to offset 0, not an error.
func decodeCursor(cursor string) (offset int, err error) {
	if cursor == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, fmt.Errorf("api: malformed cursor: %w", err)
	}
	s := string(raw)
	if !strings.HasPrefix(s, cursorPrefix) {
		return 0, fmt.Errorf("api: malformed cursor: missing version prefix")
	}
	n, err := strconv.Atoi(strings.TrimPrefix(s, cursorPrefix))
	if err != nil {
		return 0, fmt.Errorf("api: malformed cursor: %w", err)
	}
	if n < 0 {
		return 0, fmt.Errorf("api: malformed cursor: negative offset")
	}
	return n, nil
}
