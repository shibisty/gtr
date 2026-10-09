// Package archive extracts .zip and .tar.gz files without letting entries
// escape the target directory (zip-slip protection).
package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrUnsupported is returned for an unsupported archive format.
var ErrUnsupported = errors.New("archive: only .zip and .tar.gz are supported")

// Extract extracts src into dst, creating dst if needed.
func Extract(src, dst string) error {
	switch {
	case strings.HasSuffix(src, ".zip"):
		return unzip(src, dst)
	case strings.HasSuffix(src, ".tar.gz"), strings.HasSuffix(src, ".tgz"):
		return untargz(src, dst)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupported, filepath.Base(src))
	}
}

// target returns the path inside dst, or an error if the name escapes it.
func target(dst, name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" || clean == ".." ||
		strings.HasPrefix(clean, ".."+string(filepath.Separator)) || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("archive: illegal path %q in archive", name)
	}
	return filepath.Join(dst, clean), nil
}

func unzip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, f := range r.File {
		path, err := target(dst, f.Name)
		if err != nil {
			return err
		}
		mode := f.Mode()
		switch {
		case mode.IsDir():
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
		case mode&os.ModeSymlink != 0:
			return fmt.Errorf("archive: symlinks are not supported: %q", f.Name)
		default:
			rc, err := f.Open()
			if err != nil {
				return err
			}
			err = writeFile(path, rc, mode.Perm())
			rc.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func untargz(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		path, err := target(dst, hdr.Name)
		if err != nil {
			return err
		}
		// Links created by earlier entries must not redirect later ones:
		// "a/d -> ..", then "a/d/e -> ..", then "a/d/e/evil" escapes dst.
		if err := noLinkOnPath(dst, path); err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFile(path, tr, os.FileMode(hdr.Mode).Perm()); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// A link is allowed only if it points inside dst.
			if filepath.IsAbs(hdr.Linkname) {
				return fmt.Errorf("archive: link %q points to an absolute path", hdr.Name)
			}
			rel, err := filepath.Rel(dst, filepath.Join(filepath.Dir(path), filepath.FromSlash(hdr.Linkname)))
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("archive: link %q points outside the target directory", hdr.Name)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(hdr.Linkname, path); err != nil {
				return err
			}
		case tar.TypeLink:
			return fmt.Errorf("archive: hard links are not supported: %q", hdr.Name)
		default:
			// PAX headers and other metadata entries are skipped.
		}
	}
}

// noLinkOnPath fails if any existing component of path below dst (including
// path itself) is a symbolic link.
func noLinkOnPath(dst, path string) error {
	rel, err := filepath.Rel(dst, path)
	if err != nil {
		return err
	}
	cur := dst
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		st, err := os.Lstat(cur)
		if err != nil {
			return nil // does not exist yet; nothing below it exists either
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive: %q would be written through the link %q", path, cur)
		}
	}
	return nil
}

func writeFile(path string, r io.Reader, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if perm == 0 {
		perm = 0o644
	}
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm|0o200)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
