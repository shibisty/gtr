package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunExitCodes(t *testing.T) {
	t.Setenv("GTR_HOME", t.TempDir())
	t.Setenv("GTR_GO_VERSION", "")
	t.Chdir(t.TempDir())
	t.Setenv("PATH", t.TempDir()) // no system Go for the shim to fall back to
	for _, tc := range []struct {
		argv []string
		code int
		out  string
	}{
		{[]string{"gtr"}, 0, "Usage:"},
		{[]string{"/usr/local/bin/gtr", "version"}, 0, "gtr dev"},
		{[]string{"gtr.exe", "self", "list"}, 0, "none installed"},
		{[]string{"gtr", "bogus"}, 2, "unknown command"},
		{[]string{"gtr", "self", "uninstall", "go", "1.0.0"}, 1, "Error: Go 1.0.0 is not installed"},
		{[]string{filepath.Join("bin", "go.exe"), "version"}, 1, "gtr (go shim): no Go version selected"},
	} {
		var out, errOut bytes.Buffer
		code := run(context.Background(), tc.argv, &out, &errOut)
		if code != tc.code || !strings.Contains(out.String()+errOut.String(), tc.out) {
			t.Errorf("%v: code %d, out %q %q", tc.argv, code, out.String(), errOut.String())
		}
	}
}

func TestNewApp(t *testing.T) {
	a := newApp(os.Stdout, os.Stderr)
	l, err := a.Home()
	if err != nil || a.NewGo(l) == nil || a.NewSource(l, "gtr", "x/y").App != "gtr" || a.Exec == nil {
		t.Fatal("newApp wiring")
	}
}
