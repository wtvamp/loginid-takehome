package app

// Commit is injected at build time via
// `-ldflags "-X loginid-takehome/internal/app.Commit=<sha>"`. Per 02's ruling
// on the unauthenticated health endpoint (api-auth-design.md), this is the
// ONLY build identifier the response may carry — an opaque commit SHA, never
// a full semver/library-version string or a build timestamp. CI (04's
// pipeline) sets this from the commit SHA it already tags the Docker image
// with. The zero value below is what a `go build` without that flag
// produces — visible as "unknown" rather than empty, so a health response
// missing real build info is obviously wrong rather than silently blank.
//
// (LT-34 freshness-check commit: trivial, no behavior change — proves the
// build field actually moves between two deploys, per the PO review script.)
var Commit = "unknown"
