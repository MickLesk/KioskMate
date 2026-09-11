package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/supervisor"
)

func TestWriteSoakReport(t *testing.T) {
	cfg := &config.Config{Path: filepath.Join(t.TempDir(), "config.json")}
	var output bytes.Buffer
	if err := writeSoakReport(&output, cfg); err != nil {
		t.Fatal(err)
	}
	var report supervisor.SoakReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "collecting" || report.RequiredSeconds != int64((24*time.Hour).Seconds()) {
		t.Fatalf("report = %#v", report)
	}
}
