// Package home locates the gtr home directory (ADR-0005): ~/.gtr by default,
// overridden by the GTR_HOME environment variable.
package home

import (
	"fmt"
	"os"
	"path/filepath"
)

// Dir returns the gtr home directory.
func Dir() (string, error) {
	if d := os.Getenv("GTR_HOME"); d != "" {
		return filepath.Abs(d)
	}
	u, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine the user home directory (set GTR_HOME): %w", err)
	}
	return filepath.Join(u, ".gtr"), nil
}

// Layout describes paths inside the home directory.
type Layout struct{ Root string }

// New returns the layout for the default home directory.
func New() (Layout, error) {
	d, err := Dir()
	return Layout{Root: d}, err
}

// GoDir returns the directory of installed Go versions: <root>/go.
func (l Layout) GoDir() string { return filepath.Join(l.Root, "go") }

// GoVersion returns the directory of a single Go version: <root>/go/1.26.5.
func (l Layout) GoVersion(v string) string { return filepath.Join(l.GoDir(), v) }

// ManagerDir returns the directory of installed gtr-manager versions: <root>/gtr.
func (l Layout) ManagerDir() string { return filepath.Join(l.Root, "gtr") }

// Downloads returns the cache of downloaded archives: <root>/cache/downloads.
func (l Layout) Downloads() string { return filepath.Join(l.Root, "cache", "downloads") }

// Tmp returns the temporary directory for extraction: <root>/tmp (on the same
// disk as the final directories, so moving is an atomic rename).
func (l Layout) Tmp() string { return filepath.Join(l.Root, "tmp") }
