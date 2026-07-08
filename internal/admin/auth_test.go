package admin

import "testing"

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
}

func TestPasswordHashRejectsInvalidFormat(t *testing.T) {
	if VerifyPassword("password", "invalid") {
		t.Fatal("invalid hash must not verify")
	}
}
