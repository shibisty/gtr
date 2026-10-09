// Package manager installs gtr-manager versions from GitHub releases into
// ~/.gtr/gtr/<version>/ (ADR-0005, ADR-0008).
package manager

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"gtr/internal/home"
	"gtr/internal/release"
	"gtr/internal/semver"
)

// Repo is the repository that publishes gtr-manager releases.
const Repo = "shibisty/gtr-manager"

// Installer installs gtr-manager.
type Installer struct {
	Home   home.Layout
	Source *release.Source
	Log    io.Writer
}

// Binary returns the path of an installed version's executable.
func Binary(l home.Layout, version, binaryName string) string {
	return filepath.Join(l.ManagerDir(), version, binaryName)
}

// Install installs the highest release matching req ("0.2.1", "^0.2", "*")
// and returns its version. An installed version is not downloaded again.
func (in *Installer) Install(ctx context.Context, req string) (string, error) {
	rng, err := semver.ParseRange(req)
	if err != nil {
		return "", err
	}
	log := in.Log
	if log == nil {
		log = io.Discard
	}
	bin := in.Source.BinaryName()
	if rng.Exact() {
		v := semver.MustParse(req).String()
		if _, err := os.Stat(Binary(in.Home, v, bin)); err == nil {
			return v, nil
		}
	}
	releases, err := in.Source.Releases(ctx)
	if err != nil {
		return "", fmt.Errorf("gtr-manager: %w (for a local build use `gtr self link gtr <path>`)", err)
	}
	rel, err := release.Select(releases, rng)
	if err != nil {
		if errors.Is(err, release.ErrNoRelease) {
			return "", fmt.Errorf("gtr-manager: %w (see https://github.com/%s/releases; "+
				"for a local build use `gtr self link gtr <path>`)", err, in.Source.Repo)
		}
		return "", err
	}
	version := semver.MustParse(rel.Version()).String()
	dst := filepath.Join(in.Home.ManagerDir(), version)
	if _, err := os.Stat(filepath.Join(dst, bin)); err == nil {
		return version, nil
	}
	path, cleanup, err := in.Source.Fetch(ctx, rel)
	if err != nil {
		return "", err
	}
	defer cleanup()
	// Move the binary into its own directory inside tmp, then rename the whole
	// directory into place, so a half-installed version never appears.
	stage := filepath.Join(filepath.Dir(path), ".stage-"+version)
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(path, filepath.Join(stage, bin)); err != nil {
		return "", err
	}
	if err := os.Chmod(filepath.Join(stage, bin), 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(in.Home.ManagerDir(), 0o755); err != nil {
		return "", err
	}
	if err := moveIntoPlace(stage, dst, bin); err != nil {
		return "", err
	}
	fmt.Fprintf(log, "gtr-manager %s installed: %s\n", version, dst)
	return version, nil
}

// moveIntoPlace renames stage to dst. Another gtr process may have
// installed the same version meanwhile: a complete dst is kept and counts as
// success, only a leftover without the binary is replaced.
func moveIntoPlace(stage, dst, bin string) error {
	complete := func() bool { _, err := os.Stat(filepath.Join(dst, bin)); return err == nil }
	if complete() {
		return nil
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	if err := os.Rename(stage, dst); err != nil && !complete() {
		return err
	}
	return nil
}

// Installed returns installed versions, lowest first.
func Installed(l home.Layout, binaryName string) ([]string, error) {
	entries, err := os.ReadDir(l.ManagerDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(l.ManagerDir(), e.Name(), binaryName)); err == nil {
			out = append(out, e.Name())
		}
	}
	sortVersions(out)
	return out, nil
}

func sortVersions(vs []string) {
	sort.SliceStable(vs, func(i, j int) bool {
		a, ea := semver.Parse(vs[i])
		b, eb := semver.Parse(vs[j])
		if ea != nil || eb != nil {
			return vs[i] < vs[j]
		}
		return semver.Compare(a, b) < 0
	})
}
