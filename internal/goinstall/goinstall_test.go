package goinstall

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"gtr/internal/home"
)

var ctx = context.Background()

func rel(v string, stable bool) Release { return Release{Version: v, Stable: stable} }

func TestSelect(t *testing.T) {
	list := []Release{rel("go1.27rc1", false), rel("go1.26.5", true), rel("go1.26.10", true), rel("go1.26.4", true),
		rel("go1.20.14", true), rel("go1.20", true), rel("go1.9.7", true), rel("go1.10", true)}
	for req, want := range map[string]string{
		"1.26.5": "go1.26.5", "go1.26.5": "go1.26.5", " 1.26 ": "go1.26.10", "1": "go1.26.10",
		"1.27rc1": "go1.27rc1", "1.20": "go1.20.14", "1.9": "go1.9.7", "1.10": "go1.10",
	} {
		r, err := Select(list, req)
		if err != nil || r.Version != want {
			t.Errorf("Select(%q) = %v, %v; want %s", req, r, err, want)
		}
	}
	for _, req := range []string{"1.27", "1.26.99", "2", "1.28rc1"} {
		if _, err := Select(list, req); !errors.Is(err, ErrNotFound) {
			t.Errorf("Select(%q): want ErrNotFound, got %v", req, err)
		}
	}
	for _, req := range []string{"", "1.26;rm", "../1"} {
		if _, err := Select(list, req); err == nil || errors.Is(err, ErrNotFound) {
			t.Errorf("Select(%q): want validation error, got %v", req, err)
		}
	}
}

func TestFindArchive(t *testing.T) {
	files := []File{{Filename: "src", Kind: "source"}, {Filename: "msi", OS: "windows", Arch: "amd64", Kind: "installer"},
		{Filename: "zip", OS: "windows", Arch: "amd64", Kind: "archive"}}
	if f := FindArchive(files, "windows", "amd64"); f == nil || f.Filename != "zip" {
		t.Fatalf("got %v", f)
	}
	if FindArchive(files, "linux", "amd64") != nil {
		t.Fatal("no linux archive expected")
	}
}

// goZip builds an archive shaped like the go.dev ones: everything under go/.
func goZip(t *testing.T, version string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{"go/bin/go": "bin", "go/bin/go.exe": "bin", "go/VERSION": "go" + version} {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	return buf.Bytes()
}

type server struct {
	*httptest.Server
	archive   []byte
	sum       string
	listCode  int
	fileCode  int
	downloads atomic.Int32
}

func newServer(t *testing.T) *server {
	s := &server{archive: goZip(t, "1.26.5")}
	h := sha256.Sum256(s.archive)
	s.sum = hex.EncodeToString(h[:])
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/dl/" && r.URL.Query().Get("mode") == "json":
			if s.listCode != 0 {
				w.WriteHeader(s.listCode)
				return
			}
			json.NewEncoder(w).Encode([]Release{
				{Version: "go1.27rc1", Files: []File{{Filename: "go1.27rc1.test-arch.zip", OS: "test", Arch: "arch", Kind: "archive", SHA256: "x"}}},
				{Version: "go1.26.5", Stable: true, Files: []File{
					{Filename: "go1.26.5.src.tar.gz", Kind: "source"},
					{Filename: "go1.26.5.test-arch.zip", OS: "test", Arch: "arch", Kind: "archive", SHA256: s.sum},
					{Filename: "go1.26.5.nosum.zip", OS: "test", Arch: "nosum", Kind: "archive"},
				}},
			})
		case strings.HasPrefix(r.URL.Path, "/dl/go1.26.5."):
			s.downloads.Add(1)
			if s.fileCode != 0 {
				w.WriteHeader(s.fileCode)
				return
			}
			w.Write(s.archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *server) installer(t *testing.T, root string) *Installer {
	return &Installer{Home: home.Layout{Root: root}, BaseURL: s.URL + "/dl", GOOS: "test", GOARCH: "arch"}
}

func TestInstall(t *testing.T) {
	s := newServer(t)
	root := t.TempDir()
	var log bytes.Buffer
	in := s.installer(t, root)
	in.Log = &log

	v, err := in.Install(ctx, "1.26")
	if err != nil || v != "1.26.5" {
		t.Fatalf("Install = %q, %v", v, err)
	}
	got, err := os.ReadFile(filepath.Join(root, "go", "1.26.5", "VERSION"))
	if err != nil || string(got) != "go1.26.5" {
		t.Fatalf("installed tree: %q %v", got, err)
	}
	if entries, _ := os.ReadDir(filepath.Join(root, "tmp")); len(entries) != 0 {
		t.Fatalf("tmp must be cleaned: %v", entries)
	}
	if !strings.Contains(log.String(), "installed") {
		t.Fatalf("log: %s", log.String())
	}

	// Reinstalling does not download again.
	if _, err := in.Install(ctx, "1.26.5"); err != nil || s.downloads.Load() != 1 {
		t.Fatalf("reinstall downloaded again: %v, %d", err, s.downloads.Load())
	}
	// After the installation is removed, the archive comes from the cache.
	os.RemoveAll(filepath.Join(root, "go"))
	if _, err := in.Install(ctx, "1.26.5"); err != nil || s.downloads.Load() != 1 {
		t.Fatalf("cached archive not reused: %v, %d", err, s.downloads.Load())
	}
	if vs, err := Installed(home.Layout{Root: root}); err != nil || len(vs) != 1 || vs[0] != "1.26.5" {
		t.Fatalf("Installed = %v, %v", vs, err)
	}
}

func TestInstallErrors(t *testing.T) {
	cases := map[string]struct {
		setup   func(s *server, in *Installer)
		version string
		want    string
	}{
		"list 500":       {setup: func(s *server, _ *Installer) { s.listCode = 500 }, version: "1.26", want: "server responded 500"},
		"file 404":       {setup: func(s *server, _ *Installer) { s.fileCode = 404 }, version: "1.26", want: "server responded 404"},
		"sha mismatch":   {setup: func(s *server, _ *Installer) { s.archive = append(s.archive, 'x') }, version: "1.26", want: "SHA-256 mismatch"},
		"no sha":         {setup: func(_ *server, in *Installer) { in.GOARCH = "nosum" }, version: "1.26", want: "no SHA-256"},
		"no platform":    {setup: func(_ *server, in *Installer) { in.GOOS = "plan9" }, version: "1.26", want: "no archive for plan9/arch"},
		"unknown":        {version: "1.99", want: "not found"},
		"bad server url": {setup: func(_ *server, in *Installer) { in.BaseURL = "http://127.0.0.1:1/" }, version: "1.26", want: "cannot fetch"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := newServer(t)
			root := t.TempDir()
			in := s.installer(t, root)
			if tc.setup != nil {
				tc.setup(s, in)
			}
			_, err := in.Install(ctx, tc.version)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
			if _, err := os.Stat(filepath.Join(root, "go", "1.26.5")); err == nil {
				t.Fatal("failed install must not leave a Go directory")
			}
			if entries, _ := os.ReadDir(filepath.Join(root, "cache", "downloads")); len(entries) != 0 {
				t.Fatalf("failed download must not stay in cache: %v", entries)
			}
		})
	}
}

func TestBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>")) }))
	defer srv.Close()
	in := &Installer{Home: home.Layout{Root: t.TempDir()}, BaseURL: srv.URL}
	if _, err := in.Install(ctx, "1.26"); err == nil || !strings.Contains(err.Error(), "bad response") {
		t.Fatalf("got %v", err)
	}
}

func TestInstalledEmpty(t *testing.T) {
	if vs, err := Installed(home.Layout{Root: t.TempDir()}); err != nil || vs != nil {
		t.Fatalf("got %v %v", vs, err)
	}
}

func TestBadArchive(t *testing.T) {
	s := newServer(t)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("notgo/file")
	w.Write([]byte("x"))
	zw.Close()
	s.archive = buf.Bytes()
	h := sha256.Sum256(s.archive)
	s.sum = hex.EncodeToString(h[:])
	if _, err := s.installer(t, t.TempDir()).Install(ctx, "1.26.5"); err == nil || !strings.Contains(err.Error(), "no go/ directory") {
		t.Fatalf("got %v", err)
	}
}
