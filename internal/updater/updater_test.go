package updater

import (
	"os"
	"path/filepath"
	"testing"
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
