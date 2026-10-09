package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoEnv(t *testing.T) {
	base := []string{"PATH=/usr/bin", "GOROOT=/old", "GTR_GO_VERSION=1.0.0", "HOME=/h"}
	env := GoEnv(base, "/g", "1.26.9")
	joined := strings.Join(env, "\n")
	for _, want := range []string{"PATH=" + filepath.Join("/g", "bin") + string(os.PathListSeparator) + "/usr/bin", "GOROOT=/g", "GTR_GO_VERSION=1.26.9", "GOTOOLCHAIN=local", "HOME=/h"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %s in %v", want, env)
		}
	}
	if strings.Contains(joined, "/old") || strings.Contains(joined, "1.0.0") {
		t.Errorf("old values kept: %v", env)
	}
	if env := GoEnv([]string{"GOTOOLCHAIN=auto"}, "/g", "1"); !strings.Contains(strings.Join(env, " "), "GOTOOLCHAIN=auto") ||
		!strings.Contains(strings.Join(env, " "), "PATH="+filepath.Join("/g", "bin")) {
		t.Errorf("explicit GOTOOLCHAIN must stay, PATH must be added: %v", env)
	}
	if got := GoEnv(base, "", ""); len(got) != len(base) {
		t.Error("empty goroot must not change env")
	}
}

// TestHelper is run as a child process by TestRun.
func TestHelper(t *testing.T) {
	if os.Getenv("GTR_RUNNER_HELPER") != "1" {
		t.Skip("helper process")
	}
	if os.Getenv("WANT") != "yes" {
		os.Exit(9)
	}
	os.Exit(4)
}

func TestRun(t *testing.T) {
	self, _ := os.Executable()
	code, err := Run(self, []string{"-test.run=^TestHelper$"}, append(os.Environ(), "GTR_RUNNER_HELPER=1", "WANT=yes"))
	if err != nil || code != 4 {
		t.Fatalf("exit code: %d %v", code, err)
	}
	if _, err := Run(filepath.Join(t.TempDir(), "missing"), nil, nil); err == nil {
		t.Fatal("missing binary must be an error")
	}
	code, err = Run(self, []string{"-test.run=^TestNothing$"}, os.Environ())
	if err != nil || code != 0 {
		t.Fatalf("success: %d %v", code, err)
	}
}
