// Package goinstall downloads Go releases from go.dev and installs them into
// <GTR_HOME>/go/<version> (ADR-0005).
package goinstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"gtr/internal/archive"
	"gtr/internal/home"
	"gtr/internal/semver"
)

// DefaultBaseURL is the Go downloads URL.
const DefaultBaseURL = "https://go.dev/dl/"

// ErrNotFound is returned when the requested version does not exist.
var ErrNotFound = errors.New("Go version not found")

// Release is an entry from https://go.dev/dl/?mode=json&include=all.
type Release struct {
	Version string `json:"version"` // "go1.26.5"
	Stable  bool   `json:"stable"`
	Files   []File `json:"files"`
}

// File is a release file.
type File struct {
	Filename string `json:"filename"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Version  string `json:"version"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	Kind     string `json:"kind"` // "archive", "installer", "source"
}

// Installer installs Go. All fields except Home are optional.
type Installer struct {
	Home    home.Layout
	BaseURL string       // defaults to DefaultBaseURL
	Client  *http.Client // defaults to a client with a 10-minute timeout
	GOOS    string       // defaults to runtime.GOOS
	GOARCH  string       // defaults to runtime.GOARCH
	Log     io.Writer    // progress messages; discarded by default
}

func (in *Installer) defaults() {
	if in.BaseURL == "" {
		in.BaseURL = DefaultBaseURL
	}
	if !strings.HasSuffix(in.BaseURL, "/") {
		in.BaseURL += "/"
	}
	if in.Client == nil {
		in.Client = &http.Client{Timeout: 10 * time.Minute}
	}
	if in.GOOS == "" {
		in.GOOS = runtime.GOOS
	}
	if in.GOARCH == "" {
		in.GOARCH = runtime.GOARCH
	}
	if in.Log == nil {
		in.Log = io.Discard
	}
}

// Install installs the requested version ("1.26.5", "go1.26.5", "1.26", "1")
// and returns the installed version without the "go" prefix ("1.26.5"). A
// partial version means the latest stable release of that line. A version
// that is already installed is not downloaded again.
func (in *Installer) Install(ctx context.Context, requested string) (string, error) {
	in.defaults()
	releases, err := in.Releases(ctx)
	if err != nil {
		return "", err
	}
	rel, err := Select(releases, requested)
	if err != nil {
		return "", err
	}
	version := strings.TrimPrefix(rel.Version, "go")
	dst := in.Home.GoVersion(version)
	if _, err := os.Stat(filepath.Join(dst, "bin", "go"+exeSuffix(in.GOOS))); err == nil {
		fmt.Fprintf(in.Log, "Go %s is already installed: %s\n", version, dst)
		return version, nil
	}
	file := FindArchive(rel.Files, in.GOOS, in.GOARCH)
	if file == nil {
		return "", fmt.Errorf("Go %s has no archive for %s/%s", version, in.GOOS, in.GOARCH)
	}
	archivePath, err := in.download(ctx, *file)
	if err != nil {
		return "", err
	}
	if err := in.unpack(archivePath, dst); err != nil {
		return "", err
	}
	fmt.Fprintf(in.Log, "Go %s installed: %s\n", version, dst)
	return version, nil
}

// Releases fetches the list of releases.
func (in *Installer) Releases(ctx context.Context) ([]Release, error) {
	in.defaults()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, in.BaseURL+"?mode=json&include=all", nil)
	if err != nil {
		return nil, err
	}
	resp, err := in.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot fetch the list of Go versions: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list of Go versions: server responded %s", resp.Status)
	}
	var releases []Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&releases); err != nil {
		return nil, fmt.Errorf("list of Go versions: bad response: %w", err)
	}
	return releases, nil
}

// Select picks a release. requested is a version or range (ADR-0004 syntax):
// a full version ("1.26.5") or a pre-release ("1.27rc1") selects exactly that
// release; anything else ("1.26", "1", "^1.22", ">=1.26") selects the newest
// stable release in the range — for "1.20" that is 1.20.14, not 1.20.
func Select(releases []Release, requested string) (*Release, error) {
	req := strings.TrimSpace(requested)
	if req == "" {
		return nil, errors.New("specify a Go version, e.g. 1.26.5 or 1.26")
	}
	rng, err := semver.ParseRange(req)
	if err != nil {
		return nil, fmt.Errorf("invalid Go version %q", requested)
	}
	var best *Release
	var bv semver.Version
	for i := range releases {
		r := &releases[i]
		v, err := semver.Parse(r.Version)
		if err != nil || !rng.Match(v) {
			continue
		}
		if rng.Exact() {
			return r, nil
		}
		if !r.Stable || v.Pre != "" {
			continue
		}
		if best == nil || semver.Compare(v, bv) > 0 {
			best, bv = r, v
		}
	}
	if best == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, requested)
	}
	return best, nil
}

// FindArchive returns the archive (.zip/.tar.gz) for the platform, or nil.
func FindArchive(files []File, goos, goarch string) *File {
	for i := range files {
		f := &files[i]
		if f.OS == goos && f.Arch == goarch && f.Kind == "archive" {
			return f
		}
	}
	return nil
}

// download fetches the archive into the cache and verifies its SHA-256. A
// cached archive with a matching checksum is reused.
func (in *Installer) download(ctx context.Context, f File) (string, error) {
	if f.SHA256 == "" {
		return "", fmt.Errorf("%s has no SHA-256 in the release list; refusing to install", f.Filename)
	}
	if strings.ContainsAny(f.Filename, `/\`) || f.Filename == "" || f.Filename == "." || f.Filename == ".." {
		return "", fmt.Errorf("illegal file name %q", f.Filename)
	}
	dir := in.Home.Downloads()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, f.Filename)
	if sum, err := fileSHA256(dst); err == nil && sum == f.SHA256 {
		fmt.Fprintf(in.Log, "Using cached archive: %s\n", dst)
		return dst, nil
	}

	url := in.BaseURL + f.Filename
	fmt.Fprintf(in.Log, "Downloading %s\n", url)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := in.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", f.Filename, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: server responded %s", f.Filename, resp.Status)
	}

	tmp, err := os.CreateTemp(dir, f.Filename+".*.part")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name()) // no-op after rename
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(tmp, h), resp.Body)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", fmt.Errorf("download %s: %w", f.Filename, err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != f.SHA256 {
		return "", fmt.Errorf("download %s: SHA-256 mismatch (expected %s, got %s)", f.Filename, f.SHA256, got)
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return "", err
	}
	return dst, nil
}

// unpack extracts the archive into a temporary directory and moves the
// contents of go/ to dst with a single rename, so a partial install never
// becomes visible.
func (in *Installer) unpack(archivePath, dst string) error {
	if err := os.MkdirAll(in.Home.Tmp(), 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(in.Home.Tmp(), "go-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := archive.Extract(archivePath, tmp); err != nil {
		return err
	}
	root := filepath.Join(tmp, "go")
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return errors.New("the Go archive has no go/ directory")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	// Another gtr process may have installed this version meanwhile (shims
	// make parallel runs likely). Never delete a complete installation that
	// may be in use; only replace leftovers of a failed install.
	complete := func() bool {
		_, err := os.Stat(filepath.Join(dst, "bin", "go"+exeSuffix(in.GOOS)))
		return err == nil
	}
	if complete() {
		return nil
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	if err := os.Rename(root, dst); err != nil && !complete() {
		return err
	}
	return nil
}

// Installed returns the installed Go versions in ascending order.
func Installed(l home.Layout) ([]string, error) {
	entries, err := os.ReadDir(l.GoDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		// os.Stat follows symlinks: a Go linked into go/<version> counts too.
		if st, err := os.Stat(filepath.Join(l.GoDir(), e.Name(), "bin")); err == nil && st.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, ea := semver.Parse(out[i])
		b, eb := semver.Parse(out[j])
		if ea != nil || eb != nil {
			return out[i] < out[j]
		}
		return semver.Compare(a, b) < 0
	})
	return out, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func exeSuffix(goos string) string {
	if goos == "windows" {
		return ".exe"
	}
	return ""
}
