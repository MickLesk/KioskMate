package admin

import (
	"path/filepath"
	"testing"

	"github.com/MickLesk/KioskMate/internal/config"
)

func TestMQTTTestUsesStoredPasswordWhenRequestLeavesItBlank(t *testing.T) {
	cfg := &config.Config{Path: filepath.Join(t.TempDir(), "config.json"), MQTT: config.MQTTConfig{Password: "stored-secret"}}
	server := &Server{cfg: cfg}
	body := mqttTestRequest{}
	server.applyStoredMQTTPassword(&body)
	if body.Password != "stored-secret" {
		t.Fatalf("password = %q", body.Password)
	}
	body.Password = "new-secret"
	server.applyStoredMQTTPassword(&body)
	if body.Password != "new-secret" {
		t.Fatalf("explicit password was overwritten: %q", body.Password)
	}
}
