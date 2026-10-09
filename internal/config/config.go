// Package config reads and writes ~/.gtr/config.json: the global default
// versions and local development links.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gtr/internal/home"
)

// Config is the content of config.json.
type Config struct {
	// Go is the default Go version (exact, e.g. "1.26.9").
	Go string `json:"go,omitempty"`
	// Gtr is the default gtr-manager version (exact, e.g. "0.2.0").
	Gtr string `json:"gtr,omitempty"`
	// Links maps a tool ("gtr") to a local binary that overrides installed
	// versions, e.g. a gtr-manager built from the submodule.
	Links map[string]string `json:"links,omitempty"`
}

// Path returns the location of config.json.
func Path(l home.Layout) string { return filepath.Join(l.Root, "config.json") }

// Load reads the config; a missing file is an empty config.
func Load(l home.Layout) (*Config, error) {
	data, err := os.ReadFile(Path(l))
	if errors.Is(err, os.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", Path(l), err)
	}
	return &c, nil
}

// Save writes the config atomically.
func Save(l home.Layout, c *Config) error {
	data, err := json.MarshalIndent(c, "", "    ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(l.Root, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(l.Root, ".config.json.*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), Path(l))
}
