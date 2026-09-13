package api

import (
	"context"
	"encoding/json"
	"log"
	"time"
)

// AuditEventKind names a policy decision for the audit trail —
// handoff-03-auth.md v3's audit-log field list requires "policy decision
// (allow/deny, and if deny, whether it was a scope failure or an
// authorize() policy denial — these are distinct signals for detection)"
// on every search/retrieve call. The client-visible response body is
// deliberately identical for AuditScopeFailure and AuditPolicyDenial
// (decisions/error-semantics.md F7) — this is the ONLY place that
// distinction is allowed to exist.
type AuditEventKind string

const (
	AuditAuthnFailure  AuditEventKind = "authn_failure"
	AuditScopeFailure  AuditEventKind = "scope_failure"
	AuditPolicyDenial  AuditEventKind = "authorize_denial"
	AuditRateLimited   AuditEventKind = "rate_limited"
	AuditCapExceeded   AuditEventKind = "cumulative_cap_exceeded"
	AuditRequestFailed AuditEventKind = "request_failed" // a DAO/internal error, not a policy decision
	AuditSuccess       AuditEventKind = "success"
)

// AuditEvent carries exactly the fields handoff-03-auth.md v3's audit-log
// section names, and only those — RecordIDs are opaque ids, never PII
// field values; ReasonCode is empty unless the request was
// ScopeReadAny. Never add a field here without checking
// handoff-04-secrets.md's never-log list first (see AuditEvent's own
// logger implementations for the enforcement site).
type AuditEvent struct {
	Sub        string
	Scope      Scope
	Kind       AuditEventKind
	RecordIDs  []string
	ReasonCode ReasonCode
	Timestamp  time.Time
}

// AuditLogger is the seam this story wires (Ingrid Solano's PR #26
// review, ruled by the PM as an unmet acceptance criterion, not a
// follow-up story): handoff-03-auth.md v3 requires reason_code and the
// scope/authorize() distinction to be "logged with the audit record" —
// this interface is what every handler decision point calls, regardless
// of which real sink backs it. A structured-stdout implementation is
// sufficient for this story; 04's retention/RBAC-separated log pipeline
// is a separate, infrastructure-side concern this interface doesn't
// depend on.
type AuditLogger interface {
	Log(ctx context.Context, event AuditEvent)
}

// StdoutAuditLogger writes one JSON line per event to the process's
// standard logger. Real production wiring (a shipped log pipeline with
// the RBAC separation handoff-04-secrets.md requires between Secret
// access and audit-log access) is 04's infrastructure work; this
// satisfies "events are actually emitted somewhere real" for this story
// without waiting on that pipeline to exist.
type StdoutAuditLogger struct{}

func (StdoutAuditLogger) Log(_ context.Context, event AuditEvent) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	b, err := json.Marshal(event)
	if err != nil {
		log.Printf("api: audit event marshal error: %v", err)
		return
	}
	log.Printf("audit: %s", b)
}
