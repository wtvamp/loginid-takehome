// errors.go implements this package's boundary-translation function, per
// decisions/error-semantics.md: internal/api never serializes err.Error()
// on an unmapped error. Only the DAO's seven sentinels' safe messages
// (plus the authn/authz/context rules below) ever reach a client; anything
// else defaults to a generic 500 with no wrapped detail.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"loginid-takehome/internal/dao"
)

// errorResponse is the only JSON shape an error path ever writes. code is
// a short machine-readable string (never a driver error, never
// err.Error()); message is a safe, human-readable sentence.
type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Code: code, Message: message})
}

// writeUnauthorized and writeForbidden are named separately from the
// generic mapper below because F7 (decisions/error-semantics.md) requires
// an identical client-visible body for a scope failure and an
// authorize() policy denial — the two are distinguished only in the
// audit log, never in what the client sees. Both call the one function
// below so that identity is enforced by construction, not by two call
// sites happening to agree.
func writeUnauthorized(w http.ResponseWriter) {
	writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid token")
}

func writeForbidden(w http.ResponseWriter) {
	writeError(w, http.StatusForbidden, "forbidden", "not authorized for this request")
}

func writeRateLimited(w http.ResponseWriter) {
	writeError(w, http.StatusTooManyRequests, "rate_limited", "rate limit exceeded")
}

func writeTouchCapExceeded(w http.ResponseWriter) {
	writeError(w, http.StatusTooManyRequests, "cumulative_cap_exceeded", "cumulative record-access cap exceeded for the current window")
}

func writeBadRequest(w http.ResponseWriter, message string) {
	writeError(w, http.StatusBadRequest, "bad_request", message)
}

// writeDAOError translates an error returned by the DAO layer into an
// HTTP response, per decisions/error-semantics.md's boundary rule and its
// F7/F50 additions:
//   - context.DeadlineExceeded -> 504, never wrapped in a generic 500.
//   - context.Canceled -> handled by the caller before this is reached
//     (a cancelled client request has no response to write); if it
//     reaches here anyway it's treated the same as an unmapped error but
//     the caller should not count it as an alerting failure.
//   - one of the seven dao sentinels -> its mapped status/safe message.
//   - anything else -> 500, no wrapped detail, ever.
func writeDAOError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "deadline_exceeded", "request timed out")
	case errors.Is(err, dao.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no such record")
	case errors.Is(err, dao.ErrInvalidArgument):
		writeBadRequest(w, "invalid request")
	case errors.Is(err, dao.ErrInvalidQuery):
		writeBadRequest(w, "invalid search query")
	case errors.Is(err, dao.ErrDuplicateUsername),
		errors.Is(err, dao.ErrInvalidMethod),
		errors.Is(err, dao.ErrInvalidCredential),
		errors.Is(err, dao.ErrAlreadyExists):
		// None of these are reachable from this package's read-only
		// search/retrieve handlers (they're write-path sentinels), but
		// mapped defensively rather than falling through to a raw 500
		// if a future handler in this package ever does reach them.
		writeBadRequest(w, "request could not be completed")
	default:
		// Never err.Error() here — an unmapped error (a raw driver
		// error that somehow escaped the DAO's translation layer, a
		// context.Canceled that reached this far, anything else) gets a
		// generic message with no wrapped detail (decisions/
		// error-semantics.md's F50: no DETAIL text, no driver message,
		// ever, ***even in the log line a caller can't see*** — that
		// sanitization is the DAO translation layer's job upstream of
		// here, not repeated in this file).
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
	}
}

// writeInternalError is writeDAOError's twin for a collaborator that
// isn't the DAO — RateLimiter, TouchCounter, Authorizer — kept as a
// separate, identically-behaving function rather than reusing
// writeDAOError at those call sites (Oren Castellan, PR #26 review):
// writeDAOError is named and documented as the DAO-sentinel translator,
// and a future edit to its switch (adding a case for some DAO-specific
// condition) should not silently also change how an authz/rate-limit
// collaborator's errors are handled. Behavior is identical today — never
// err.Error(), context.DeadlineExceeded still maps to 504 — because none
// of these collaborators have their own sentinel set to translate.
func writeInternalError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		writeError(w, http.StatusGatewayTimeout, "deadline_exceeded", "request timed out")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
}

// WriteServiceUnavailable is exported for internal/app's router wiring:
// when the DAO repository couldn't be constructed at startup (e.g. no
// DB_DRIVER/DB_DSN configured yet), the verifying Deployment must still
// start and serve /healthz — it must not crash-loop the whole process
// over a dependency LT-39's handlers need but nothing else does. Callers
// that have no working Repository at all route the protected endpoints
// to this instead of constructing a Deps with a nil Repo (which would
// panic the first time a handler called d.Repo.Profiles()).
func WriteServiceUnavailable(w http.ResponseWriter) {
	writeError(w, http.StatusServiceUnavailable, "service_unavailable", "service temporarily unavailable")
}
