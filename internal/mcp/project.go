package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gtr/schema"
)

// projectInfo answers the project_info tool: gtr.json, the workspace
// members, the lock, and gtr env.
func projectInfo(ctx context.Context, s *Server, a args, dir string) (map[string]any, error) {
	manifest, mdir := findManifest(dir, s.root)
	out := map[string]any{"root": s.root}
	if manifest == "" {
		out["gtr_json"] = nil
		out["hint"] = "no gtr.json here: gtr init creates one (or gtr new <name> a project)"
	} else {
		data, err := os.ReadFile(manifest)
		if err != nil {
			return nil, err
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("%s: %v", manifest, err)
		}
		rel, _ := filepath.Rel(s.root, mdir)
		out["dir"] = filepath.ToSlash(rel)
		out["gtr_json"] = m
		_, err = os.Stat(filepath.Join(mdir, "gtr.lock"))
		out["locked"] = err == nil
		if ws, ok := m["workspaces"].([]any); ok {
			out["members"] = members(mdir, s.root, ws)
		}
		if scripts, ok := m["scripts"].(map[string]any); ok {
			names := make([]string, 0, len(scripts))
			for k := range scripts {
				names = append(names, k)
			}
			sort.Strings(names)
			out["scripts"] = names
		}
	}
	res, err := s.o.Runner.Run(ctx, dir, []string{"env", "--json"}, nil)
	if err == nil && res.ExitCode == 0 {
		parseJSONOutput("env")(res.Output, out)
	} else if err == nil {
		out["env_error"] = strings.TrimSpace(res.Output)
	}
	out["gtr_version"] = s.o.Version
	return out, nil
}

// findManifest walks up from dir to the nearest gtr.json, not above root.
func findManifest(dir, root string) (path, mdir string) {
	for {
		p := filepath.Join(dir, "gtr.json")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, dir
		}
		if dir == root {
			return "", ""
		}
		parent := filepath.Dir(dir)
		if parent == dir || !within(root, parent) {
			return "", ""
		}
		dir = parent
	}
}

// members expands the workspace globs of a root gtr.json.
func members(dir, root string, globs []any) []any {
	var out []any
	seen := map[string]bool{}
	for _, g := range globs {
		pat, _ := g.(string)
		if pat == "" {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(dir, filepath.FromSlash(pat)))
		sort.Strings(matches)
		for _, m := range matches {
			data, err := os.ReadFile(filepath.Join(m, "gtr.json"))
			if err != nil || seen[m] {
				continue
			}
			seen[m] = true
			var mf struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}
			json.Unmarshal(data, &mf)
			rel, _ := filepath.Rel(root, m)
			out = append(out, map[string]any{"dir": filepath.ToSlash(rel), "name": mf.Name, "version": mf.Version})
		}
	}
	return out
}

// Resources: the root's gtr.json and gtr.lock, and the gtr.json schema.
func (s *Server) resourceList() []any {
	var out []any
	for _, r := range []struct{ uri, name, title, desc, file string }{
		{"gtr://gtr.json", "gtr.json", "gtr.json", "The project manifest: dependencies, scripts, engines, build and test settings.", "gtr.json"},
		{"gtr://gtr.lock", "gtr.lock", "gtr.lock", "The exact versions, sources and hashes gtr installed.", "gtr.lock"},
	} {
		if st, err := os.Stat(filepath.Join(s.root, r.file)); err == nil && !st.IsDir() {
			out = append(out, map[string]any{"uri": r.uri, "name": r.name, "title": r.title, "description": r.desc,
				"mimeType": "application/json", "size": st.Size()})
		}
	}
	out = append(out, map[string]any{"uri": "gtr://schema", "name": "gtr.schema.json", "title": "gtr.json schema",
		"description": "JSON Schema of gtr.json.", "mimeType": "application/schema+json", "size": len(schema.JSON)})
	return out
}

func (s *Server) readResource(req *request) (any, error) {
	var p struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(req.params, &p); err != nil {
		return nil, errorf(codeInvalidParams, "resources/read: %v", err)
	}
	notFound := func() error {
		code := codeResourceNotFound
		if req.modern {
			code = codeInvalidParams
		}
		return &rpcError{Code: code, Message: "Resource not found", Data: map[string]any{"uri": p.URI}}
	}
	var text, mime string
	switch p.URI {
	case "gtr://schema":
		text, mime = string(schema.JSON), "application/schema+json"
	case "gtr://gtr.json", "gtr://gtr.lock":
		data, err := os.ReadFile(filepath.Join(s.root, strings.TrimPrefix(p.URI, "gtr://")))
		if err != nil {
			return nil, notFound()
		}
		text, mime = string(data), "application/json"
	default:
		return nil, notFound()
	}
	return map[string]any{
		"contents": []any{map[string]any{"uri": p.URI, "mimeType": mime, "text": text}},
		"ttlMs":    0, "cacheScope": "private",
	}, nil
}
