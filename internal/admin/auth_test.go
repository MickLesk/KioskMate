package admin

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MickLesk/KioskMate/internal/config"
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

func TestLoginResponseBootstrapsAuthenticatedUIWithoutRuntimeStatus(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	hash, err := HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Mutate(func(next *config.Config) error {
		next.Admin.PasswordHash = hash
		next.MQTT.Password = "secret"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(cfg, &panicStatusBrowser{}, nil, nil, nil, nil, "0.7.2", slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"password":"test-password"}`))
	rec := httptest.NewRecorder()

	server.authLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Authenticated bool          `json:"authenticated"`
		Version       string        `json:"version"`
		Config        config.Config `json:"config"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Authenticated || body.Version != "0.7.2" {
		t.Fatalf("unexpected bootstrap response: %#v", body)
	}
	if body.Config.Admin.PasswordHash != "" || body.Config.MQTT.Password != "" {
		t.Fatal("login response exposed private credentials")
	}
	if body.Config.Kiosk.ZoomPercent == 0 {
		t.Fatal("login response is missing the public Admin configuration")
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("login response did not create an Admin session")
	}
	statusReq := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	statusReq.AddCookie(cookies[0])
	statusRec := httptest.NewRecorder()
	server.authStatus(statusRec, statusReq)
	var statusBody struct {
		Authenticated bool           `json:"authenticated"`
		Config        *config.Config `json:"config"`
	}
	if err := json.Unmarshal(statusRec.Body.Bytes(), &statusBody); err != nil {
		t.Fatal(err)
	}
	if !statusBody.Authenticated || statusBody.Config == nil || statusBody.Config.Kiosk.ZoomPercent == 0 {
		t.Fatalf("session bootstrap is incomplete: %#v", statusBody)
	}
}
