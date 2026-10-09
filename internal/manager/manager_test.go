package manager

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMoveIntoPlace(t *testing.T) {
	dir := t.TempDir()
	stage := func(content string) string {
		s, _ := os.MkdirTemp(dir, "stage")
		os.WriteFile(filepath.Join(s, "bin"), []byte(content), 0o755)
		return s
	}
	dst := filepath.Join(dir, "0.2.0")
	// A leftover without the binary is replaced.
	os.MkdirAll(dst, 0o755)
	os.WriteFile(filepath.Join(dst, "junk"), nil, 0o644)
	if err := moveIntoPlace(stage("first"), dst, "bin"); err != nil {
		t.Fatal(err)
	}
	// A complete installation (by another process) is kept.
	if err := moveIntoPlace(stage("second"), dst, "bin"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dst, "bin")); string(got) != "first" {
		t.Fatalf("complete install was replaced: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dst, "junk")); !os.IsNotExist(err) {
		t.Fatal("leftover not removed")
	}
}
