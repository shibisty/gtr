// Package resolve decides which Go and gtr-manager versions apply to a
// directory (ADR-0008) and installs them on demand.
//
// Go, highest priority first:
//
//  1. GTR_GO_VERSION (a version or range);
//  2. the nearest gtr.json: engines.go (or the legacy "engine" field);
//     in a gtr workspace also at least the go line of its go.work;
//  3. the global default set with `gtr self use go`.
//
// gtr-manager:
//
//  1. a local build linked with `gtr self link gtr <path>`;
//  2. GTR_MANAGER_VERSION;
//  3. engines.gtr in the nearest gtr.json;
//  4. the global default set with `gtr self use gtr`;
//  5. the latest release.
//
// A range picks the highest installed version that satisfies it; if none is
// installed, the newest matching release is installed.
package resolve

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"gtr/internal/config"
	"gtr/internal/goenv"
	"gtr/internal/goinstall"
	"gtr/internal/home"
	"gtr/internal/manager"
	"gtr/internal/project"
	"gtr/internal/semver"
)

// Installer installs a version matching a range and returns the version.
type Installer interface {
	Install(ctx context.Context, req string) (string, error)
}

// Choice is a resolved tool version.
type Choice struct {
	Version string // exact version, e.g. "1.26.9"; "link" for a linked build
	Path    string // Go: GOROOT; gtr-manager: path to the executable
	Source  string // why this version was chosen, for `gtr self current`
}

// Resolver resolves versions for one gtr home.
type Resolver struct {
	Home             home.Layout
	Getenv           func(string) string
	GoInstaller      Installer
	ManagerInstaller Installer
	ManagerBinary    string // "gtr-manager" or "gtr-manager.exe"
	Log              io.Writer
}

func (r *Resolver) log() io.Writer {
	if r.Log == nil {
		return io.Discard
	}
	return r.Log
}

// Go returns the Go version for dir, or (nil, nil) if nothing selects one.
func (r *Resolver) Go(ctx context.Context, dir string) (*Choice, error) {
	req, source, err := r.goRequest(dir)
	if err != nil || req == "" {
		return nil, err
	}
	installed, err := goinstall.Installed(r.Home)
	if err != nil {
		return nil, err
	}
	v, err := r.pick(ctx, "Go", req, source, installed, r.GoInstaller)
	if err != nil {
		return nil, err
	}
	return &Choice{Version: v, Path: r.Home.GoVersion(v), Source: source}, nil
}

func (r *Resolver) goRequest(dir string) (req, source string, err error) {
	if v := r.Getenv("GTR_GO_VERSION"); v != "" {
		return v, "GTR_GO_VERSION=" + v, nil
	}
	p, err := project.Find(dir)
	if err != nil {
		return "", "", err
	}
	if p != nil && p.Go != "" {
		req, source = p.Go, fmt.Sprintf("%s (engines.go %q)", p.Path, p.Go)
	} else {
		c, err := config.Load(r.Home)
		if err != nil {
			return "", "", err
		}
		if c.Go != "" {
			req, source = c.Go, "default (gtr self use go)"
		}
	}
	// In a workspace member, go.work may need a newer Go than the member
	// asks for (another member, or the root, raised it; ADR-0009).
	if work := goenv.FindWork(dir); work != "" {
		if lang := goLanguage(goenv.WorkGo(work)); lang != "" {
			if req == "" {
				req, source = ">="+lang, fmt.Sprintf("%s (go %s)", work, lang)
			} else if rng, err := semver.ParseRange(req); err == nil {
				req, source = rng.AtLeast(semver.MustParse(lang+".0")), source+fmt.Sprintf(" and %s (go %s)", work, lang)
			}
		}
	}
	return req, source, nil
}

// goLanguage returns major.minor of a go directive ("1.23.1" → "1.23").
func goLanguage(v string) string {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return ""
	}
	minor := parts[1]
	if i := strings.IndexFunc(minor, func(r rune) bool { return r < '0' || r > '9' }); i >= 0 {
		minor = minor[:i]
	}
	if minor == "" {
		return ""
	}
	return parts[0] + "." + minor
}

// Manager returns the gtr-manager for dir.
func (r *Resolver) Manager(ctx context.Context, dir string) (*Choice, error) {
	c, err := config.Load(r.Home)
	if err != nil {
		return nil, err
	}
	if link := c.Links["gtr"]; link != "" {
		return &Choice{Version: "link", Path: link, Source: "linked with gtr self link gtr"}, nil
	}
	req, source := "", ""
	switch p, err := project.Find(dir); {
	case err != nil:
		return nil, err
	case r.Getenv("GTR_MANAGER_VERSION") != "":
		req = r.Getenv("GTR_MANAGER_VERSION")
		source = "GTR_MANAGER_VERSION=" + req
	case p != nil && p.Gtr != "":
		req, source = p.Gtr, fmt.Sprintf("%s (engines.gtr %q)", p.Path, p.Gtr)
	case c.Gtr != "":
		req, source = c.Gtr, "default (gtr self use gtr)"
	default:
		req, source = "*", "latest"
	}
	installed, err := manager.Installed(r.Home, r.ManagerBinary)
	if err != nil {
		return nil, err
	}
	v, err := r.pick(ctx, "gtr-manager", req, source, installed, r.ManagerInstaller)
	if err != nil {
		return nil, err
	}
	return &Choice{Version: v, Path: manager.Binary(r.Home, v, r.ManagerBinary), Source: source}, nil
}

// pick returns the best installed version for req or installs one.
func (r *Resolver) pick(ctx context.Context, tool, req, source string, installed []string, in Installer) (string, error) {
	rng, err := semver.ParseRange(req)
	if err != nil {
		return "", fmt.Errorf("%s version from %s: %w", tool, source, err)
	}
	if v, ok := rng.Best(installed); ok {
		return v, nil
	}
	fmt.Fprintf(r.log(), "%s %s is required by %s; installing it...\n", tool, rng, source)
	v, err := in.Install(ctx, req)
	if err != nil {
		return "", fmt.Errorf("install %s %s (required by %s): %w", tool, rng, source, err)
	}
	return v, nil
}

// GoBin returns the path of a Go tool ("go", "gofmt") inside a GOROOT.
func GoBin(goroot, name, goos string) string {
	if goos == "windows" {
		name += ".exe"
	}
	return filepath.Join(goroot, "bin", name)
}
