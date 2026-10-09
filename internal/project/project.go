// Package project finds the gtr.json that applies to a directory and reads
// the tool versions it requires.
package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// FileName is the project manifest.
const FileName = "gtr.json"

// Project is the part of gtr.json the launcher needs.
type Project struct {
	Dir  string // directory that contains gtr.json
	Path string // path to gtr.json
	Go   string // required Go range ("" = any)
	Gtr  string // required gtr-manager range ("" = any)
}

// Find walks up from dir to the nearest gtr.json. It returns (nil, nil) if
// there is none.
func Find(dir string) (*Project, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	for {
		path := filepath.Join(dir, FileName)
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			return Load(path)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, nil
		}
		dir = parent
	}
}

// Load reads one gtr.json. "engines.go" is preferred; the legacy "engine"
// field written by older `gtr-manager init` is read as the Go range.
func Load(path string) (*Project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Engines struct {
			Go  string `json:"go"`
			Gtr string `json:"gtr"`
		} `json:"engines"`
		Engine string `json:"engine"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	p := &Project{Dir: filepath.Dir(path), Path: path, Go: raw.Engines.Go, Gtr: raw.Engines.Gtr}
	if p.Go == "" {
		p.Go = raw.Engine
	}
	return p, nil
}
