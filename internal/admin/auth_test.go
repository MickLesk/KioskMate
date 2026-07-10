package admin

import (
	"strings"
	"testing"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("long-enough-password")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("long-enough-password", hash) {
		t.Fatal("expected password to verify")
	}
	if VerifyPassword("wrong-password", hash) {
		t.Fatal("expected wrong password to fail")
	}
	if !strings.HasPrefix(hash, "argon2id$") {
		t.Fatalf("hash = %q, want Argon2id", hash)
	}
}

func TestPasswordHashRejectsInvalidFormat(t *testing.T) {
	if VerifyPassword("password", "invalid") {
		t.Fatal("invalid hash must not verify")
	}
}
