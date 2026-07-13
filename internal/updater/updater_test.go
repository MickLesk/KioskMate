package updater

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
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
