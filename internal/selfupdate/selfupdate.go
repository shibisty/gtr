// Package selfupdate replaces the running gtr executable with a release.
package selfupdate

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gtr/internal/release"
	"gtr/internal/semver"
)

// Repo publishes gtr releases.
const Repo = "shibisty/gtr"

// Update installs the release matching req ("" = latest) over exe and
// returns the new version. It does nothing if current is already that
// version.
//
// The running file cannot be overwritten on Windows, but it can be renamed:
// exe is moved to exe.old, the new binary takes its place, and exe.old is
// removed (on Windows: by Cleanup on the next start).
func Update(ctx context.Context, src *release.Source, current, req, exe string, log io.Writer) (string, error) {
	if req == "" {
		req = "*"
	}
	rng, err := semver.ParseRange(req)
	if err != nil {
		return "", err
	}
	releases, err := src.Releases(ctx)
	if err != nil {
		return "", err
	}
	rel, err := release.Select(releases, rng)
	if err != nil {
		return "", err
	}
	version := semver.MustParse(rel.Version()).String()
	if cv, err := semver.Parse(current); err == nil && semver.Compare(cv, semver.MustParse(version)) == 0 {
		fmt.Fprintf(log, "gtr %s is already installed\n", version)
		return version, nil
	}
	bin, cleanup, err := src.Fetch(ctx, rel)
	if err != nil {
		return "", err
	}
	defer cleanup()
	if err := Replace(exe, bin); err != nil {
		return "", err
	}
	fmt.Fprintf(log, "gtr updated: %s → %s\n", current, version)
	return version, nil
}

// Replace puts the file newBin in place of exe.
func Replace(exe, newBin string) error {
	data, err := os.ReadFile(newBin)
	if err != nil {
		return err
	}
	st, err := os.Stat(exe)
	if err != nil {
		return err
	}
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, filepath.Base(exe)+".new-*")
	if err != nil {
		return fmt.Errorf("cannot write next to %s (no permission?): %w", exe, err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), st.Mode().Perm()|0o111); err != nil {
		return err
	}
	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), exe); err != nil {
		os.Rename(old, exe) // put the working binary back
		return err
	}
	os.Remove(old) // fails on Windows while running; Cleanup removes it later
	return nil
}

// Cleanup removes exe.old left by a previous update.
func Cleanup(exe string) { os.Remove(exe + ".old") }
