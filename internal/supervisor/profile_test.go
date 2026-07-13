package supervisor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestHardenChromiumProfileDisablesCredentialReplay(t *testing.T) {
	profile := t.TempDir()
	defaultDir := filepath.Join(profile, "Default")
	if err := os.MkdirAll(defaultDir, 0o700); err != nil {
		t.Fatal(err)
	}
	initial := `{"profile":{"name":"Kiosk"},"custom":{"preserved":true}}`
	if err := os.WriteFile(filepath.Join(defaultDir, "Preferences"), []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defaultDir, "Login Data"), []byte("stored-password"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := hardenChromiumProfile(profile); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(defaultDir, "Preferences"))
	if err != nil {
		t.Fatal(err)
	}
	var preferences map[string]any
	if err := json.Unmarshal(data, &preferences); err != nil {
		t.Fatal(err)
	}
	profileSettings := preferences["profile"].(map[string]any)
	sessionSettings := preferences["session"].(map[string]any)
	if preferences["credentials_enable_service"] != false || profileSettings["password_manager_enabled"] != false {
		t.Fatalf("credential replay is not disabled: %#v", preferences)
	}
	if sessionSettings["restore_on_startup"] != float64(4) {
		t.Fatalf("session restore is not disabled: %#v", sessionSettings)
	}
	if profileSettings["name"] != "Kiosk" || preferences["custom"].(map[string]any)["preserved"] != true {
		t.Fatalf("existing preferences were not preserved: %#v", preferences)
	}
	if _, err := os.Stat(filepath.Join(defaultDir, "Login Data")); !os.IsNotExist(err) {
		t.Fatalf("stored credential database was not quarantined: %v", err)
	}
	backups, err := filepath.Glob(filepath.Join(profile, "SessionBackups", "auth-safety-*", "Login Data"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("stored credential backup missing: %v %#v", err, backups)
	}
	if _, err := os.Stat(filepath.Join(profile, ".kioskmate-auth-safety-v1")); err != nil {
		t.Fatalf("auth safety migration marker missing: %v", err)
	}
}

func TestAuthenticationFailureDetection(t *testing.T) {
	if !homeAssistantAuthFailureFrame(`{"type":"auth_invalid","message":"Invalid access token"}`) {
		t.Fatal("auth_invalid websocket frame was not detected")
	}
	if homeAssistantAuthFailureFrame(`{"type":"auth_ok"}`) {
		t.Fatal("successful authentication was classified as a failure")
	}
	if !homeAssistantAuthFailureResponse(401, "XHR", "http://ha.local:8123/auth/token") {
		t.Fatal("Home Assistant 401 was not detected")
	}
	if homeAssistantAuthFailureResponse(401, "Image", "http://ha.local:8123/local/image.png") {
		t.Fatal("non-auth-relevant resources must not trip the guard")
	}
}
