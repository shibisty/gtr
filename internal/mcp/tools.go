package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// tool is one MCP tool: a gtr command with a JSON schema for its
// arguments.
type tool struct {
	name, title, description string
	props                    map[string]any
	required                 []string
	// hints for the client (MCP tool annotations): it may ask before
	// running tools that change the project.
	readOnly, destructive, idempotent, openWorld bool
	timeout                                      time.Duration
	// args builds the gtr command line.
	args func(a args) ([]string, error)
	// parse adds structured fields from the command's output.
	parse func(out string, into map[string]any)
	// call replaces running a command (project_info).
	call func(ctx context.Context, s *Server, a args, dir string) (map[string]any, error)
}

// Default and largest time limits of a tool run.
const (
	shortTimeout = 2 * time.Minute
	longTimeout  = 10 * time.Minute
	maxTimeout   = time.Hour
)

func (t *tool) describe() map[string]any {
	props := map[string]any{
		"dir": prop("string", "Directory to run in, relative to the project root (a workspace member such as \"core\"); default: the root."),
	}
	if t.call == nil {
		props["timeout_seconds"] = prop("integer", fmt.Sprintf("Time limit (default %d, at most %d).", int(t.timeout.Seconds()), int(maxTimeout.Seconds())))
	}
	for k, v := range t.props {
		props[k] = v
	}
	schema := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(t.required) > 0 {
		schema["required"] = t.required
	}
	return map[string]any{
		"name":        t.name,
		"title":       t.title,
		"description": t.description,
		"inputSchema": schema,
		"annotations": map[string]any{
			"title":           t.title,
			"readOnlyHint":    t.readOnly,
			"destructiveHint": t.destructive,
			"idempotentHint":  t.idempotent,
			"openWorldHint":   t.openWorld,
		},
	}
}

func prop(typ, desc string) map[string]any { return map[string]any{"type": typ, "description": desc} }

func strList(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
}

func enum(desc string, values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values, "description": desc}
}

// toolTable lists the tools in a fixed order (clients cache the list).
func (s *Server) toolTable() []*tool {
	read := func(t *tool) *tool {
		t.readOnly, t.idempotent = true, true
		if t.timeout == 0 {
			t.timeout = shortTimeout
		}
		return t
	}
	change := func(t *tool) *tool {
		if t.timeout == 0 {
			t.timeout = 5 * time.Minute
		}
		return t
	}
	tools := []*tool{
		read(&tool{name: "project_info", title: "Project info",
			description: "The project's gtr.json, its workspace members, whether gtr.lock exists, and the Go and gtr-manager versions and paths gtr uses here (gtr env). Start here.",
			call:        projectInfo}),
		read(&tool{name: "list", title: "List packages",
			description: "Installed packages from gtr.lock, as JSON (name, version, source, dependencies). tree: an indented dependency tree instead.",
			props:       map[string]any{"tree": prop("boolean", "Print the dependency tree."), "depth": prop("integer", "Depth of the tree.")},
			args: func(a args) ([]string, error) {
				if !a.bool("tree") && a.int("depth") == 0 {
					return []string{"list", "--json"}, nil
				}
				out := []string{"list", "--tree"}
				if d := a.int("depth"); d > 0 {
					out = append(out, "--depth", strconv.Itoa(d))
				}
				return out, nil
			}, parse: parseJSONOutput("packages")}),
		read(&tool{name: "why", title: "Why a package",
			description: "Which requirements of gtr.json bring a package in.",
			props:       map[string]any{"package": prop("string", "Package name.")}, required: []string{"package"},
			args: func(a args) ([]string, error) { return []string{"why", a.str("package")}, nil }}),
		read(&tool{name: "outdated", title: "Outdated packages",
			description: "Packages with newer versions, as JSON (current, wanted by the range, latest). all: indirect ones too.",
			props:       map[string]any{"all": prop("boolean", "Include indirect dependencies.")},
			openWorld:   true,
			args: func(a args) ([]string, error) {
				return flags([]string{"outdated", "--json"}, a, "all", "--all"), nil
			}, parse: parseJSONOutput("packages")}),
		read(&tool{name: "search", title: "Search the registry",
			description: "Search packages in the gtr registry (GTR_REGISTRY).",
			props:       map[string]any{"query": prop("string", "Words to look for.")}, required: []string{"query"},
			openWorld: true,
			args:      func(a args) ([]string, error) { return []string{"search", a.str("query")}, nil }}),
		read(&tool{name: "info", title: "Package info",
			description: "A package in the gtr registry: description, owners, versions (yanked ones marked).",
			props:       map[string]any{"package": prop("string", "Package name.")}, required: []string{"package"},
			openWorld: true,
			args:      func(a args) ([]string, error) { return []string{"info", a.str("package")}, nil }}),
		read(&tool{name: "env", title: "gtr env",
			description: "The Go and gtr-manager versions and paths gtr selects in this directory, and why (installs nothing).",
			args:        func(a args) ([]string, error) { return []string{"env", "--json"}, nil }, parse: parseJSONOutput("env")}),
		read(&tool{name: "doctor", title: "gtr doctor",
			description: "Check the gtr installation, PATH, the project and the network.",
			props:       map[string]any{"offline": prop("boolean", "Skip the network checks.")},
			args: func(a args) ([]string, error) {
				return flags([]string{"doctor"}, a, "offline", "--offline"), nil
			}}),

		change(&tool{name: "add", title: "Add packages",
			description: "Add packages to gtr.json and install them. Sources: a registry name (faker, faker@^0.2), github:owner/repo[#range], gitlab:, git+https://, file:../path, npm:name, or a workspace member. Use this instead of go get.",
			props: map[string]any{"packages": strList("Packages to add."), "dev": prop("boolean", "As devDependencies."),
				"offline": prop("boolean", "Only what the local store has.")},
			required: []string{"packages"}, openWorld: true,
			args: func(a args) ([]string, error) {
				p := a.strs("packages")
				if len(p) == 0 {
					return nil, errors.New("packages: at least one")
				}
				return flags(flags(append([]string{"add"}, p...), a, "dev", "-D"), a, "offline", "--offline"), nil
			}}),
		change(&tool{name: "remove", title: "Remove packages",
			description: "Remove packages from gtr.json, gtr.lock and gtr_modules.",
			props:       map[string]any{"packages": strList("Package names.")}, required: []string{"packages"},
			destructive: true,
			args: func(a args) ([]string, error) {
				p := a.strs("packages")
				if len(p) == 0 {
					return nil, errors.New("packages: at least one")
				}
				return append([]string{"remove"}, p...), nil
			}}),
		change(&tool{name: "install", title: "Install",
			description: "Install what gtr.json asks for (keeping gtr.lock) and regenerate go.mod. ci: install exactly gtr.lock and fail if it does not match gtr.json.",
			props:       map[string]any{"ci": prop("boolean", "Strict install from gtr.lock (gtr ci)."), "offline": prop("boolean", "Only what the local store has.")},
			idempotent:  true, openWorld: true,
			args: func(a args) ([]string, error) {
				cmd := "install"
				if a.bool("ci") {
					cmd = "ci"
				}
				return flags([]string{cmd}, a, "offline", "--offline"), nil
			}}),
		change(&tool{name: "update", title: "Update packages",
			description: "Move packages (all, or the given ones) to the newest versions their ranges in gtr.json allow; gtr.lock changes.",
			props:       map[string]any{"packages": strList("Package names (default: all).")},
			openWorld:   true,
			args:        func(a args) ([]string, error) { return append([]string{"update"}, a.strs("packages")...), nil }}),
		change(&tool{name: "sync", title: "Sync go.mod",
			description: "Carry go.mod edits over to gtr.json. force: regenerate go.mod from gtr.json, discarding edits.",
			props:       map[string]any{"force": prop("boolean", "Regenerate go.mod.")},
			args: func(a args) ([]string, error) {
				return flags([]string{"sync"}, a, "force", "--force"), nil
			}}),
		change(&tool{name: "link", title: "Link a local copy",
			description: "Without target, in a package: register it for linking. With target (a name or a path), in a project: use that local copy of the package (not written to gtr.lock).",
			props:       map[string]any{"target": prop("string", "Package name or path.")},
			args: func(a args) ([]string, error) {
				if t := a.str("target"); t != "" {
					return []string{"link", t}, nil
				}
				return []string{"link"}, nil
			}}),
		change(&tool{name: "unlink", title: "Unlink",
			description: "Stop using local copies (one package or all). global: unregister this package.",
			props:       map[string]any{"package": prop("string", "Package name (default: all)."), "global": prop("boolean", "Unregister the package itself.")},
			args: func(a args) ([]string, error) {
				out := []string{"unlink"}
				if p := a.str("package"); p != "" {
					out = append(out, p)
				}
				return flags(out, a, "global", "--global"), nil
			}}),
		change(&tool{name: "version_bump", title: "Bump the version",
			description: "Bump the package version in gtr.json and CHANGELOG.md, commit and tag it (no_git: files only; no_tag: commit without a tag).",
			props: map[string]any{"bump": prop("string", "patch, minor, major, premajor, preminor, prepatch, prerelease, or a version such as 1.2.0."),
				"preid": prop("string", "Prerelease id (rc, beta)."), "no_git": prop("boolean", "Do not commit or tag."),
				"no_tag": prop("boolean", "Commit without a tag.")},
			required: []string{"bump"},
			args: func(a args) ([]string, error) {
				out := []string{"version", a.str("bump")}
				if p := a.str("preid"); p != "" {
					out = append(out, "--preid", p)
				}
				return flags(flags(out, a, "no_git", "--no-git"), a, "no_tag", "--no-tag"), nil
			}}),

		{name: "run", title: "Run a script",
			description: "Run a script from gtr.json (with its pre/post scripts and .env). Long-running scripts such as dev or start servers stop at the time limit.",
			props: map[string]any{"script": prop("string", "Script name."), "args": strList("Arguments after --."),
				"no_env": prop("boolean", "Without the .env files.")},
			required: []string{"script"}, timeout: longTimeout,
			args: func(a args) ([]string, error) {
				out := flags([]string{"run"}, a, "no_env", "--no-env")
				out = append(out, a.str("script"))
				if extra := a.strs("args"); len(extra) > 0 {
					out = append(append(out, "--"), extra...)
				}
				return out, nil
			}},
		{name: "test", title: "Run tests",
			description: "go test ./... with the project's Go and build tags; results per package as JSON. kind: unit (default), integration or e2e. coverage: per-package coverage, checked against test.coverage of gtr.json.",
			props: map[string]any{"kind": enum("Which tests.", "unit", "integration", "e2e"),
				"coverage": prop("boolean", "Measure coverage."), "run": prop("string", "Only tests matching this regular expression (-run)."),
				"recursive": prop("boolean", "Every workspace member."), "verbose": prop("boolean", "go test -v."),
				"short": prop("boolean", "-short."), "race": prop("boolean", "Force the race detector on (true) or off (false).")},
			timeout: longTimeout,
			args: func(a args) ([]string, error) {
				out := []string{"test"}
				switch k := a.str("kind"); k {
				case "", "unit":
				case "integration", "e2e":
					out = append(out, "--"+k)
				default:
					return nil, fmt.Errorf("kind: %q is not unit, integration or e2e", k)
				}
				if r := a.str("run"); r != "" {
					out = append(out, "--run", r)
				}
				if v, ok := a["race"].(bool); ok {
					if v {
						out = append(out, "--race")
					} else {
						out = append(out, "--no-race")
					}
				}
				out = flags(out, a, "coverage", "--coverage")
				out = flags(out, a, "recursive", "-r")
				out = flags(out, a, "verbose", "-v")
				return flags(out, a, "short", "--short"), nil
			}, parse: parseTest},
		{name: "build", title: "Build",
			description: "Build by the build block of gtr.json into build/<version>/<os>-<arch>/. targets: os/arch pairs; all_targets: every target; archive: release archives and SHA256SUMS.",
			props: map[string]any{"targets": strList("Targets such as linux/amd64."), "all_targets": prop("boolean", "Every target."),
				"archive": prop("boolean", "Write archives.")},
			timeout: longTimeout, idempotent: true,
			args: func(a args) ([]string, error) {
				out := []string{"build"}
				for _, t := range a.strs("targets") {
					out = append(out, "-t", t)
				}
				return flags(flags(out, a, "all_targets", "--all-targets"), a, "archive", "--archive"), nil
			}},
		{name: "publish", title: "Publish",
			description: "Publish the package to the gtr registry (or push the version tag with git). Only a dry run unless the user allowed publishing in the server's settings (gtr mcp --allow-publish).",
			props: map[string]any{"dry_run": prop("boolean", "Check and list the archive only (default true)."),
				"skip_tests": prop("boolean", "Do not run the tests first."), "allow_dirty": prop("boolean", "Publish uncommitted changes."),
				"git": prop("boolean", "Publish by pushing the git tag instead of the registry.")},
			timeout: longTimeout, openWorld: true, destructive: true,
			args: func(a args) ([]string, error) {
				dry := true
				if v, ok := a["dry_run"].(bool); ok {
					dry = v
				}
				if !dry && !s.o.AllowPublish {
					return nil, errors.New("publishing is not allowed: the user starts the server with --allow-publish (the allow_publish setting of the extension); a dry run is allowed")
				}
				out := []string{"publish"}
				if dry {
					out = append(out, "--dry-run")
				}
				out = flags(out, a, "skip_tests", "--skip-tests")
				out = flags(out, a, "allow_dirty", "--allow-dirty")
				return flags(out, a, "git", "--git"), nil
			}},
	}
	if s.o.AllowSelfInstall {
		tools = append(tools, &tool{name: "self_install", title: "Install Go or gtr-manager",
			description: "Install a Go or gtr-manager version into the gtr home (without a version: what gtr.json requires).",
			props:       map[string]any{"tool": enum("What to install.", "go", "gtr"), "version": prop("string", "Version or range (1.26, ^0.2).")},
			required:    []string{"tool"}, timeout: longTimeout, openWorld: true, idempotent: true,
			args: func(a args) ([]string, error) {
				t := a.str("tool")
				if t != "go" && t != "gtr" {
					return nil, fmt.Errorf("tool: %q is not go or gtr", t)
				}
				out := []string{"self", "install", t}
				if v := a.str("version"); v != "" {
					out = append(out, v)
				}
				return out, nil
			}})
	}
	return tools
}

// args are a tool call's arguments.
type args map[string]any

func (a args) str(k string) string {
	s, _ := a[k].(string)
	return strings.TrimSpace(s)
}

func (a args) bool(k string) bool { b, _ := a[k].(bool); return b }

func (a args) int(k string) int {
	switch v := a[k].(type) {
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

func (a args) strs(k string) []string {
	var out []string
	switch v := a[k].(type) {
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	case string: // a model that sent one value as a string
		out = strings.Fields(v)
	}
	return out
}

// flags appends flag when the boolean argument k is true.
func flags(out []string, a args, k, flag string) []string {
	if a.bool(k) {
		out = append(out, flag)
	}
	return out
}

func (s *Server) callTool(ctx context.Context, req *request) (map[string]any, error) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(req.params, &p); err != nil {
		return nil, errorf(codeInvalidParams, "tools/call: %v", err)
	}
	var t *tool
	for _, x := range s.tools {
		if x.name == p.Name {
			t = x
		}
	}
	if t == nil {
		return nil, errorf(codeInvalidParams, "unknown tool %q", p.Name)
	}
	a := args(p.Arguments)
	for k := range a {
		if _, ok := t.props[k]; !ok && k != "dir" && k != "timeout_seconds" {
			return toolError(fmt.Sprintf("unknown argument %q of %s", k, t.name)), nil
		}
	}
	for _, k := range t.required {
		if v, ok := a[k]; !ok || v == nil || v == "" {
			return toolError(fmt.Sprintf("%s needs the argument %q", t.name, k)), nil
		}
	}
	dir, err := s.dir(a.str("dir"))
	if err != nil {
		return toolError(err.Error()), nil
	}
	if t.call != nil {
		out, err := t.call(ctx, s, a, dir)
		if err != nil {
			return toolError(err.Error()), nil
		}
		text, _ := json.MarshalIndent(out, "", "  ")
		return map[string]any{"content": []any{textContent(string(text))}, "structuredContent": out, "isError": false}, nil
	}
	argv, err := t.args(a)
	if err != nil {
		return toolError(err.Error()), nil
	}
	timeout := t.timeout
	if n := a.int("timeout_seconds"); n > 0 {
		timeout = time.Duration(n) * time.Second
		if timeout > maxTimeout {
			timeout = maxTimeout
		}
	}
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	res, err := s.o.Runner.Run(rctx, dir, argv, s.progress(req))
	if err != nil {
		return toolError(fmt.Sprintf("gtr %s: %v", strings.Join(argv, " "), err)), nil
	}
	cancelled := ctx.Err() != nil
	timedOut := !cancelled && errors.Is(rctx.Err(), context.DeadlineExceeded)
	rel, _ := filepath.Rel(s.root, dir)
	out := map[string]any{
		"command":   "gtr " + strings.Join(argv, " "),
		"dir":       filepath.ToSlash(rel),
		"exit_code": res.ExitCode,
		"output":    res.Output,
		"truncated": res.Truncated,
		"timed_out": timedOut,
	}
	if t.parse != nil {
		t.parse(res.Output, out)
	}
	text := res.Output
	switch {
	case timedOut:
		text += fmt.Sprintf("\n[gtr mcp: stopped after the time limit of %s; pass timeout_seconds for more]", timeout)
	case cancelled:
		text += "\n[gtr mcp: cancelled]"
	case res.ExitCode != 0:
		text += fmt.Sprintf("\n[exit code %d]", res.ExitCode)
	}
	if strings.TrimSpace(text) == "" {
		text = "(no output)"
	}
	return map[string]any{
		"content":           []any{textContent(text)},
		"structuredContent": out,
		"isError":           res.ExitCode != 0 || timedOut || cancelled,
	}, nil
}

func textContent(s string) map[string]any { return map[string]any{"type": "text", "text": s} }

// toolError is a failed call the model can read and correct (not a
// protocol error).
func toolError(msg string) map[string]any {
	return map[string]any{"content": []any{textContent(msg)}, "isError": true}
}

// dir resolves a tool's dir argument inside the project root.
func (s *Server) dir(rel string) (string, error) {
	if rel == "" || rel == "." {
		return s.root, nil
	}
	if filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" {
		return "", fmt.Errorf("dir %q: give a path relative to the project root %s", rel, s.root)
	}
	p := filepath.Join(s.root, filepath.FromSlash(rel))
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	} else {
		return "", fmt.Errorf("dir %q: %v", rel, err)
	}
	if !within(s.root, p) {
		return "", fmt.Errorf("dir %q is outside the project root %s", rel, s.root)
	}
	if st, err := os.Stat(p); err != nil || !st.IsDir() {
		return "", fmt.Errorf("dir %q is not a directory", rel)
	}
	return p, nil
}

func within(root, p string) bool {
	r, err := filepath.Rel(root, p)
	return err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) && !filepath.IsAbs(r)
}

// progress returns the callback that streams output lines to the client as
// progress notifications, when the request asked for them (at most a few
// per second).
func (s *Server) progress(req *request) func(string) {
	if req.progress == nil {
		return nil
	}
	n, last := 0, time.Time{}
	return func(line string) {
		n++
		if time.Since(last) < 250*time.Millisecond {
			return
		}
		last = time.Now()
		if len(line) > 200 {
			line = line[:200] + "…"
		}
		s.send(message{Method: "notifications/progress", Params: mustJSON(map[string]any{
			"progressToken": req.progress, "progress": n, "message": line})})
	}
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// parseJSONOutput puts the JSON document the command printed under key.
func parseJSONOutput(key string) func(string, map[string]any) {
	return func(out string, into map[string]any) {
		start := strings.IndexAny(out, "[{")
		if start < 0 {
			return
		}
		var v any
		dec := json.NewDecoder(strings.NewReader(out[start:]))
		if dec.Decode(&v) == nil {
			into[key] = v
		}
	}
}

var (
	goTestLine   = regexp.MustCompile(`^(ok|FAIL|\?)\s+(\S+)\s+(?:(\d+(?:\.\d+)?s|\(cached\))|\[([^\]]+)\])?(?:\s+coverage: ([\d.]+)% of statements)?`)
	coverageLine = regexp.MustCompile(`^coverage\s+(\S+)\s+([\d.]+)%(?:\s+\(min ([\d.]+)%, (ok|BELOW)\))?`)
	failedTest   = regexp.MustCompile(`^\s*--- FAIL: (\S+)`)
)

// parseTest reads go test's summary lines and gtr's coverage table.
func parseTest(out string, into map[string]any) {
	var pkgs, cover, failed []any
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := goTestLine.FindStringSubmatch(line); m != nil {
			status := map[string]string{"ok": "pass", "FAIL": "fail", "?": "no tests"}[m[1]]
			p := map[string]any{"package": m[2], "status": status}
			if m[3] != "" {
				p["time"] = m[3]
			}
			if m[4] != "" {
				p["note"] = m[4]
			}
			if m[5] != "" {
				f, _ := strconv.ParseFloat(m[5], 64)
				p["coverage"] = f
			}
			pkgs = append(pkgs, p)
		} else if m := coverageLine.FindStringSubmatch(line); m != nil {
			f, _ := strconv.ParseFloat(m[2], 64)
			c := map[string]any{"package": m[1], "percent": f}
			if m[3] != "" {
				min, _ := strconv.ParseFloat(m[3], 64)
				c["min"], c["ok"] = min, m[4] == "ok"
			}
			cover = append(cover, c)
		} else if m := failedTest.FindStringSubmatch(line); m != nil {
			failed = append(failed, m[1])
		}
	}
	if pkgs != nil {
		into["packages"] = pkgs
	}
	if cover != nil {
		into["coverage"] = cover
	}
	if failed != nil {
		into["failed_tests"] = failed
	}
}
