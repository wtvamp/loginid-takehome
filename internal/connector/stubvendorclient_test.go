package connector

import (
	"context"
	"testing"
)

func TestStubVendorClient_AuthenticateThenFetchIdentity(t *testing.T) {
	stub := NewStubVendorClient()
	stub.Seed("demo-user", "demo-password", Identity{Name: "Jane Demo", Country: "US"})

	token, err := stub.Authenticate(context.Background(), "demo-user", "demo-password")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if token == "" {
		t.Fatalf("Authenticate returned an empty token")
	}

	identity, err := stub.FetchIdentity(context.Background(), token, "ignored-phone", "ignored-name")
	if err != nil {
		t.Fatalf("FetchIdentity: %v", err)
	}
	if identity.Name != "Jane Demo" || identity.Country != "US" {
		t.Errorf("unexpected identity: %+v", identity)
	}
}

func TestStubVendorClient_WrongPassword(t *testing.T) {
	stub := NewStubVendorClient()
	stub.Seed("demo-user", "demo-password", Identity{Name: "Jane Demo"})

	if _, err := stub.Authenticate(context.Background(), "demo-user", "wrong-password"); err != ErrVendorAuthFailed {
		t.Errorf("err = %v, want ErrVendorAuthFailed", err)
	}
}

func TestStubVendorClient_UnknownUsername(t *testing.T) {
	stub := NewStubVendorClient()
	if _, err := stub.Authenticate(context.Background(), "nobody", "whatever"); err != ErrVendorAuthFailed {
		t.Errorf("err = %v, want ErrVendorAuthFailed — must not distinguish unknown username from wrong password", err)
	}
}

func TestStubVendorClient_FetchIdentity_UnknownToken(t *testing.T) {
	stub := NewStubVendorClient()
	if _, err := stub.FetchIdentity(context.Background(), "never-issued-token", "phone", "name"); err != ErrVendorIdentityNotFound {
		t.Errorf("err = %v, want ErrVendorIdentityNotFound", err)
	}
}

func TestStubVendorClient_DistinctTokensPerAuthenticateCall(t *testing.T) {
	stub := NewStubVendorClient()
	stub.Seed("demo-user", "demo-password", Identity{Name: "Jane Demo"})

	t1, err := stub.Authenticate(context.Background(), "demo-user", "demo-password")
	if err != nil {
		t.Fatalf("first Authenticate: %v", err)
	}
	t2, err := stub.Authenticate(context.Background(), "demo-user", "demo-password")
	if err != nil {
		t.Fatalf("second Authenticate: %v", err)
	}
	if t1 == t2 {
		t.Errorf("two Authenticate calls returned the same token %q, want distinct tokens per call", t1)
	}
}
