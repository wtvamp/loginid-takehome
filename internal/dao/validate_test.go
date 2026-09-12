package dao

import (
	"testing"

	"loginid-takehome/internal/model"
)

func TestValidateCreateID(t *testing.T) {
	if err := ValidateCreateID(""); err != nil {
		t.Errorf("empty ID should be valid, got %v", err)
	}
	if err := ValidateCreateID("caller-supplied-id"); err != ErrInvalidArgument {
		t.Errorf("non-empty ID should be ErrInvalidArgument, got %v", err)
	}
}

func ptr(s string) *string { return &s }

func TestValidateProfilePointers_Phone(t *testing.T) {
	if err := ValidateProfilePointers(&model.UserProfile{Phone: nil}); err != nil {
		t.Errorf("nil Phone should be valid (absent), got %v", err)
	}
	if err := ValidateProfilePointers(&model.UserProfile{Phone: ptr("+15551234567")}); err != nil {
		t.Errorf("non-empty Phone should be valid, got %v", err)
	}
	if err := ValidateProfilePointers(&model.UserProfile{Phone: ptr("")}); err != ErrInvalidArgument {
		t.Errorf("empty-string Phone should be ErrInvalidArgument, got %v", err)
	}
}

func TestValidateProfilePointers_StreetAddress(t *testing.T) {
	if err := ValidateProfilePointers(&model.UserProfile{StreetAddress: ptr("")}); err != ErrInvalidArgument {
		t.Errorf("empty-string StreetAddress should be ErrInvalidArgument, got %v", err)
	}
}

func TestValidateProfilePointers_Locality(t *testing.T) {
	if err := ValidateProfilePointers(&model.UserProfile{Locality: ptr("")}); err != ErrInvalidArgument {
		t.Errorf("empty-string Locality should be ErrInvalidArgument, got %v", err)
	}
}

func TestValidateProfilePointers_Region(t *testing.T) {
	if err := ValidateProfilePointers(&model.UserProfile{Region: ptr("")}); err != ErrInvalidArgument {
		t.Errorf("empty-string Region should be ErrInvalidArgument, got %v", err)
	}
}

func TestValidateProfilePointers_PostalCode(t *testing.T) {
	if err := ValidateProfilePointers(&model.UserProfile{PostalCode: ptr("")}); err != ErrInvalidArgument {
		t.Errorf("empty-string PostalCode should be ErrInvalidArgument, got %v", err)
	}
}

func TestValidateProfilePointers_Country(t *testing.T) {
	if err := ValidateProfilePointers(&model.UserProfile{Country: ptr("")}); err != ErrInvalidArgument {
		t.Errorf("empty-string Country should be ErrInvalidArgument, got %v", err)
	}
}

func TestValidateCredentialPointers_HashAlgo(t *testing.T) {
	if err := ValidateCredentialPointers(&model.UserCredential{HashAlgo: nil}); err != nil {
		t.Errorf("nil HashAlgo should be valid (absent), got %v", err)
	}
	if err := ValidateCredentialPointers(&model.UserCredential{HashAlgo: ptr("argon2id")}); err != nil {
		t.Errorf("non-empty HashAlgo should be valid, got %v", err)
	}
	if err := ValidateCredentialPointers(&model.UserCredential{HashAlgo: ptr("")}); err != ErrInvalidArgument {
		t.Errorf("empty-string HashAlgo should be ErrInvalidArgument, got %v", err)
	}
}

func TestPrepareCreateProfileWithCredential_ClearsCallerSuppliedUserID(t *testing.T) {
	c := &model.UserCredential{UserID: "attacker-supplied-user-id"}
	PrepareCreateProfileWithCredential(c)
	if c.UserID != "" {
		t.Errorf("UserID = %q, want empty — caller-supplied UserID must be cleared before the backend sets it from the newly created profile", c.UserID)
	}
}
