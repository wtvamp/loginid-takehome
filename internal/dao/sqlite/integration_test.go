package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"loginid-takehome/internal/dao"
	"loginid-takehome/internal/model"
)

func strp(s string) *string { return &s }

func newTestRepo(t *testing.T) dao.Repository {
	db := setupDB(t)
	return &repository{db: db}
}

func mustCreateProfile(t *testing.T, ctx context.Context, repo dao.Repository, name string) *model.UserProfile {
	t.Helper()
	p, err := repo.Profiles().Create(ctx, &model.UserProfile{Name: name, Source: "direct"})
	if err != nil {
		t.Fatalf("creating profile: %v", err)
	}
	return p
}

func TestIntegration_ProfileCRUD(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	created := mustCreateProfile(t, ctx, repo, "Jane Doe")
	if created.ID == "" {
		t.Fatal("Create should populate an app-generated ID")
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Error("Create should populate timestamps")
	}

	got, err := repo.Profiles().Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Jane Doe" {
		t.Errorf("Get returned Name=%q, want %q", got.Name, "Jane Doe")
	}

	got.Name = "Jane Smith"
	got.Phone = strp("+15551234567")
	updated, err := repo.Profiles().Update(ctx, got)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "Jane Smith" || updated.Phone == nil || *updated.Phone != "+15551234567" {
		t.Errorf("Update did not persist changes: %+v", updated)
	}

	if err := repo.Profiles().Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.Profiles().Get(ctx, created.ID); !errors.Is(err, dao.ErrNotFound) {
		t.Errorf("Get after Delete = %v, want ErrNotFound", err)
	}
}

func TestIntegration_IDIsAppGeneratedAndStructurallyLikePostgres(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	created := mustCreateProfile(t, ctx, repo, "UUID Check")
	if len(created.ID) != 36 {
		t.Errorf("ID = %q, want a 36-char UUID string (same format as the server-generated Postgres path)", created.ID)
	}
}

func TestIntegration_CreateWithCallerSuppliedID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	_, err := repo.Profiles().Create(ctx, &model.UserProfile{ID: "caller-supplied", Name: "X", Source: "direct"})
	if !errors.Is(err, dao.ErrInvalidArgument) {
		t.Errorf("Create with non-empty ID = %v, want ErrInvalidArgument", err)
	}
}

func TestIntegration_UpdateMissingRow(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	_, err := repo.Profiles().Update(ctx, &model.UserProfile{ID: "missing", Name: "X", Source: "direct"})
	if !errors.Is(err, dao.ErrNotFound) {
		t.Errorf("Update on missing row = %v, want ErrNotFound", err)
	}
}

func TestIntegration_UpsertExcludesCreatedAtFromUpdate(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	created := mustCreateProfile(t, ctx, repo, "Rehydrate Me")
	originalCreatedAt := created.CreatedAt

	time.Sleep(1100 * time.Millisecond) // RFC3339 has second resolution — need >1s to detect a regression

	rehydrated, err := repo.Profiles().Upsert(ctx, &model.UserProfile{
		ID: created.ID, Name: "Rehydrate Me Updated", Source: "idp_cache",
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if !rehydrated.CreatedAt.Equal(originalCreatedAt) {
		t.Errorf("Upsert changed CreatedAt from %v to %v — created_at must be excluded from the update", originalCreatedAt, rehydrated.CreatedAt)
	}
	if rehydrated.Name != "Rehydrate Me Updated" {
		t.Errorf("Upsert did not apply the update: %+v", rehydrated)
	}
}

func TestIntegration_UpsertInsertsWhenAbsent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	result, err := repo.Profiles().Upsert(ctx, &model.UserProfile{
		ID: "11111111-1111-1111-1111-111111111111", Name: "New Via Upsert", Source: "idp_cache",
	})
	if err != nil {
		t.Fatalf("Upsert (insert path): %v", err)
	}
	if result.Name != "New Via Upsert" {
		t.Errorf("Upsert insert path: got %+v", result)
	}
}

func TestIntegration_SearchLikeEscaping(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	mustCreateProfile(t, ctx, repo, "50% Off Corp")
	mustCreateProfile(t, ctx, repo, "500 Off Corp")

	results, total, err := repo.Profiles().Search(ctx, dao.ProfileQuery{Name: strp("50%")})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("Search(%q) matched %d rows, want exactly 1 — escaping may not be applied", "50%", total)
	}
	if results[0].Name != "50% Off Corp" {
		t.Errorf("Search matched the wrong row: %+v", results[0])
	}
}

func TestIntegration_SearchCaseFoldingASCII(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	mustCreateProfile(t, ctx, repo, "UPPERCASE NAME")

	_, total, err := repo.Profiles().Search(ctx, dao.ProfileQuery{Name: strp("uppercase")})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if total != 1 {
		t.Errorf("case-insensitive search for ASCII input matched %d, want 1", total)
	}
}

func TestIntegration_SearchPagination(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		mustCreateProfile(t, ctx, repo, "Pagination Test")
	}

	page1, total, err := repo.Profiles().Search(ctx, dao.ProfileQuery{Name: strp("Pagination Test"), Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(page1) != 2 {
		t.Errorf("page1 len = %d, want 2", len(page1))
	}
}

func TestIntegration_SearchEmptyQueryIsLegal(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	mustCreateProfile(t, ctx, repo, "Anyone")

	_, total, err := repo.Profiles().Search(ctx, dao.ProfileQuery{})
	if err != nil {
		t.Fatalf("an all-nil query should be legal, got %v", err)
	}
	if total < 1 {
		t.Errorf("expected at least 1 row, got %d", total)
	}
}

func TestIntegration_SearchInvalidQuery(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	_, _, err := repo.Profiles().Search(ctx, dao.ProfileQuery{Name: strp("")})
	if !errors.Is(err, dao.ErrInvalidQuery) {
		t.Errorf("empty-string Name filter = %v, want ErrInvalidQuery", err)
	}
}

func TestIntegration_CredentialCRUDAndErrorTranslation(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	methodID := seedAuthMethodID()

	profile := mustCreateProfile(t, ctx, repo, "Cred Owner")

	cred, err := repo.Credentials().Create(ctx, &model.UserCredential{
		UserID: profile.ID, Username: "alice", MethodID: methodID, SecretState: "none",
	})
	if err != nil {
		t.Fatalf("Create credential: %v", err)
	}

	got, err := repo.Credentials().GetByUsername(ctx, "ALICE")
	if err != nil {
		t.Fatalf("GetByUsername (case-folded): %v", err)
	}
	if got.ID != cred.ID {
		t.Error("GetByUsername returned the wrong credential")
	}

	_, err = repo.Credentials().Create(ctx, &model.UserCredential{
		UserID: profile.ID, Username: "Alice", MethodID: methodID, SecretState: "none",
	})
	if !errors.Is(err, dao.ErrDuplicateUsername) {
		t.Errorf("duplicate username = %v, want ErrDuplicateUsername", err)
	}

	_, err = repo.Credentials().Create(ctx, &model.UserCredential{
		UserID: profile.ID, Username: "bob", MethodID: "nonexistent-method-id", SecretState: "none",
	})
	if !errors.Is(err, dao.ErrInvalidMethod) {
		t.Errorf("unknown method_id = %v, want ErrInvalidMethod", err)
	}

	_, err = repo.Credentials().Create(ctx, &model.UserCredential{
		UserID: profile.ID, Username: "carol", MethodID: methodID, SecretState: "set",
	})
	if !errors.Is(err, dao.ErrInvalidCredential) {
		t.Errorf("secret_state=set with no secret = %v, want ErrInvalidCredential", err)
	}

	list, err := repo.Credentials().ListByUserID(ctx, profile.ID)
	if err != nil {
		t.Fatalf("ListByUserID: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("ListByUserID len = %d, want 1", len(list))
	}
}

func TestIntegration_ListByUserID_EmptyNotError(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	list, err := repo.Credentials().ListByUserID(ctx, "no-such-user")
	if err != nil {
		t.Fatalf("ListByUserID for a user with no credentials should not error, got %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty slice, got %d", len(list))
	}
}

func TestIntegration_CreateProfileWithCredential_Atomic(t *testing.T) {
	db := setupDB(t)
	repo := &repository{db: db}
	methodID := seedAuthMethodID()
	ctx := context.Background()

	_, _, err := repo.CreateProfileWithCredential(ctx,
		&model.UserProfile{Name: "Dave One", Source: "direct"},
		&model.UserCredential{UserID: "ignored-on-input", Username: "dave", MethodID: methodID, SecretState: "none"},
	)
	if err != nil {
		t.Fatalf("first CreateProfileWithCredential: %v", err)
	}

	var countBefore int
	if err := db.QueryRow("SELECT COUNT(*) FROM user_profile").Scan(&countBefore); err != nil {
		t.Fatalf("counting profiles: %v", err)
	}

	_, _, err = repo.CreateProfileWithCredential(ctx,
		&model.UserProfile{Name: "Dave Two (should not persist)", Source: "direct"},
		&model.UserCredential{Username: "dave", MethodID: methodID, SecretState: "none"},
	)
	if !errors.Is(err, dao.ErrDuplicateUsername) {
		t.Fatalf("expected ErrDuplicateUsername, got %v", err)
	}

	var countAfter int
	if err := db.QueryRow("SELECT COUNT(*) FROM user_profile").Scan(&countAfter); err != nil {
		t.Fatalf("counting profiles: %v", err)
	}
	if countAfter != countBefore {
		t.Errorf("profile count changed from %d to %d after a failed CreateProfileWithCredential", countBefore, countAfter)
	}
}

func TestIntegration_CreateProfileWithCredential_IgnoresCallerSuppliedUserID(t *testing.T) {
	db := setupDB(t)
	repo := &repository{db: db}
	methodID := seedAuthMethodID()
	ctx := context.Background()

	_, cred, err := repo.CreateProfileWithCredential(ctx,
		&model.UserProfile{Name: "Erin", Source: "direct"},
		&model.UserCredential{UserID: "attacker-supplied-mismatch", Username: "erin", MethodID: methodID, SecretState: "none"},
	)
	if err != nil {
		t.Fatalf("CreateProfileWithCredential: %v", err)
	}
	if cred.UserID == "attacker-supplied-mismatch" {
		t.Error("c.UserID must be ignored on input and set from the newly created profile")
	}
}

func TestIntegration_DeleteExpiredAndOldestSurviving(t *testing.T) {
	db := setupDB(t)
	repo := &repository{db: db}
	ctx := context.Background()

	old, err := repo.Profiles().Create(ctx, &model.UserProfile{Name: "Old Direct Row", Source: "direct"})
	if err != nil {
		t.Fatalf("creating profile: %v", err)
	}
	if _, err := db.Exec("UPDATE user_profile SET updated_at = ? WHERE id = ?", "2000-01-01T00:00:00Z", old.ID); err != nil {
		t.Fatalf("backdating updated_at: %v", err)
	}

	young, err := repo.Profiles().Create(ctx, &model.UserProfile{Name: "Young Direct Row", Source: "direct"})
	if err != nil {
		t.Fatalf("creating profile: %v", err)
	}

	result, err := repo.DeleteExpired(ctx, dao.RetentionDirect, time.Now().Add(-365*24*time.Hour), 100)
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if result.RowsDeleted != 1 {
		t.Errorf("RowsDeleted = %d, want 1", result.RowsDeleted)
	}
	if !result.Drained {
		t.Error("Drained = false, want true")
	}
	if result.OldestSurvivingAt == nil {
		t.Fatal("OldestSurvivingAt is nil, want the young row's updated_at")
	}

	if _, err := repo.Profiles().Get(ctx, old.ID); !errors.Is(err, dao.ErrNotFound) {
		t.Errorf("old row should be deleted, Get returned %v", err)
	}
	if _, err := repo.Profiles().Get(ctx, young.ID); err != nil {
		t.Errorf("young row should survive, Get returned %v", err)
	}

	var logCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM deletion_log WHERE profile_id = ? AND reason = 'retention_sweep'", old.ID).Scan(&logCount); err != nil {
		t.Fatalf("checking deletion_log: %v", err)
	}
	if logCount != 1 {
		t.Errorf("expected one deletion_log row, got %d", logCount)
	}
}

func TestIntegration_DeleteExpired_MaxRowsBounds(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	_, err := repo.DeleteExpired(ctx, dao.RetentionDirect, time.Now(), 0)
	if !errors.Is(err, dao.ErrInvalidArgument) {
		t.Errorf("maxRows=0 = %v, want ErrInvalidArgument", err)
	}
	_, err = repo.DeleteExpired(ctx, dao.RetentionDirect, time.Now(), 10_001)
	if !errors.Is(err, dao.ErrInvalidArgument) {
		t.Errorf("maxRows=10001 = %v, want ErrInvalidArgument", err)
	}
}

func TestIntegration_DeleteProfile_SubjectRequest(t *testing.T) {
	db := setupDB(t)
	repo := &repository{db: db}
	ctx := context.Background()

	profile, err := repo.Profiles().Create(ctx, &model.UserProfile{Name: "Subject", Source: "direct"})
	if err != nil {
		t.Fatalf("creating profile: %v", err)
	}

	ref := "ticket-123"
	if err := repo.DeleteProfile(ctx, profile.ID, &ref); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}

	if _, err := repo.Profiles().Get(ctx, profile.ID); !errors.Is(err, dao.ErrNotFound) {
		t.Errorf("profile should be deleted, Get returned %v", err)
	}

	var reason string
	var gotRef *string
	if err := db.QueryRow("SELECT reason, external_ref FROM deletion_log WHERE profile_id = ?", profile.ID).Scan(&reason, &gotRef); err != nil {
		t.Fatalf("checking deletion_log: %v", err)
	}
	if reason != "subject_request" {
		t.Errorf("reason = %q, want %q", reason, "subject_request")
	}
	if gotRef == nil || *gotRef != ref {
		t.Errorf("external_ref = %v, want %q", gotRef, ref)
	}
}

func TestIntegration_DeleteProfile_NotFound(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	err := repo.DeleteProfile(ctx, "missing", nil)
	if !errors.Is(err, dao.ErrNotFound) {
		t.Errorf("DeleteProfile on missing row = %v, want ErrNotFound", err)
	}
}

func TestIntegration_ProfileDeleteCascadesToCredentials(t *testing.T) {
	repo := newTestRepo(t)
	methodID := seedAuthMethodID()
	ctx := context.Background()

	profile := mustCreateProfile(t, ctx, repo, "Cascade Test")
	_, err := repo.Credentials().Create(ctx, &model.UserCredential{
		UserID: profile.ID, Username: "cascadetest", MethodID: methodID, SecretState: "none",
	})
	if err != nil {
		t.Fatalf("creating credential: %v", err)
	}

	if err := repo.Profiles().Delete(ctx, profile.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	list, err := repo.Credentials().ListByUserID(ctx, profile.ID)
	if err != nil {
		t.Fatalf("ListByUserID: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("credential survived profile deletion — ON DELETE CASCADE not applied")
	}
}
