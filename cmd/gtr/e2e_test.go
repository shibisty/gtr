package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gtr/internal/goinstall"
)

// TestE2E runs the built gtr against a fake go.dev: install, list, use,
// env, prune and uninstall, with nothing written outside GTR_HOME.
func TestE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("end-to-end test")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not in PATH")
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "gtr")
	exe := ""
	if runtime.GOOS == "windows" {
		bin += ".exe"
		exe = ".exe"
	}
	if out, err := exec.Command(goBin, "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	// go.dev/dl with three releases for this platform.
	archives := map[string][]byte{}
	var rels []goinstall.Release
	for _, v := range []string{"1.26.3", "1.26.9", "1.27.2"} {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		f, _ := zw.Create("go/bin/go" + exe)
		f.Write([]byte("fake go " + v))
		zw.Close()
		name := "go" + v + "." + runtime.GOOS + "-" + runtime.GOARCH + ".zip"
		archives["/"+name] = buf.Bytes()
		sum := sha256.Sum256(buf.Bytes())
		rels = append(rels, goinstall.Release{Version: "go" + v, Stable: true, Files: []goinstall.File{
			{Filename: name, OS: runtime.GOOS, Arch: runtime.GOARCH, Kind: "archive", SHA256: hex.EncodeToString(sum[:])}}})
	}
	godev := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("mode") == "json" {
			json.NewEncoder(w).Encode(rels)
			return
		}
		if data, ok := archives[r.URL.Path]; ok {
			w.Write(data)
			return
		}
		http.NotFound(w, r)
	}))
	defer godev.Close()

	home, gtrHome := filepath.Join(tmp, "home"), filepath.Join(tmp, "gtrhome")
	os.MkdirAll(home, 0o755)
	env := []string{"PATH=" + filepath.Join(tmp, "nopath"), "SYSTEMROOT=" + os.Getenv("SYSTEMROOT"),
		"HOME=" + home, "USERPROFILE=" + home, "GTR_HOME=" + gtrHome, "GTR_GO_MIRROR=" + godev.URL + "/",
		"GTR_GITHUB_API=http://127.0.0.1:1", "GITHUB_TOKEN=x"}
	gtr := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Dir, cmd.Env = dir, env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("gtr %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}

	gtr(tmp, "self", "install", "go", "1.26")
	if out := gtr(tmp, "self", "list", "go"); !strings.Contains(out, "1.26.9") || strings.Contains(out, "1.26.3") {
		t.Fatalf("list:\n%s", out)
	}
	gtr(tmp, "self", "use", "go", "1.27") // installs 1.27.2
	if out := gtr(tmp, "self", "list", "--remote", "go"); !strings.Contains(out, "* 1.27.2") || !strings.Contains(out, "i 1.26.9") {
		t.Fatalf("list --remote:\n%s", out)
	}

	proj := filepath.Join(tmp, "proj")
	os.MkdirAll(filepath.Join(proj, "sub"), 0o755)
	os.WriteFile(filepath.Join(proj, "gtr.json"), []byte(`{"name":"proj","engines":{"go":"~1.26"}}`), 0o644)
	if out := gtr(filepath.Join(proj, "sub"), "env", "GTR_GO_VERSION", "GOROOT"); out != "1.26.9\n"+filepath.Join(gtrHome, "go", "1.26.9")+"\n" {
		t.Fatalf("env in a project: %q", out)
	}
	if out := gtr(proj, "--go", "1.27", "env", "GTR_GO_VERSION"); out != "1.27.2\n" {
		t.Fatalf("--go: %q", out)
	}
	if out := gtr(tmp, "env", "GTR_GO_VERSION"); out != "1.27.2\n" {
		t.Fatalf("default outside a project: %q", out)
	}

	gtr(tmp, "self", "install", "go", "1.26.3")
	if out := gtr(tmp, "self", "prune"); !strings.Contains(out, "removed go/1.26.3") || !strings.Contains(out, "cache/downloads") {
		t.Fatalf("prune:\n%s", out)
	}
	gtr(tmp, "self", "uninstall", "go", "1.26.9")
	if _, err := os.Stat(filepath.Join(gtrHome, "go", "1.26.9")); err == nil {
		t.Fatal("not uninstalled")
	}
	if out := gtr(tmp, "doctor", "--offline"); !strings.Contains(out, "0 problem(s)") {
		t.Fatalf("doctor:\n%s", out)
	}

	filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			t.Errorf("written to HOME: %s", p)
		}
		return nil
	})
}
