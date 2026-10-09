package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gtr/internal/config"
	"gtr/internal/goinstall"
	"gtr/internal/home"
	"gtr/internal/release"
)

var ctx = context.Background()

// --- fake GitHub releases ---------------------------------------------------

type fakeRelease struct {
	tag        string
	prerelease bool
	noSums     bool
	badSum     bool
	noAsset    bool
	binary     string // content of the binary in the archive
}

type github struct {
	*httptest.Server
	releases map[string][]fakeRelease // repo → releases
}

func tgz(t *testing.T, name, content string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "pkg/" + name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg})
	tw.Write([]byte(content))
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func newGitHub(t *testing.T) *github {
	g := &github{releases: map[string][]fakeRelease{}}
	files := map[string][]byte{}
	g.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for repo, rels := range g.releases {
			if r.URL.Path != "/repos/"+repo+"/releases" {
				continue
			}
			app := strings.Split(repo, "/")[1]
			var out []release.Release
			for _, fr := range rels {
				v := strings.TrimPrefix(fr.tag, "v")
				name := fmt.Sprintf("%s_%s_linux_amd64.tar.gz", app, v)
				data := tgz(t, app, fr.binary)
				sum := sha256.Sum256(data)
				hexsum := hex.EncodeToString(sum[:])
				if fr.badSum {
					hexsum = strings.Repeat("0", 64)
				}
				base := g.URL + "/dl/" + app + "/" + v + "/"
				files[base+name] = data
				files[base+"SHA256SUMS"] = []byte(hexsum + "  " + name + "\n")
				rel := release.Release{Tag: fr.tag, Prerelease: fr.prerelease}
				if !fr.noAsset {
					rel.Assets = append(rel.Assets, release.Asset{Name: name, URL: base + name})
				}
				if !fr.noSums {
					rel.Assets = append(rel.Assets, release.Asset{Name: "SHA256SUMS", URL: base + "SHA256SUMS"})
				}
				out = append(out, rel)
			}
			out = append(out, release.Release{Tag: "v9.9.9", Draft: true})
			json.NewEncoder(w).Encode(out)
			return
		}
		if data, ok := files[g.URL+r.URL.Path]; ok {
			w.Write(data)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(g.Close)
	return g
}

// --- fake go.dev --------------------------------------------------------------

func newGoDev(t *testing.T, versions ...string) *httptest.Server {
	archives := map[string][]byte{}
	var rels []goinstall.Release
	for _, v := range versions {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		f, _ := zw.Create("go/bin/go")
		f.Write([]byte("go " + v))
		zw.Close()
		name := "go" + v + ".linux-amd64.zip"
		archives["/"+name] = buf.Bytes()
		sum := sha256.Sum256(buf.Bytes())
		rels = append(rels, goinstall.Release{Version: "go" + v, Stable: !strings.Contains(v, "rc"), Files: []goinstall.File{
			{Filename: name, OS: "linux", Arch: "amd64", Kind: "archive", SHA256: hex.EncodeToString(sum[:])}}})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	t.Cleanup(srv.Close)
	return srv
}

// --- app under test -----------------------------------------------------------

type execCall struct {
	bin  string
	args []string
	env  map[string]string
}

type harness struct {
	app   *App
	out   *bytes.Buffer // stdout and stderr together
	sout  *bytes.Buffer
	serr  *bytes.Buffer
	root  string
	cwd   string
	env   map[string]string
	calls []execCall
	code  int
	gh    *github
	godev *httptest.Server
	exe   string
}

func newHarness(t *testing.T) *harness {
	h := &harness{out: &bytes.Buffer{}, sout: &bytes.Buffer{}, serr: &bytes.Buffer{}, root: t.TempDir(), cwd: t.TempDir(), env: map[string]string{"PATH": "/usr/bin"}}
	h.gh = newGitHub(t)
	h.godev = newGoDev(t, "1.26.9", "1.27.2", "1.28rc1")
	h.exe = filepath.Join(t.TempDir(), "gtr")
	os.WriteFile(h.exe, []byte("gtr-old"), 0o755)
	h.app = &App{
		Version:    "0.1.0",
		Out:        io.MultiWriter(h.out, h.sout),
		Err:        io.MultiWriter(h.out, h.serr),
		GOOS:       "linux",
		Home:       func() (home.Layout, error) { return home.Layout{Root: h.root}, nil },
		Getenv:     func(k string) string { return h.env[k] },
		Getwd:      func() (string, error) { return h.cwd, nil },
		Environ:    func() []string { return []string{"PATH=/usr/bin", "HOME=/home/x"} },
		Executable: func() (string, error) { return h.exe, nil },
		NewGo: func(l home.Layout) *goinstall.Installer {
			return &goinstall.Installer{Home: l, BaseURL: h.godev.URL, GOOS: "linux", GOARCH: "amd64"}
		},
		NewSource: func(l home.Layout, app, repo string) *release.Source {
			return &release.Source{App: app, Repo: repo, API: h.gh.URL, GOOS: "linux", GOARCH: "amd64", Home: l}
		},
		Exec: func(bin string, args, env []string) (int, error) {
			c := execCall{bin: bin, args: args, env: map[string]string{}}
			for _, kv := range env {
				k, v, _ := strings.Cut(kv, "=")
				c.env[k] = v
			}
			h.calls = append(h.calls, c)
			return h.code, nil
		},
	}
	return h
}

func (h *harness) run(args ...string) error { return h.app.Run(ctx, args) }
func (h *harness) stdout() string           { return h.sout.String() }
func (h *harness) stderr() string           { return h.serr.String() }

func (h *harness) installGo(versions ...string) {
	for _, v := range versions {
		os.MkdirAll(filepath.Join(h.root, "go", v, "bin"), 0o755)
		os.WriteFile(filepath.Join(h.root, "go", v, "bin", "go"), []byte("go"), 0o755)
	}
}

func (h *harness) installManager(versions ...string) {
	for _, v := range versions {
		os.MkdirAll(filepath.Join(h.root, "gtr", v), 0o755)
		os.WriteFile(filepath.Join(h.root, "gtr", v, "gtr-manager"), []byte("m"), 0o755)
	}
}

func (h *harness) project(json string) {
	os.WriteFile(filepath.Join(h.cwd, "gtr.json"), []byte(json), 0o644)
}

// --- tests ----------------------------------------------------------------------

func TestHelpAndVersion(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{nil, {"help"}, {"-h"}} {
		h.out.Reset()
		if err := h.run(args...); err != nil || !strings.Contains(h.out.String(), "self install go") {
			t.Fatalf("%v: %v", args, err)
		}
	}
	h.out.Reset()
	if err := h.run("--version"); err != nil || h.out.String() != "gtr 0.1.0\n" {
		t.Fatalf("version: %q", h.out)
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"nope"}, {"self"}, {"self", "nope"}, {"self", "install"}, {"self", "install", "python"},
		{"self", "install", "go", "1", "2"}, {"self", "use", "go"}, {"self", "uninstall", "gtr"},
		{"self", "list", "x"}, {"self", "link"}, {"self", "link", "go", "x"}, {"self", "unlink"},
		{"self", "shims", "x"}, {"self", "update", "1", "2"},
	} {
		if err := newHarness(t).run(args...); !errors.Is(err, ErrUsage) {
			t.Errorf("%v: want usage error, got %v", args, err)
		}
	}
}

func TestForwardUsesProjectVersions(t *testing.T) {
	h := newHarness(t)
	h.installGo("1.25.0", "1.26.9", "1.26.3")
	h.installManager("0.1.0", "0.2.1", "0.3.0")
	h.project(`{"name":"app","engines":{"go":">=1.26 <1.27","gtr":"^0.2"}}`)
	sub := filepath.Join(h.cwd, "cmd", "app")
	os.MkdirAll(sub, 0o755)
	h.cwd = sub // gtr.json is found in a parent directory

	if err := h.run("run", "test", "--", "-v"); err != nil {
		t.Fatal(err)
	}
	c := h.calls[0]
	if c.bin != filepath.Join(h.root, "gtr", "0.2.1", "gtr-manager") || strings.Join(c.args, " ") != "run test -- -v" {
		t.Fatalf("exec %s %v", c.bin, c.args)
	}
	goroot := filepath.Join(h.root, "go", "1.26.9")
	if c.env["GOROOT"] != goroot || c.env["GTR_GO_VERSION"] != "1.26.9" || c.env["GOTOOLCHAIN"] != "local" ||
		!strings.HasPrefix(c.env["PATH"], filepath.Join(goroot, "bin")+string(os.PathListSeparator)) || c.env["HOME"] != "/home/x" {
		t.Fatalf("env %v", c.env)
	}

	h.code = 3
	if err := h.run("install", "github:a/b"); err != ExitCode(3) {
		t.Fatalf("exit code: %v", err)
	}
}

func TestForwardInstallsMissingVersions(t *testing.T) {
	h := newHarness(t)
	h.gh.releases["shibisty/gtr-manager"] = []fakeRelease{
		{tag: "v0.1.0", binary: "m010"}, {tag: "v0.2.0", binary: "m020"}, {tag: "v0.2.3", binary: "m023"},
		{tag: "v0.2.4-rc.1", prerelease: true, binary: "rc"}, {tag: "v0.3.0", binary: "m030"},
	}
	h.project(`{"engines":{"go":"^1.27","gtr":"^0.2"}}`)
	if err := h.run("init"); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(h.root, "gtr", "0.2.3", "gtr-manager")
	if got, _ := os.ReadFile(bin); string(got) != "m023" || h.calls[0].bin != bin {
		t.Fatalf("manager: %q, exec %s", got, h.calls[0].bin)
	}
	if got, _ := os.ReadFile(filepath.Join(h.root, "go", "1.27.2", "bin", "go")); string(got) != "go 1.27.2" {
		t.Fatalf("go not installed: %q", got)
	}
	if !strings.Contains(h.stderr(), "required by") || strings.Contains(h.stdout(), "installed") {
		t.Fatalf("install messages must go to stderr only: out=%q err=%q", h.stdout(), h.stderr())
	}
	// Second run: everything is installed, nothing is downloaded or printed.
	h.out.Reset()
	if err := h.run("init"); err != nil || strings.Contains(h.out.String(), "installing") {
		t.Fatalf("second run: %v %s", err, h.out)
	}
}

func TestLegacyEngineField(t *testing.T) {
	h := newHarness(t)
	h.installGo("1.26.9")
	h.installManager("0.2.0")
	h.project(`{"name":"old","engine":">=1.26"}`)
	if err := h.run("start"); err != nil || h.calls[0].env["GTR_GO_VERSION"] != "1.26.9" {
		t.Fatalf("%v %v", err, h.calls)
	}
}

func TestNoProjectNoGo(t *testing.T) {
	h := newHarness(t)
	h.installManager("0.2.0")
	if err := h.run("new", "app"); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.calls[0].env["GOROOT"]; ok {
		t.Fatal("without a selected Go the environment must stay untouched")
	}
}

func TestManagerReleaseErrors(t *testing.T) {
	cases := map[string]struct {
		rel  fakeRelease
		want string
	}{
		"bad checksum": {fakeRelease{tag: "v0.2.0", badSum: true}, "SHA-256 mismatch"},
		"no checksums": {fakeRelease{tag: "v0.2.0", noSums: true}, "unverified"},
		"no build":     {fakeRelease{tag: "v0.2.0", noAsset: true}, "no build for linux/amd64"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.gh.releases["shibisty/gtr-manager"] = []fakeRelease{tc.rel}
			err := h.run("install", "x")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
			if entries, _ := os.ReadDir(filepath.Join(h.root, "gtr")); len(entries) != 0 {
				t.Fatal("failed install must leave nothing")
			}
		})
	}
	h := newHarness(t)
	if err := h.run("install", "x"); err == nil || !strings.Contains(err.Error(), "self link gtr") {
		t.Fatalf("no releases: %v", err)
	}
}

func TestLink(t *testing.T) {
	h := newHarness(t)
	h.installManager("0.2.0")
	local := filepath.Join(t.TempDir(), "gtr-manager")
	os.WriteFile(local, []byte("dev"), 0o755)
	if err := h.run("self", "link", "gtr", local); err != nil {
		t.Fatal(err)
	}
	h.run("run")
	if h.calls[0].bin != local {
		t.Fatalf("linked build not used: %s", h.calls[0].bin)
	}
	h.out.Reset()
	h.run("self", "list", "gtr")
	if !strings.Contains(h.out.String(), "linked: "+local) {
		t.Fatalf("list: %s", h.out)
	}
	if err := h.run("self", "unlink", "gtr"); err != nil {
		t.Fatal(err)
	}
	h.run("run")
	if h.calls[1].bin == local {
		t.Fatal("unlink did not take effect")
	}
	if err := h.run("self", "unlink", "gtr"); err == nil {
		t.Fatal("second unlink must fail")
	}
	if err := h.run("self", "link", "gtr", filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("link to a missing file must fail")
	}
}

func TestUseListUninstall(t *testing.T) {
	h := newHarness(t)
	h.installGo("1.26.3")
	if err := h.run("self", "use", "go", "1.26"); err != nil {
		t.Fatal(err)
	}
	c, _ := config.Load(home.Layout{Root: h.root})
	if c.Go != "1.26.3" {
		t.Fatalf("use picked %q", c.Go)
	}
	// Not installed: use installs it.
	if err := h.run("self", "use", "go", "1.27"); err != nil {
		t.Fatal(err)
	}
	h.out.Reset()
	h.run("self", "list", "go")
	if !strings.Contains(h.out.String(), "  1.26.3\n* 1.27.2\n") {
		t.Fatalf("list: %q", h.out)
	}
	// The default applies outside projects.
	h.installManager("0.2.0")
	h.run("run")
	if h.calls[0].env["GTR_GO_VERSION"] != "1.27.2" {
		t.Fatalf("default not used: %v", h.calls[0].env)
	}
	h.out.Reset()
	if err := h.run("self", "uninstall", "go", "1.27.2"); err != nil || !strings.Contains(h.out.String(), "no default is set") {
		t.Fatalf("uninstall: %v %s", err, h.out)
	}
	if err := h.run("self", "uninstall", "go", "1.27.2"); err == nil {
		t.Fatal("uninstalling twice must fail")
	}
	if err := h.run("self", "uninstall", "go", "go1.26.3"); err != nil {
		t.Fatalf("go-prefixed version: %v", err)
	}

	h.gh.releases["shibisty/gtr-manager"] = []fakeRelease{{tag: "v0.4.0", binary: "m"}, {tag: "v0.5.0", prerelease: true, binary: "p"}}
	if err := h.run("self", "use", "gtr", "0.4"); err != nil {
		t.Fatal(err)
	}
	if c, _ := config.Load(home.Layout{Root: h.root}); c.Gtr != "0.4.0" {
		t.Fatalf("gtr default %q", c.Gtr)
	}
	if err := h.run("self", "uninstall", "gtr", "v0.4.0"); err != nil {
		t.Fatal(err)
	}
	if c, _ := config.Load(home.Layout{Root: h.root}); c.Gtr != "" {
		t.Fatalf("default not cleared: %q", c.Gtr)
	}
	// A release flagged as pre-release is installed only when named exactly.
	if err := h.run("self", "install", "gtr", "^0.5"); err == nil {
		t.Fatal("pre-release flagged release must not satisfy a range")
	}
	if err := h.run("self", "install", "gtr", "0.5.0"); err != nil {
		t.Fatalf("exact pre-release flagged release: %v", err)
	}
}

func TestInstallGoFromProject(t *testing.T) {
	h := newHarness(t)
	if err := h.run("self", "install", "go"); !errors.Is(err, ErrUsage) {
		t.Fatalf("no project: %v", err)
	}
	h.project(`{"engines":{"go":"~1.26"}}`)
	h.out.Reset()
	if err := h.run("self", "install", "go"); err != nil || !strings.Contains(h.out.String(), "Go 1.26.9") {
		t.Fatalf("%v %s", err, h.out)
	}
	if err := h.run("self", "install", "go", "1.28rc1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(h.root, "go", "1.28rc1", "bin", "go")); err != nil {
		t.Fatal("explicit pre-release must install")
	}
	h.project(`{"engines":{"go":"banana"}}`)
	if err := h.run("self", "install", "go"); err == nil || !strings.Contains(err.Error(), "engines.go") {
		t.Fatalf("bad range: %v", err)
	}
}

func TestCurrent(t *testing.T) {
	h := newHarness(t)
	h.out.Reset()
	h.run("self", "current")
	if !strings.Contains(h.out.String(), "Go:          none selected") || !strings.Contains(h.out.String(), "gtr-manager: error") {
		t.Fatalf("empty: %s", h.out)
	}
	h.installGo("1.26.9")
	h.installManager("0.2.0")
	h.project(`{"engines":{"go":"1.26"}}`)
	h.env["GTR_MANAGER_VERSION"] = "0.2.0"
	h.out.Reset()
	h.run("self", "current")
	if !strings.Contains(h.out.String(), `1.26.9  from `+filepath.Join(h.cwd, "gtr.json")+` (engines.go "1.26")`) ||
		!strings.Contains(h.out.String(), "0.2.0  from GTR_MANAGER_VERSION=0.2.0") {
		t.Fatalf("current: %s", h.out)
	}
}

func TestShim(t *testing.T) {
	h := newHarness(t)
	if err := h.app.Shim(ctx, "go", []string{"version"}); err == nil || !strings.Contains(err.Error(), "no Go version selected") {
		t.Fatalf("no go: %v", err)
	}
	h.installGo("1.26.9", "1.27.2")
	h.project(`{"engines":{"go":"1.26"}}`)
	if err := h.app.Shim(ctx, "gofmt", []string{"-l", "."}); err != nil {
		t.Fatal(err)
	}
	if c := h.calls[0]; c.bin != filepath.Join(h.root, "go", "1.26.9", "bin", "gofmt") || strings.Join(c.args, " ") != "-l ." {
		t.Fatalf("shim exec %s %v", c.bin, c.args)
	}
	h.env["GTR_GO_VERSION"] = "1.27.2"
	h.code = 2
	if err := h.app.Shim(ctx, "go", nil); err != ExitCode(2) || h.calls[1].bin != filepath.Join(h.root, "go", "1.27.2", "bin", "go") {
		t.Fatalf("override: %v %s", err, h.calls[1].bin)
	}
}

func TestShimGtrProjectEnv(t *testing.T) {
	h := newHarness(t)
	h.installGo("1.26.9")
	h.project(`{"engines":{"go":"1.26"}}`)
	var viaExecGo int
	h.app.ExecGo = func(bin string, args, env []string) (int, error) {
		viaExecGo++
		return h.app.Exec(bin, args, env)
	}
	os.WriteFile(filepath.Join(h.cwd, "go.mod"), []byte("module app\n"), 0o644)
	if err := h.app.Shim(ctx, "go", []string{"build"}); err != nil || h.calls[0].env["GOPROXY"] != "" || viaExecGo != 0 {
		t.Fatalf("a foreign go.mod: %v %v", err, h.calls[0].env)
	}
	os.WriteFile(filepath.Join(h.cwd, "go.mod"), []byte("// generated by gtr from gtr.json — edit gtr.json or run `gtr sync`\nmodule app\n"), 0o644)
	sub := filepath.Join(h.cwd, "cmd", "x")
	os.MkdirAll(sub, 0o755)
	h.cwd = sub
	if err := h.app.Shim(ctx, "go", []string{"build"}); err != nil {
		t.Fatal(err)
	}
	if c := h.calls[1]; c.env["GOPROXY"] != "off" || c.env["GOWORK"] != "off" || c.env["GOFLAGS"] != "-mod=mod" || viaExecGo != 1 {
		t.Fatalf("generated go.mod: %v (ExecGo %d)", c.env, viaExecGo)
	}
	if err := h.app.Shim(ctx, "gofmt", []string{"-l", "."}); err != nil || h.calls[2].env["GOPROXY"] != "" {
		t.Fatalf("gofmt: %v", err)
	}
	// A nested module of its own is not part of the gtr project.
	tools := filepath.Join(h.cwd, "tools")
	os.MkdirAll(tools, 0o755)
	os.WriteFile(filepath.Join(tools, "go.mod"), []byte("module tools\n"), 0o644)
	h.cwd = tools
	if err := h.app.Shim(ctx, "go", []string{"build"}); err != nil || h.calls[3].env["GOPROXY"] != "" {
		t.Fatalf("nested module: %v %v", err, h.calls[3].env)
	}
	if err := h.app.Shim(ctx, "go", []string{"-C", "..", "build"}); err != nil || h.calls[4].env["GOPROXY"] != "off" {
		t.Fatalf("-C: %v %v", err, h.calls[4].env)
	}
	// A workspace: go.work generated by gtr above the member.
	ws := filepath.Join(h.cwd, "ws")
	member := filepath.Join(ws, "core")
	os.MkdirAll(member, 0o755)
	os.WriteFile(filepath.Join(ws, "go.work"), []byte("// generated by gtr from gtr.json — edit gtr.json or run `gtr sync`\ngo 1.26\n\nuse (\n\t.\n\t./core\n)\n"), 0o644)
	os.WriteFile(filepath.Join(member, "go.mod"), []byte("// generated by gtr from gtr.json — edit gtr.json or run `gtr sync`\nmodule orm\n"), 0o644)
	h.cwd = member
	if err := h.app.Shim(ctx, "go", []string{"test"}); err != nil {
		t.Fatal(err)
	}
	if c := h.calls[5]; c.env["GOWORK"] != filepath.Join(ws, "go.work") || c.env["GOPROXY"] != "off" || strings.Contains(c.env["GOFLAGS"], "-mod=mod") {
		t.Fatalf("workspace: %v", c.env)
	}
}

// In a workspace member, Go must be new enough for go.work.
func TestShimWorkspaceGo(t *testing.T) {
	h := newHarness(t)
	h.installGo("1.26.9", "1.27.2")
	ws := h.cwd
	member := filepath.Join(ws, "core")
	os.MkdirAll(member, 0o755)
	os.WriteFile(filepath.Join(ws, "go.work"), []byte("// generated by gtr from gtr.json — edit gtr.json or run `gtr sync`\ngo 1.27\n\nuse ./core\n"), 0o644)
	os.WriteFile(filepath.Join(member, "go.mod"), []byte("// generated by gtr from gtr.json — edit gtr.json or run `gtr sync`\nmodule orm\n"), 0o644)
	os.WriteFile(filepath.Join(member, "gtr.json"), []byte(`{"name":"orm","engines":{"go":">=1.26"}}`), 0o644)
	h.cwd = member
	if err := h.app.Shim(ctx, "go", []string{"version"}); err != nil {
		t.Fatal(err)
	}
	if c := h.calls[0]; c.bin != filepath.Join(h.root, "go", "1.27.2", "bin", "go") {
		t.Fatalf("selected %s", c.bin)
	}
}

func TestShimFallsBackToSystemGo(t *testing.T) {
	h := newHarness(t)
	// ~/.gtr/bin (the shims) comes first in PATH and must be skipped.
	h.run("self", "shims")
	sys := t.TempDir()
	os.WriteFile(filepath.Join(sys, "go"), []byte("system go"), 0o755)
	h.env["PATH"] = filepath.Join(h.root, "bin") + string(os.PathListSeparator) + sys
	if err := h.app.Shim(ctx, "go", []string{"env"}); err != nil {
		t.Fatal(err)
	}
	if c := h.calls[0]; c.bin != filepath.Join(sys, "go") || c.env["GOROOT"] != "" {
		t.Fatalf("fallback exec %s %v", c.bin, c.env)
	}
	// A copy of the shim elsewhere in PATH is skipped too (no recursion).
	h.env["PATH"] = filepath.Join(h.root, "bin")
	if err := h.app.Shim(ctx, "go", nil); err == nil || !strings.Contains(err.Error(), "no other go in PATH") {
		t.Fatalf("no system go: %v", err)
	}
}

func TestShims(t *testing.T) {
	h := newHarness(t)
	if err := h.run("self", "shims"); err != nil {
		t.Fatal(err)
	}
	for _, n := range ShimNames {
		got, err := os.ReadFile(filepath.Join(h.root, "bin", n))
		if err != nil || string(got) != "gtr-old" {
			t.Fatalf("%s: %q %v", n, got, err)
		}
	}
	if !strings.Contains(h.stdout(), "export PATH=") {
		t.Fatalf("PATH hint: %s", h.out)
	}
	h.env["PATH"] = filepath.Join(h.root, "bin")
	h.out.Reset()
	if err := h.run("self", "shims"); err != nil || strings.Contains(h.out.String(), "export PATH") {
		t.Fatalf("rerun: %v %s", err, h.out)
	}
	w := newHarness(t)
	w.app.GOOS = "windows"
	w.env["PATH"] = strings.ToUpper(filepath.Join(w.root, "bin"))
	w.run("self", "shims")
	if strings.Contains(w.out.String(), "SetEnvironmentVariable") {
		t.Fatal("PATH comparison must be case-insensitive on Windows")
	}
	w.env["PATH"] = ""
	w.out.Reset()
	w.run("self", "shims")
	if !strings.Contains(w.out.String(), "SetEnvironmentVariable('Path'") || strings.Contains(w.out.String(), "setx") {
		t.Fatalf("windows hint: %s", w.out)
	}
}

func TestSelfUpdate(t *testing.T) {
	h := newHarness(t)
	h.gh.releases["shibisty/gtr"] = []fakeRelease{{tag: "v0.1.0", binary: "gtr-010"}, {tag: "v0.2.0", binary: "gtr-020"}}
	h.run("self", "shims")
	// After the rename os.Executable may report the deleted gtr.old.
	calls := 0
	h.app.Executable = func() (string, error) {
		calls++
		if calls > 1 {
			return h.exe + ".old", nil
		}
		return h.exe, nil
	}
	if err := h.run("self", "update"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(h.exe); string(got) != "gtr-020" {
		t.Fatalf("exe: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(h.root, "bin", "go")); string(got) != "gtr-020" {
		t.Fatalf("shims must be refreshed: %q", got)
	}
	if _, err := os.Stat(h.exe + ".old"); !os.IsNotExist(err) {
		t.Fatal(".old must be removed")
	}
	h.app.Executable = func() (string, error) { return h.exe, nil } // a new process
	h.app.Version = "0.2.0"
	h.out.Reset()
	if err := h.run("self", "update"); err != nil || !strings.Contains(h.out.String(), "already installed") {
		t.Fatalf("no-op update: %v %s", err, h.out)
	}
	if err := h.run("self", "update", "0.1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(h.exe); string(got) != "gtr-010" {
		t.Fatalf("downgrade: %q", got)
	}
	if err := h.run("self", "update", "9"); err == nil {
		t.Fatal("missing version must fail")
	}
}

func TestHomeError(t *testing.T) {
	h := newHarness(t)
	h.app.Home = func() (home.Layout, error) { return home.Layout{}, errors.New("no home") }
	for _, args := range [][]string{{"self", "list"}, {"run"}} {
		if err := h.run(args...); err == nil || err.Error() != "no home" {
			t.Errorf("%v: %v", args, err)
		}
	}
	if err := h.app.Shim(ctx, "go", nil); err == nil {
		t.Error("shim without home")
	}
}
