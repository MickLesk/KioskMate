package admin

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestAuthStatusReportsRateLimitAndAttemptMapIsBounded(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(cfg, &fakeActionBrowser{}, nil, nil, nil, nil, "test", slog.Default())
	now := time.Now()
	server.attempts["192.0.2.10"] = make([]time.Time, loginAttemptLimit)
	for index := range server.attempts["192.0.2.10"] {
		server.attempts["192.0.2.10"][index] = now
	}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	rec := httptest.NewRecorder()
	server.authStatus(rec, req)
	var status struct {
		RateLimited       bool `json:"rateLimited"`
		RetryAfterSeconds int  `json:"retryAfterSeconds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.RateLimited || status.RetryAfterSeconds <= 0 {
		t.Fatalf("unexpected rate limit status: %#v", status)
	}
	for index := 0; index < maxAttemptClients+20; index++ {
		failed := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
		failed.RemoteAddr = fmt.Sprintf("198.51.100.%d:1234", index)
		server.recordFailedAttempt(failed)
	}
	if len(server.attempts) > maxAttemptClients {
		t.Fatalf("attempt map has %d clients, want at most %d", len(server.attempts), maxAttemptClients)
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
	server := NewServer(cfg, &fakeActionBrowser{}, nil, nil, nil, nil, "0.7.2", slog.Default())
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
		CSRF          string        `json:"csrf"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Authenticated || body.Version != "0.7.2" {
		t.Fatalf("unexpected bootstrap response: %#v", body)
	}
	if body.CSRF == "" {
		t.Fatal("login response is missing CSRF token")
	}
	if body.Config.Admin.PasswordHash != "" || body.Config.MQTT.Password != "" {
		t.Fatal("login response exposed private credentials")
	}
	if !body.Config.MQTT.PasswordConfigured {
		t.Fatal("login response should indicate that an MQTT password is configured")
	}
	if body.Config.Kiosk.ZoomPercent == 0 {
		t.Fatal("login response is missing the public Admin configuration")
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("login response did not create an Admin session")
	}
	if _, exposed := server.sessions[cookies[0].Value]; exposed {
		t.Fatal("raw session token was stored as a map key")
	}
	if _, ok := server.sessions[sessionKey(cookies[0].Value)]; !ok {
		t.Fatal("hashed session token was not stored")
	}
	statusReq := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	statusReq.AddCookie(cookies[0])
	statusRec := httptest.NewRecorder()
	server.authStatus(statusRec, statusReq)
	var statusBody struct {
		Authenticated bool           `json:"authenticated"`
		Config        *config.Config `json:"config"`
		CSRF          string         `json:"csrf"`
	}
	if err := json.Unmarshal(statusRec.Body.Bytes(), &statusBody); err != nil {
		t.Fatal(err)
	}
	if !statusBody.Authenticated || statusBody.Config == nil || statusBody.Config.Kiosk.ZoomPercent == 0 || statusBody.CSRF != body.CSRF {
		t.Fatalf("session bootstrap is incomplete: %#v", statusBody)
	}

	protected := server.auth(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	missingReq := httptest.NewRequest(http.MethodPost, "/protected", nil)
	missingReq.AddCookie(cookies[0])
	missingRec := httptest.NewRecorder()
	protected(missingRec, missingReq)
	if missingRec.Code != http.StatusForbidden {
		t.Fatalf("mutation without CSRF = %d, want 403", missingRec.Code)
	}
	validReq := httptest.NewRequest(http.MethodPost, "/protected", nil)
	validReq.AddCookie(cookies[0])
	validReq.Header.Set("X-KioskMate-CSRF", body.CSRF)
	validRec := httptest.NewRecorder()
	protected(validRec, validReq)
	if validRec.Code != http.StatusNoContent {
		t.Fatalf("mutation with CSRF = %d, want 204", validRec.Code)
	}
}

func TestAuthenticatedSessionSurvivesServerRestart(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	hash, err := HashPassword("persistent-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Mutate(func(next *config.Config) error {
		next.Admin.PasswordHash = hash
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	first := NewServer(cfg, &fakeActionBrowser{}, nil, nil, nil, nil, "test", slog.Default())
	login := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"password":"persistent-password"}`))
	response := httptest.NewRecorder()
	first.authLogin(response, login)
	if response.Code != http.StatusOK || len(response.Result().Cookies()) == 0 {
		t.Fatalf("login status=%d body=%s", response.Code, response.Body.String())
	}
	cookie := response.Result().Cookies()[0]
	if _, err := os.Stat(first.sessionsFilePath()); err != nil {
		t.Fatalf("persistent session file missing: %v", err)
	}

	second := NewServer(cfg, &fakeActionBrowser{}, nil, nil, nil, nil, "test", slog.Default())
	statusRequest := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	statusRequest.AddCookie(cookie)
	statusResponse := httptest.NewRecorder()
	second.authStatus(statusResponse, statusRequest)
	var status struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Authenticated {
		t.Fatal("persisted Admin session was not restored after server restart")
	}
}
