package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeGtr records its arguments, so the test sees `gtr self shims`.
const fakeGtr = "#!/bin/sh\necho \"$@\" >> \"$GTR_HOME/calls.log\"\n"

func releaseArchive(t *testing.T, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range map[string]string{"./gtr": fakeGtr + "# " + version + "\n", "./README.md": "readme"} {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg})
		tw.Write([]byte(content))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// TestInstallScript runs install.sh against a fake GitHub.
func TestInstallScript(t *testing.T) {
	if testing.Short() {
		t.Skip("end-to-end test")
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("install.sh is for Linux and macOS")
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("no release for " + runtime.GOARCH)
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	script, err := filepath.Abs("../../install.sh")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	var badSum atomic.Bool
	for _, v := range []string{"0.2.0", "0.3.0"} {
		asset := fmt.Sprintf("gtr_%s_%s_%s.tar.gz", v, runtime.GOOS, runtime.GOARCH)
		data := releaseArchive(t, v)
		sum := sha256.Sum256(data)
		files["/dl/v"+v+"/"+asset] = data
		files["/dl/v"+v+"/SHA256SUMS"] = []byte(hex.EncodeToString(sum[:]) + "  " + asset + "\n")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/shibisty/gtr/releases/latest" {
			fmt.Fprint(w, `{"tag_name": "v0.3.0", "name": "v0.3.0"}`)
			return
		}
		data, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if badSum.Load() && strings.HasSuffix(r.URL.Path, ".tar.gz") {
			data = append([]byte("x"), data...)
		}
		w.Write(data)
	}))
	defer srv.Close()

	userHome := t.TempDir()
	os.WriteFile(filepath.Join(userHome, ".bashrc"), []byte("# mine\n"), 0o644)
	run := func(extra ...string) (string, error) {
		cmd := exec.Command("sh", script)
		cmd.Env = append([]string{"HOME=" + userHome, "PATH=" + os.Getenv("PATH"),
			"GTR_GITHUB_API=" + srv.URL, "GTR_DOWNLOAD_BASE=" + srv.URL + "/dl"}, extra...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	out, err := run()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	gtrHome := filepath.Join(userHome, ".gtr")
	bin, _ := os.ReadFile(filepath.Join(gtrHome, "bin", "gtr"))
	if !strings.Contains(string(bin), "# 0.3.0") {
		t.Fatalf("installed: %q\n%s", bin, out)
	}
	if calls, _ := os.ReadFile(filepath.Join(gtrHome, "calls.log")); string(calls) != "self shims\n" {
		t.Fatalf("calls: %q", calls)
	}
	for _, rc := range []string{".profile", ".bashrc"} {
		data, _ := os.ReadFile(filepath.Join(userHome, rc))
		if strings.Count(string(data), filepath.Join(gtrHome, "bin")) != 1 {
			t.Fatalf("%s:\n%s", rc, data)
		}
	}
	_, err = os.Stat(filepath.Join(userHome, ".zshrc"))
	if runtime.GOOS == "linux" && err == nil {
		t.Fatal(".zshrc created although zsh is not used")
	}
	if runtime.GOOS == "darwin" && err != nil {
		t.Fatal("no .zshrc on macOS, where zsh is the default shell")
	}

	// A pinned version, run again: profiles are not edited twice.
	if out, err := run("GTR_VERSION=v0.2.0"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if bin, _ := os.ReadFile(filepath.Join(gtrHome, "bin", "gtr")); !strings.Contains(string(bin), "# 0.2.0") {
		t.Fatalf("pinned: %q", bin)
	}
	if data, _ := os.ReadFile(filepath.Join(userHome, ".profile")); strings.Count(string(data), "# gtr") != 1 {
		t.Fatalf("profile edited twice:\n%s", data)
	}

	// A corrupted download is refused and the installed gtr stays.
	badSum.Store(true)
	out, err = run()
	if err == nil || !strings.Contains(out, "SHA-256 mismatch") {
		t.Fatalf("bad checksum: %v\n%s", err, out)
	}
	if bin, _ := os.ReadFile(filepath.Join(gtrHome, "bin", "gtr")); !strings.Contains(string(bin), "# 0.2.0") {
		t.Fatal("a failed install replaced gtr")
	}
	badSum.Store(false)

	// Custom home, no shims, no profile edits; an unknown version fails.
	other := filepath.Join(t.TempDir(), "gtrhome")
	if out, err := run("GTR_HOME="+other, "GTR_NO_SHIMS=1", "GTR_NO_MODIFY_PATH=1"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(other, "calls.log")); err == nil {
		t.Fatal("shims created with GTR_NO_SHIMS")
	}
	if out, err := run("GTR_VERSION=9.9.9"); err == nil || !strings.Contains(out, "cannot download") {
		t.Fatalf("unknown version: %v\n%s", err, out)
	}
}
