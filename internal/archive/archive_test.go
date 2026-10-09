package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type entry struct {
	name, body, link string
	dir              bool
	mode             int64
}

func writeZip(t *testing.T, entries []entry) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		switch {
		case e.dir:
			h.SetMode(os.ModeDir | 0o755)
		case e.link != "":
			h.SetMode(os.ModeSymlink | 0o777)
		default:
			h.SetMode(0o644)
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(e.body + e.link))
	}
	zw.Close()
	p := filepath.Join(t.TempDir(), "a.zip")
	os.WriteFile(p, buf.Bytes(), 0o644)
	return p
}

func writeTgz(t *testing.T, entries []entry) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: 0o644}
		if e.mode != 0 {
			h.Mode = e.mode
		}
		switch {
		case e.dir:
			h.Typeflag, h.Mode = tar.TypeDir, 0o755
		case e.link != "":
			h.Typeflag, h.Linkname = tar.TypeSymlink, e.link
		default:
			h.Typeflag, h.Size = tar.TypeReg, int64(len(e.body))
		}
		tw.WriteHeader(h)
		tw.Write([]byte(e.body))
	}
	tw.Close()
	gz.Close()
	p := filepath.Join(t.TempDir(), "a.tar.gz")
	os.WriteFile(p, buf.Bytes(), 0o644)
	return p
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestExtract(t *testing.T) {
	entries := []entry{{name: "go/", dir: true}, {name: "go/bin/go", body: "binary", mode: 0o755}, {name: "go/VERSION", body: "go1.26.5"}}
	for name, src := range map[string]string{"zip": writeZip(t, entries), "tgz": writeTgz(t, entries)} {
		t.Run(name, func(t *testing.T) {
			dst := filepath.Join(t.TempDir(), "out")
			if err := Extract(src, dst); err != nil {
				t.Fatal(err)
			}
			if read(t, filepath.Join(dst, "go", "VERSION")) != "go1.26.5" || read(t, filepath.Join(dst, "go", "bin", "go")) != "binary" {
				t.Fatal("wrong content")
			}
			if name == "tgz" && runtime.GOOS != "windows" {
				if st, _ := os.Stat(filepath.Join(dst, "go", "bin", "go")); st.Mode().Perm()&0o100 == 0 {
					t.Fatal("executable bit lost")
				}
			}
		})
	}
}

func TestZipSlip(t *testing.T) {
	for _, bad := range []string{"../evil", "a/../../evil", "/abs/evil"} {
		for name, src := range map[string]string{
			"zip": writeZip(t, []entry{{name: bad, body: "x"}}),
			"tgz": writeTgz(t, []entry{{name: bad, body: "x"}}),
		} {
			base := t.TempDir()
			dst := filepath.Join(base, "out")
			err := Extract(src, dst)
			if err == nil || !strings.Contains(err.Error(), "illegal path") {
				t.Errorf("%s %q: want illegal path error, got %v", name, bad, err)
			}
			if _, err := os.Stat(filepath.Join(base, "evil")); err == nil {
				t.Errorf("%s %q: file written outside target", name, bad)
			}
		}
	}
}

func TestLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	ok := writeTgz(t, []entry{{name: "d/f", body: "x"}, {name: "d/l", link: "f"}})
	dst := t.TempDir()
	if err := Extract(ok, dst); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dst, "d", "l")) != "x" {
		t.Fatal("symlink inside target must work")
	}
	for _, link := range []string{"../../etc/passwd", "/etc/passwd"} {
		if err := Extract(writeTgz(t, []entry{{name: "l", link: link}}), t.TempDir()); err == nil {
			t.Errorf("link to %s must be rejected", link)
		}
	}
	// A chain of links that each look harmless must not lead outside.
	base := t.TempDir()
	chain := writeTgz(t, []entry{{name: "a/d", link: ".."}, {name: "a/d/e", link: ".."}, {name: "a/d/e/evil", body: "x"}})
	if err := Extract(chain, filepath.Join(base, "out")); err == nil {
		t.Error("writing through a link chain must be rejected")
	}
	if _, err := os.Stat(filepath.Join(base, "evil")); err == nil {
		t.Error("file escaped through a link chain")
	}
	// A file entry must not overwrite the target of an earlier link.
	outside := filepath.Join(t.TempDir(), "victim")
	os.WriteFile(outside, []byte("safe"), 0o644)
	if err := Extract(writeTgz(t, []entry{{name: "f", link: "g"}, {name: "f", body: "pwned"}}), t.TempDir()); err == nil {
		t.Error("writing through a link must be rejected")
	}
	if err := Extract(writeZip(t, []entry{{name: "l", link: "x"}}), t.TempDir()); err == nil {
		t.Error("zip symlink must be rejected")
	}
}

func TestErrors(t *testing.T) {
	if err := Extract("a.rar", t.TempDir()); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("got %v", err)
	}
	bad := filepath.Join(t.TempDir(), "bad.zip")
	os.WriteFile(bad, []byte("not a zip"), 0o644)
	if Extract(bad, t.TempDir()) == nil {
		t.Fatal("corrupt zip must fail")
	}
	badgz := filepath.Join(t.TempDir(), "bad.tar.gz")
	os.WriteFile(badgz, []byte("not gzip"), 0o644)
	if Extract(badgz, t.TempDir()) == nil {
		t.Fatal("corrupt tgz must fail")
	}
	if Extract(filepath.Join(t.TempDir(), "missing.tgz"), t.TempDir()) == nil {
		t.Fatal("missing file must fail")
	}
}
