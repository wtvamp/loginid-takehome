package app

import (
	_ "embed"
	"net/http"
)

// demoPage is LT-53's single static HTML page, embedded directly into
// the api-service binary — no separate deploy artifact, no separate
// service. Served only at GET /demo on NewVerifierRouter (never on the
// issuer or sweep code paths, since this is a demo of the
// search/retrieve surface, not the issuer internals — refinement/
// LT-53.md's own acceptance criterion).
//
// The page is a pure client of the real API: its own JS mints a real
// token from the visitor's pasted QA client credential and calls
// /auth/token and /profiles/* directly from the browser, same-origin
// (Theo Bergman's, 04, ingress routing puts both the issuer and
// verifier Deployments behind the one public hostname). api-service's
// server-side code here never sees the plaintext QA secret — this
// handler only ever serves static bytes.
//
//go:embed demo_page.html
var demoPage []byte

// demoHandler serves the embedded page verbatim. No template
// execution, no per-request state, no query-string handling — a static
// GET, deliberately as simple as the acceptance criteria allow, so
// there's no server-side code path here to add never-log coverage for
// beyond "this handler never logs anything, and never will" (LT-53's
// own architecture criterion — a future server-side proxy step would
// need its own review at that point, not be assumed covered by this
// one).
func demoHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(demoPage)
	}
}
