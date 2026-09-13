package dao

// Conformance test — the "seam-13" mechanical check named in
// 03-engineering-delivery/refinement/LT-35.md and
// decisions/test-double-strategy.md. Asserts that this package's exported
// Go shapes match 05-data-ops/multi-db-strategy.md's §1–§4 (the factory and
// composite, domain types, sub-interfaces plus ProfileQuery, and sentinels)
// byte-for-byte, not "close enough".
//
// MECHANISM: FROZEN COPY, not live-parse. Reliably extracting exact Go
// signatures out of markdown prose is impractical — multi-db-strategy.md is
// prose describing Go shapes inside fenced code blocks, not a Go source
// file a parser can trust. Instead, the "expected*" declarations below were
// transcribed by hand from §1–§4 at the time this test was written, and
// pinned two independent ways so a drift on either side is caught:
//
//  1. Structural, on OUR side: assertInterfaceEquivalent checks the actual
//     dao interfaces against the hand-transcribed expected* interfaces by
//     mutual reflect.Type.AssignableTo (same method set, both directions)
//     plus a method-count check, which catches an added, removed, or
//     resignatured method that "compiles fine" checks alone would miss.
//     assertStructFields does the same for domain/query/result structs,
//     field by field, in order. The factory signature (§1) is pinned by a
//     compile-time function-variable assignment — a mismatch fails the
//     build, not just this test. Sentinel identity (§4) is checked by
//     exact wrapped-message string equality, and sentinel-SET completeness
//     (nothing added or removed without updating this test) is checked by
//     parsing this package's own errors.go with go/parser — a syntax parse
//     of our own Go source, not of the markdown, so it doesn't fall under
//     the "impractical" case above.
//
//  2. Drift, on the DOC's side: TestContractChecksumUpToDate hashes
//     05-data-ops/multi-db-strategy.md's §1–§4 section (delimited by the
//     marker strings below, matching this file's own scope) and compares
//     it against expectedContractChecksum, frozen at transcription time.
//     If 05 revises that section, this test FAILS LOUDLY instead of
//     silently continuing to assert a stale copy of the contract — a human
//     must re-transcribe the expected* declarations below and refresh the
//     checksum constant, not just bump a version number.
//
// Known scope limit, stated rather than hidden: this test proves the Go
// shapes named in §1–§4 match what was transcribed from the doc at the time
// of transcription, and that the doc section hasn't since changed
// underneath that transcription. It cannot prove the original transcription
// itself was faithful — that's why the PO review script
// (refinement/LT-35.md) has a human diff the PR against the doc directly,
// rather than trusting this test's own claim of a match.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"loginid-takehome/internal/model"
)

// ---- §1: the factory and the composite ----

// New's signature, pinned at compile time: if dao.New's signature ever
// drifts from multi-db-strategy.md §1, this assignment fails to build.
var _ func(string, string) (Repository, error) = New

type expectedRepository interface {
	Profiles() ProfileRepository
	Credentials() CredentialRepository
	Methods() AuthMethodRepository
	CreateProfileWithCredential(ctx context.Context, p *model.UserProfile, c *model.UserCredential) (*model.UserProfile, *model.UserCredential, error)
	DeleteExpired(ctx context.Context, class RetentionClass, olderThan time.Time, maxRows int) (SweepResult, error)
	DeleteProfile(ctx context.Context, id string, externalRef *string) error
	Close() error
}

func TestConformance_Repository(t *testing.T) {
	assertInterfaceEquivalent(t, "dao.Repository",
		reflect.TypeOf((*Repository)(nil)).Elem(),
		reflect.TypeOf((*expectedRepository)(nil)).Elem())
}

// ---- §3: the three sub-interfaces plus ProfileQuery ----

type expectedProfileRepository interface {
	Get(ctx context.Context, id string) (*model.UserProfile, error)
	Search(ctx context.Context, q ProfileQuery) (results []model.UserProfile, total int, err error)
	Create(ctx context.Context, p *model.UserProfile) (*model.UserProfile, error)
	Update(ctx context.Context, p *model.UserProfile) (*model.UserProfile, error)
	Upsert(ctx context.Context, p *model.UserProfile) (*model.UserProfile, error)
	Delete(ctx context.Context, id string) error
}

type expectedCredentialRepository interface {
	GetByUsername(ctx context.Context, username string) (*model.UserCredential, error)
	ListByUserID(ctx context.Context, userID string) ([]model.UserCredential, error)
	Create(ctx context.Context, c *model.UserCredential) (*model.UserCredential, error)
	Update(ctx context.Context, c *model.UserCredential) (*model.UserCredential, error)
	Delete(ctx context.Context, id string) error
}

type expectedAuthMethodRepository interface {
	Get(ctx context.Context, id string) (*model.AuthMethod, error)
	GetByName(ctx context.Context, name string) (*model.AuthMethod, error)
	List(ctx context.Context, activeOnly bool) ([]model.AuthMethod, error)
}

func TestConformance_SubInterfaces(t *testing.T) {
	assertInterfaceEquivalent(t, "dao.ProfileRepository",
		reflect.TypeOf((*ProfileRepository)(nil)).Elem(),
		reflect.TypeOf((*expectedProfileRepository)(nil)).Elem())
	assertInterfaceEquivalent(t, "dao.CredentialRepository",
		reflect.TypeOf((*CredentialRepository)(nil)).Elem(),
		reflect.TypeOf((*expectedCredentialRepository)(nil)).Elem())
	assertInterfaceEquivalent(t, "dao.AuthMethodRepository",
		reflect.TypeOf((*AuthMethodRepository)(nil)).Elem(),
		reflect.TypeOf((*expectedAuthMethodRepository)(nil)).Elem())
}

func TestConformance_ProfileQuery(t *testing.T) {
	assertStructFields(t, "dao.ProfileQuery", reflect.TypeOf(ProfileQuery{}), []fieldSpec{
		{"Name", "*string"},
		{"Phone", "*string"},
		{"Region", "*string"},
		{"Country", "*string"},
		{"Limit", "int"},
		{"Offset", "int"},
	})
}

// §3c addenda ride along with §3's scope in this test, since RetentionClass
// and SweepResult are part of the same sub-interface family the Repository
// composite exposes (DeleteExpired/DeleteProfile), not a separate document
// section with its own conformance obligation.
func TestConformance_RetentionTypes(t *testing.T) {
	if RetentionDirect != "direct" {
		t.Errorf("RetentionDirect = %q, want %q", RetentionDirect, "direct")
	}
	if RetentionIDPCache != "idp_cache" {
		t.Errorf("RetentionIDPCache = %q, want %q", RetentionIDPCache, "idp_cache")
	}
	if RetentionIDPCacheOrphan != "idp_cache_orphan" {
		t.Errorf("RetentionIDPCacheOrphan = %q, want %q", RetentionIDPCacheOrphan, "idp_cache_orphan")
	}
	assertStructFields(t, "dao.SweepResult", reflect.TypeOf(SweepResult{}), []fieldSpec{
		{"RowsExamined", "int"},
		{"RowsDeleted", "int"},
		{"OldestSurvivingAt", "*time.Time"},
		{"Drained", "bool"},
	})
}

// ---- §2: domain types (package model) ----

func TestConformance_DomainTypes(t *testing.T) {
	assertStructFields(t, "model.UserProfile", reflect.TypeOf(model.UserProfile{}), []fieldSpec{
		{"ID", "string"},
		{"Name", "string"},
		{"Phone", "*string"},
		{"StreetAddress", "*string"},
		{"Locality", "*string"},
		{"Region", "*string"},
		{"PostalCode", "*string"},
		{"Country", "*string"},
		{"Source", "string"},
		{"CreatedAt", "time.Time"},
		{"UpdatedAt", "time.Time"},
	})
	assertStructFields(t, "model.UserCredential", reflect.TypeOf(model.UserCredential{}), []fieldSpec{
		{"ID", "string"},
		{"UserID", "string"},
		{"Username", "string"},
		{"MethodID", "string"},
		{"Secret", "[]uint8"}, // []byte and []uint8 are identical types; reflect prints the latter
		{"SecretState", "string"},
		{"HashAlgo", "*string"},
		{"HashCost", "*int"},
		{"CreatedAt", "time.Time"},
		{"UpdatedAt", "time.Time"},
	})
	assertStructFields(t, "model.AuthMethod", reflect.TypeOf(model.AuthMethod{}), []fieldSpec{
		{"ID", "string"},
		{"Name", "string"},
		{"RequiresSecret", "bool"},
		{"IsActive", "bool"},
		{"CreatedAt", "time.Time"},
	})
}

// ---- §4: error semantics ----

// expectedSentinels is the frozen, exact set from multi-db-strategy.md §4 —
// name and wrapped message. ErrAlreadyExists is included: it is a real
// member of the sentinel set (05's ruling on Oren's original-review finding
// #1), just not a caller-reachable one — see its doc comment in errors.go,
// which is a plain-comment criterion this test cannot check and the PO
// review script checks by hand instead.
var expectedSentinels = []struct {
	name string
	err  error
	msg  string
}{
	{"ErrNotFound", ErrNotFound, "dao: not found"},
	{"ErrAlreadyExists", ErrAlreadyExists, "dao: already exists"},
	{"ErrDuplicateUsername", ErrDuplicateUsername, "dao: duplicate username"},
	{"ErrInvalidMethod", ErrInvalidMethod, "dao: unknown or inactive auth method"},
	{"ErrInvalidQuery", ErrInvalidQuery, "dao: invalid query"},
	{"ErrInvalidArgument", ErrInvalidArgument, "dao: invalid argument"},
	{"ErrInvalidCredential", ErrInvalidCredential, "dao: credential violates secret-state rules"},
}

func TestConformance_SentinelMessages(t *testing.T) {
	for _, s := range expectedSentinels {
		if s.err == nil {
			t.Errorf("sentinel %s is nil", s.name)
			continue
		}
		if got := s.err.Error(); got != s.msg {
			t.Errorf("%s.Error() = %q, want %q", s.name, got, s.msg)
		}
	}
}

// TestConformance_SentinelSetIsExactlyTheFrozenSeven parses this package's
// own errors.go (a syntax parse of our Go source, not of the markdown doc)
// to catch a sentinel added or removed from the `var (...)` block without
// this test being updated — reflection alone has no API to enumerate
// package-level vars, so a static AST walk is the only way to check
// set-completeness rather than just checking the seven names we already
// know to look for.
func TestConformance_SentinelSetIsExactlyTheFrozenSeven(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "errors.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing errors.go: %v", err)
	}

	var found []string
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				// Only count entries actually initialized with errors.New(...)
				// or fmt.Errorf(...) — this is a sentinel var block, and we
				// want to fail loudly if that assumption ever stops holding
				// rather than silently miscounting.
				if i >= len(vs.Values) {
					continue
				}
				call, ok := vs.Values[i].(*ast.CallExpr)
				if !ok {
					continue
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				if sel.Sel.Name == "New" || sel.Sel.Name == "Errorf" {
					found = append(found, name.Name)
				}
			}
		}
	}

	want := make([]string, len(expectedSentinels))
	for i, s := range expectedSentinels {
		want[i] = s.name
	}
	sort.Strings(found)
	sort.Strings(want)

	if len(found) != len(want) {
		t.Fatalf("errors.go declares %d sentinel(s) %v, frozen §4 copy expects %d %v — a sentinel was added or removed without updating this test", len(found), found, len(want), want)
	}
	for i := range want {
		if found[i] != want[i] {
			t.Fatalf("errors.go's sentinel set %v does not match frozen §4 copy %v", found, want)
		}
	}
}

// ---- Drift detection on the doc side ----

// expectedContractChecksum is the SHA-256 of multi-db-strategy.md's exact
// §1–§4 text (from the start of "## 1. The factory and the composite" up
// to, not including, "## 5. Field-level schema"), frozen at the time the
// expected* declarations above were transcribed. Computed with:
//
//	python3 -c 'import hashlib; c=open("05-data-ops/multi-db-strategy.md").read(); \
//	s=c.index("## 1. The factory and the composite"); e=c.index("## 5. Field-level schema"); \
//	print(hashlib.sha256(c[s:e].encode()).hexdigest())'
const expectedContractChecksum = "8c6551c5fe08f108f07ace1b76785104146dd96eb911156d53185d953bfd6dd3"

const contractSectionStartMarker = "## 1. The factory and the composite"
const contractSectionEndMarker = "## 5. Field-level schema"

// contractDocPath is relative to this package's directory (internal/dao),
// which is also `go test`'s working directory for this package.
const contractDocPath = "../../05-data-ops/multi-db-strategy.md"

func TestContractChecksumUpToDate(t *testing.T) {
	raw, err := os.ReadFile(contractDocPath)
	if err != nil {
		t.Fatalf("reading %s: %v — this test cannot verify the frozen copy above is still in sync with 05's contract", contractDocPath, err)
	}
	content := string(raw)

	start := strings.Index(content, contractSectionStartMarker)
	end := strings.Index(content, contractSectionEndMarker)
	if start == -1 || end == -1 || end <= start {
		t.Fatalf("could not locate §1–§4 markers in %s (start found=%v, end found=%v) — 05's contract structure changed enough that this test can no longer even find the section it's supposed to check", contractDocPath, start != -1, end != -1)
	}

	section := content[start:end]
	got := sha256Hex(section)
	if got != expectedContractChecksum {
		t.Fatalf(
			"05-data-ops/multi-db-strategy.md §1–§4 has changed since this test's expected* "+
				"declarations were transcribed (checksum got %s, want %s). This test is FAILING "+
				"LOUDLY rather than silently passing against a stale copy of the contract: a human "+
				"must re-read the current §1–§4, update the expected* types/messages in this file, "+
				"and refresh expectedContractChecksum to match.",
			got, expectedContractChecksum)
	}
}

// ---- shared assertion helpers ----

type fieldSpec struct {
	Name string
	Type string
}

func assertStructFields(t *testing.T, label string, got reflect.Type, want []fieldSpec) {
	t.Helper()
	if got.Kind() != reflect.Struct {
		t.Fatalf("%s: not a struct (kind=%s)", label, got.Kind())
	}
	if got.NumField() != len(want) {
		t.Fatalf("%s: field count drifted from 05's frozen contract — got %d fields, expected %d", label, got.NumField(), len(want))
	}
	for i, w := range want {
		f := got.Field(i)
		if f.Name != w.Name {
			t.Errorf("%s: field %d name = %q, want %q (field order or name drifted from 05's contract)", label, i, f.Name, w.Name)
			continue
		}
		if f.Type.String() != w.Type {
			t.Errorf("%s: field %q type = %s, want %s", label, f.Name, f.Type.String(), w.Type)
		}
	}
}

func assertInterfaceEquivalent(t *testing.T, label string, got, want reflect.Type) {
	t.Helper()
	if got.Kind() != reflect.Interface {
		t.Fatalf("%s: actual type is not an interface (kind=%s)", label, got.Kind())
	}
	if want.Kind() != reflect.Interface {
		t.Fatalf("%s: expected type is not an interface (kind=%s)", label, want.Kind())
	}
	if got.NumMethod() != want.NumMethod() {
		t.Fatalf("%s: method count drifted from 05's frozen contract — got %d method(s), expected %d", label, got.NumMethod(), want.NumMethod())
	}
	// Mutual assignability of two interface types holds iff their method
	// sets are identical (same names, same signatures) — this is Go's own
	// interface-satisfaction rule, checked structurally by reflect, not
	// stringly.
	if !got.AssignableTo(want) {
		t.Fatalf("%s: does not satisfy the frozen §1/§3 contract shape (missing or resignatured method) — got=%s want=%s", label, got.String(), want.String())
	}
	if !want.AssignableTo(got) {
		t.Fatalf("%s: has grown a method beyond the frozen §1/§3 contract shape — got=%s want=%s", label, got.String(), want.String())
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
