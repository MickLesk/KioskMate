package admin

import (
	"encoding/base64"
	"fmt"
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

func TestLegacyPasswordHashStillVerifies(t *testing.T) {
	salt := []byte("0123456789abcdef")
	hash := passwordDigest([]byte("legacy-password"), salt, 120000)
	encoded := fmt.Sprintf("sha256iter$120000$%s$%s", base64.RawURLEncoding.EncodeToString(salt), base64.RawURLEncoding.EncodeToString(hash))
	if !VerifyPassword("legacy-password", encoded) {
		t.Fatal("legacy password hash must remain valid for migration")
	}
}

func TestPasswordHashRejectsInvalidFormat(t *testing.T) {
	if VerifyPassword("password", "invalid") {
		t.Fatal("invalid hash must not verify")
	}
}
