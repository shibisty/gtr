// Package release downloads binaries published as GitHub release assets.
//
// Asset naming (produced by the release workflow of gtr and gtr-manager):
//
//	<app>_<version>_<os>_<arch>.zip      Windows
//	<app>_<version>_<os>_<arch>.tar.gz   other systems
//	SHA256SUMS                           "<sha256>  <asset name>" per line
//
// Every archive is verified against SHA256SUMS before it is unpacked.
package release

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gtr/internal/archive"
	"gtr/internal/home"
	"gtr/internal/semver"
)

// DefaultAPI is the GitHub REST API.
const DefaultAPI = "https://api.github.com"

// ChecksumsAsset is the name of the checksum file in every release.
const ChecksumsAsset = "SHA256SUMS"

// ErrNoRelease means no published release satisfies the request.
var ErrNoRelease = errors.New("no matching release")

// Release is a published GitHub release.
type Release struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

// Asset is a release file.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// Version returns the tag without the "v" prefix.
func (r Release) Version() string { return strings.TrimPrefix(r.Tag, "v") }

// Source downloads releases of one repository.
type Source struct {
	App    string // asset prefix and binary name: "gtr-manager", "gtr"
	Repo   string // "shibisty/gtr-manager"
	API    string // DefaultAPI if empty
	Token  string // optional GitHub token (GITHUB_TOKEN)
	Client *http.Client
	GOOS   string
	GOARCH string
	Home   home.Layout
	Log    io.Writer
}

func (s *Source) defaults() {
	if s.API == "" {
		s.API = DefaultAPI
	}
	if s.Client == nil {
		s.Client = &http.Client{Timeout: 5 * time.Minute}
	}
	if s.GOOS == "" {
		s.GOOS = runtime.GOOS
	}
	if s.GOARCH == "" {
		s.GOARCH = runtime.GOARCH
	}
	if s.Log == nil {
		s.Log = io.Discard
	}
}

func (s *Source) get(ctx context.Context, url string, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	if s.Token != "" && sameHost(url, s.API) {
		req.Header.Set("Authorization", "Bearer "+s.Token)
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.Repo, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		if resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return nil, fmt.Errorf("%s: GitHub API rate limit exceeded; set GITHUB_TOKEN", s.Repo)
		}
		return nil, fmt.Errorf("%s: %s responded %s", s.Repo, url, resp.Status)
	}
	return resp, nil
}

// sameHost reports whether rawURL is on the API server (scheme and host).
// Asset downloads redirect to other hosts and never get the token.
func sameHost(rawURL, api string) bool {
	u, err1 := url.Parse(rawURL)
	a, err2 := url.Parse(api)
	return err1 == nil && err2 == nil && u.Scheme == a.Scheme && strings.EqualFold(u.Host, a.Host)
}

// Releases lists published (non-draft) releases.
func (s *Source) Releases(ctx context.Context) ([]Release, error) {
	s.defaults()
	resp, err := s.get(ctx, strings.TrimRight(s.API, "/")+"/repos/"+s.Repo+"/releases?per_page=100", "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var all []Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&all); err != nil {
		return nil, fmt.Errorf("%s: bad releases response: %w", s.Repo, err)
	}
	out := all[:0]
	for _, r := range all {
		if !r.Draft {
			out = append(out, r)
		}
	}
	return out, nil
}

// Select picks the highest release matching the range. GitHub pre-releases
// are skipped unless the range names a pre-release version.
func Select(releases []Release, rng semver.Range) (*Release, error) {
	var best *Release
	var bv semver.Version
	for i := range releases {
		r := &releases[i]
		v, err := semver.Parse(r.Version())
		// A release flagged "pre-release" on GitHub is used only when the range
		// names that exact version.
		if err != nil || !rng.Match(v) || (r.Prerelease && !rng.Exact()) {
			continue
		}
		if best == nil || semver.Compare(v, bv) > 0 {
			best, bv = r, v
		}
	}
	if best == nil {
		return nil, fmt.Errorf("%w for %s", ErrNoRelease, rng)
	}
	return best, nil
}

// AssetName returns the archive name for a version on this platform.
func (s *Source) AssetName(version string) string {
	s.defaults()
	ext := ".tar.gz"
	if s.GOOS == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("%s_%s_%s_%s%s", s.App, version, s.GOOS, s.GOARCH, ext)
}

// BinaryName is the executable file name on this platform.
func (s *Source) BinaryName() string {
	s.defaults()
	if s.GOOS == "windows" {
		return s.App + ".exe"
	}
	return s.App
}

// Fetch downloads the archive of r for this platform, verifies it against
// SHA256SUMS, unpacks it into a temporary directory under ~/.gtr/tmp and
// returns the path of the binary inside it. The caller removes cleanup().
func (s *Source) Fetch(ctx context.Context, r *Release) (binary string, cleanup func(), err error) {
	s.defaults()
	name := s.AssetName(r.Version())
	var archiveAsset, sums *Asset
	for i := range r.Assets {
		switch r.Assets[i].Name {
		case name:
			archiveAsset = &r.Assets[i]
		case ChecksumsAsset:
			sums = &r.Assets[i]
		}
	}
	if archiveAsset == nil {
		return "", nil, fmt.Errorf("%s %s has no build for %s/%s (%s)", s.App, r.Version(), s.GOOS, s.GOARCH, name)
	}
	if sums == nil {
		return "", nil, fmt.Errorf("%s %s has no %s; refusing to install an unverified binary", s.App, r.Version(), ChecksumsAsset)
	}
	want, err := s.checksum(ctx, sums.URL, name)
	if err != nil {
		return "", nil, err
	}
	archivePath, err := s.download(ctx, archiveAsset.URL, name, want)
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(s.Home.Tmp(), 0o755); err != nil {
		return "", nil, err
	}
	tmp, err := os.MkdirTemp(s.Home.Tmp(), s.App+"-*")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { os.RemoveAll(tmp) }
	if err := archive.Extract(archivePath, tmp); err != nil {
		cleanup()
		return "", nil, err
	}
	bin, err := findFile(tmp, s.BinaryName())
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("%s %s: %w", s.App, r.Version(), err)
	}
	return bin, cleanup, nil
}

func (s *Source) checksum(ctx context.Context, url, name string) (string, error) {
	resp, err := s.get(ctx, url, "application/octet-stream")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 1<<20))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name && len(f[0]) == 64 {
			return strings.ToLower(f[0]), nil
		}
	}
	return "", fmt.Errorf("%s does not list %s", ChecksumsAsset, name)
}

func (s *Source) download(ctx context.Context, url, name, wantSHA string) (string, error) {
	dir := s.Home.Downloads()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, name)
	if sum, err := fileSHA256(dst); err == nil && sum == wantSHA {
		return dst, nil
	}
	fmt.Fprintf(s.Log, "Downloading %s\n", name)
	resp, err := s.get(ctx, url, "application/octet-stream")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	tmp, err := os.CreateTemp(dir, name+".*.part")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(tmp, h), resp.Body)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", fmt.Errorf("download %s: %w", name, err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != wantSHA {
		return "", fmt.Errorf("download %s: SHA-256 mismatch (expected %s, got %s)", name, wantSHA, got)
	}
	return dst, os.Rename(tmp.Name(), dst)
}

func findFile(root, name string) (string, error) {
	var found string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == name && found == "" {
			found = path
		}
		return nil
	})
	if err == nil && found == "" {
		err = fmt.Errorf("archive does not contain %s", name)
	}
	return found, err
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
