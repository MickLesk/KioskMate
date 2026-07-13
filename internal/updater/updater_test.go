package updater

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
)

func TestVerifyRequiresDigest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "package.deb")
	if err := os.WriteFile(path, []byte("package"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verify(path, ""); err == nil {
		t.Fatal("expected missing digest to be rejected")
	}
}

func TestBuildPrivilegedCommandUsesNonInteractiveSudo(t *testing.T) {
	command, args, input, err := buildPrivilegedCommand(privilegeCommand{Mode: "sudo"}, "apt-get", "install", "-y", "/tmp/package.deb")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-n", "apt-get", "install", "-y", "/tmp/package.deb"}
	if command != "sudo" || !reflect.DeepEqual(args, want) || input != "" {
		t.Fatalf("command = %q, args = %#v, input = %q", command, args, input)
	}
}

func TestBuildPrivilegedCommandDoesNotPutPasswordInArguments(t *testing.T) {
	command, args, input, err := buildPrivilegedCommand(privilegeCommand{Mode: "sudo", Password: "secret"}, "apt-get", "install", "-y", "/tmp/package.deb")
	if err != nil {
		t.Fatal(err)
	}
	if command != "sudo" || input != "secret\n" {
		t.Fatalf("command = %q, input = %q", command, input)
	}
	for _, arg := range args {
		if arg == "secret" {
			t.Fatal("password leaked into process arguments")
		}
	}
}

func TestStoreCheckPreservesLastSuccessfulReleaseOnError(t *testing.T) {
	service := &Service{version: "0.3.1", status: ReleaseInfo{LatestVersion: "0.4.0", UpdateAvailable: true, URL: "https://example.test/release"}}
	service.storeCheck(ReleaseInfo{}, errors.New("offline"))
	status := service.Status()
	if status.LatestVersion != "0.4.0" || !status.UpdateAvailable || status.URL == "" || status.Error != "offline" || status.CheckedAt == nil {
		t.Fatalf("status = %#v", status)
	}
}

func TestJobSnapshotIncludesUpdateLifecycle(t *testing.T) {
	now := time.Now()
	job := &Job{ID: "1", Name: "update-install", Stage: "installing", Target: "0.3.1", Error: "failed", Started: now, ExitCode: 1, Output: []string{"line"}}
	snapshot := job.snapshot()
	if snapshot.Name != job.Name || snapshot.Stage != job.Stage || snapshot.Target != job.Target || snapshot.Error != job.Error || !reflect.DeepEqual(snapshot.Output, job.Output) {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestHistoryFinalizesInstalledVersionAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Path: path}
	service := New(cfg, "0.3.1")
	job := &Job{ID: "update-1", Name: "update-install", Stage: "restarting", Target: "0.4.0", Started: time.Now(), ExitCode: 0}
	service.beginHistory(job, "install", "0.4.0")
	service.updateHistory(job, "restarting")

	restarted := New(cfg, "0.4.0")
	history := restarted.History()
	if len(history.Entries) != 1 || history.Entries[0].Status != "installed" || history.RollbackTarget != "0.3.1" || !history.RollbackAvailable {
		t.Fatalf("history = %#v", history)
	}
}

func TestHistoryMarksUnexpectedVersionAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Path: path}
	service := New(cfg, "0.3.1")
	job := &Job{ID: "update-1", Name: "update-install", Stage: "restarting", Target: "0.4.0", Started: time.Now(), ExitCode: 0}
	service.beginHistory(job, "install", "0.4.0")
	service.updateHistory(job, "restarting")

	restarted := New(cfg, "0.3.1")
	history := restarted.History()
	if history.Entries[0].Status != "failed" || history.Entries[0].Error == "" {
		t.Fatalf("history = %#v", history)
	}
}

func TestBackupConfigCreatesPrivateRecoveryCopy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{\"version\":2}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := New(&config.Config{Path: path}, "0.3.1")
	job := &Job{ID: "update-1", Started: time.Now(), ExitCode: -1}
	service.beginHistory(job, "install", "0.4.0")
	backup, err := service.backupConfig(job)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(backup)
	if err != nil || string(data) != "{\"version\":2}\n" {
		t.Fatalf("backup = %q, err = %v", data, err)
	}
}

func TestRequiredDiskBytesIncludesPackageExpansionAndReserve(t *testing.T) {
	if got, want := requiredDiskBytes(10<<20), int64(94<<20); got != want {
		t.Fatalf("requiredDiskBytes = %d, want %d", got, want)
	}
}

func TestParseDPKGHistoryFindsPreviousInstalledVersion(t *testing.T) {
	log := []byte("2026-07-13 10:00:00 upgrade other:arm64 1.0 2.0\n2026-07-13 10:01:00 upgrade kioskmate:arm64 0.3.1 0.4.0\n")
	entry, ok := parseDPKGHistory(log, "0.4.0")
	if !ok || entry.FromVersion != "0.3.1" || entry.Target != "0.4.0" || entry.Status != "installed" {
		t.Fatalf("entry = %#v, ok = %v", entry, ok)
	}
}

func TestParseDPKGHistoryIgnoresDifferentCurrentVersion(t *testing.T) {
	log := []byte("2026-07-13 10:01:00 upgrade kioskmate:arm64 0.3.1 0.4.0\n")
	if entry, ok := parseDPKGHistory(log, "0.5.0"); ok {
		t.Fatalf("unexpected entry = %#v", entry)
	}
}

func TestFinishInstallKeepsLockDuringRestartWindow(t *testing.T) {
	service := &Service{installing: true}
	job := &Job{Stage: "restarting", ExitCode: 0}
	service.finishInstallJob(job)
	if !service.installing {
		t.Fatal("install lock was released before service restart")
	}
}

func TestFinishInstallReleasesLockAfterFailure(t *testing.T) {
	service := &Service{installing: true}
	job := &Job{Stage: "installing", ExitCode: 1}
	service.finishInstallJob(job)
	if service.installing {
		t.Fatal("install lock was not released after failed installation")
	}
}
