package cli

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gtr/internal/mcp"
)

const mcpUsage = "mcp [--root <dir>] [--allow-publish] [--allow-self-install] | mcp --print-config | mcp bundle [-o file.mcpb]"

// mcp runs `gtr mcp`: the MCP server on stdin/stdout (ADR-0011).
func (a *App) mcp(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "bundle" {
		return a.mcpBundle(args[1:])
	}
	var root string
	var printConfig bool
	o := mcp.Options{Version: a.Version, Log: a.Err}
	for i := 0; i < len(args); i++ {
		name, value, hasValue := strings.Cut(args[i], "=")
		switch name {
		case "--root":
			if !hasValue {
				if i+1 >= len(args) {
					return fmt.Errorf("%w: --root needs a directory; %s", ErrUsage, mcpUsage)
				}
				i++
				value = args[i]
			}
			root = value
		case "--allow-publish":
			o.AllowPublish = flagValue(value, hasValue)
		case "--allow-self-install":
			o.AllowSelfInstall = flagValue(value, hasValue)
		case "--print-config":
			printConfig = true
		case "-h", "--help":
			fmt.Fprintf(a.Out, "Usage: gtr %s\n", mcpUsage)
			return nil
		default:
			return fmt.Errorf("%w: unknown flag %s; %s", ErrUsage, args[i], mcpUsage)
		}
	}
	dir, err := a.mcpRoot(root)
	if err != nil {
		return err
	}
	exe, err := a.Executable()
	if err != nil {
		return err
	}
	if printConfig {
		return a.printMCPConfig(exe, dir)
	}
	o.Root = dir
	o.Runner = mcp.ExecRunner{Exe: exe, Env: a.Environ()}
	s, err := mcp.New(o)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Err, "gtr mcp %s: serving %s on stdio\n", a.Version, s.Root())
	in := a.In
	if in == nil {
		in = os.Stdin
	}
	if err := s.Serve(ctx, in, a.Out); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

// flagValue reads --flag or --flag=true|false; a value the client did not
// substitute (${user_config.x}) is false.
func flagValue(value string, hasValue bool) bool {
	if !hasValue {
		return true
	}
	b, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && b
}

// mcpRoot is the project the server works in: --root, else
// CLAUDE_PROJECT_DIR, else the working directory; from there, the
// workspace root or the nearest gtr.json above it.
func (a *App) mcpRoot(root string) (string, error) {
	if strings.Contains(root, "${") { // a placeholder the client left as it was
		root = ""
	}
	if root == "" {
		root = a.Getenv("CLAUDE_PROJECT_DIR")
	}
	if root == "" {
		wd, err := a.Getwd()
		if err != nil {
			return "", err
		}
		root = wd
	}
	dir, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return "", fmt.Errorf("gtr mcp: %s is not a directory", root)
	}
	found := ""
	for d := dir; ; d = filepath.Dir(d) {
		data, err := os.ReadFile(filepath.Join(d, "gtr.json"))
		if err == nil {
			if found == "" {
				found = d
			}
			var m struct {
				Workspaces []string `json:"workspaces"`
			}
			if json.Unmarshal(data, &m) == nil && len(m.Workspaces) > 0 {
				found = d // a workspace root above the member
			}
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	if found != "" {
		return found, nil
	}
	return dir, nil
}

// printMCPConfig prints the client configuration for this gtr and project.
func (a *App) printMCPConfig(exe, dir string) error {
	cfg := map[string]any{"mcpServers": map[string]any{"gtr": map[string]any{
		"type": "stdio", "command": exe, "args": []string{"mcp", "--root", dir}}}}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Fprintln(a.Out, string(data))
	fmt.Fprintf(a.Err, `
This goes into .mcp.json of the project (Claude Code) or claude_desktop_config.json
(Claude Desktop). For Claude Code alone: claude mcp add gtr -- gtr mcp
Allow publishing with "--allow-publish" in args; gtr mcp bundle makes a Claude Desktop extension.
`)
	return nil
}

// mcpBundle writes a Claude Desktop extension (.mcpb): this gtr binary and
// a manifest whose settings are the project folder and allow_publish.
func (a *App) mcpBundle(args []string) error {
	out := "gtr.mcpb"
	for i := 0; i < len(args); i++ {
		switch {
		case (args[i] == "-o" || args[i] == "--output") && i+1 < len(args):
			out = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--output="):
			out = strings.TrimPrefix(args[i], "--output=")
		default:
			return fmt.Errorf("%w: %s", ErrUsage, mcpUsage)
		}
	}
	exe, err := a.Executable()
	if err != nil {
		return err
	}
	bin, err := os.Open(exe)
	if err != nil {
		return err
	}
	defer bin.Close()
	entry := "server/" + a.exe("gtr")
	version := a.Version
	if version == "" || version == "dev" || version[0] < '0' || version[0] > '9' {
		version = "0.0.0-dev"
	}
	platform := map[string]string{"windows": "win32"}[a.GOOS]
	if platform == "" {
		platform = a.GOOS
	}
	manifest := map[string]any{
		"manifest_version": "0.3",
		"name":             "gtr",
		"display_name":     "gtr — Go project manager",
		"version":          version,
		"description":      "Install packages, run scripts and tests and read gtr.json in a Go project managed by gtr.",
		"long_description": "gtr is a package manager and launcher for Go. This extension lets Claude work with one gtr project: list, add, remove and update packages, install, run scripts, run tests with coverage and build, with the project's Go version and gtr's error hints. Publishing is only a dry run unless allowed in the settings.",
		"author":           map[string]any{"name": "shibisty", "url": "https://github.com/shibisty"},
		"repository":       map[string]any{"type": "git", "url": "https://github.com/shibisty/gtr"},
		"homepage":         "https://github.com/shibisty/gtr",
		"license":          "MIT",
		"keywords":         []string{"go", "golang", "package manager", "gtr"},
		"server": map[string]any{
			"type":        "binary",
			"entry_point": entry,
			"mcp_config": map[string]any{
				"command": "${__dirname}/" + entry,
				"args":    []string{"mcp", "--root", "${user_config.project}", "--allow-publish=${user_config.allow_publish}"},
			},
		},
		"tools": bundleTools(),
		"user_config": map[string]any{
			"project": map[string]any{"type": "directory", "title": "Project folder", "required": true,
				"description": "The gtr project (the folder with gtr.json, or the workspace root) Claude works in."},
			"allow_publish": map[string]any{"type": "boolean", "title": "Allow publishing", "default": false,
				"description": "Let Claude publish packages (otherwise publish is only a dry run)."},
		},
		"compatibility": map[string]any{"platforms": []string{platform}},
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	mdata, _ := json.MarshalIndent(manifest, "", "  ")
	now := time.Now()
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "manifest.json", Method: zip.Deflate, Modified: now})
	if err == nil {
		_, err = w.Write(append(mdata, '\n'))
	}
	if err == nil {
		h := &zip.FileHeader{Name: entry, Method: zip.Deflate, Modified: now}
		h.SetMode(0o755)
		if w, err = zw.CreateHeader(h); err == nil {
			_, err = io.Copy(w, bin)
		}
	}
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(out)
		return err
	}
	fmt.Fprintf(a.Out, "wrote %s (gtr %s for %s/%s): open it with Claude Desktop to install the extension\n",
		out, version, a.GOOS, runtime.GOARCH)
	return nil
}

// bundleTools lists the tools for the extension's manifest.
func bundleTools() []map[string]string {
	s, err := mcp.New(mcp.Options{Root: os.TempDir(), Runner: mcp.ExecRunner{}})
	if err != nil {
		return nil
	}
	var out []map[string]string
	for _, t := range s.ToolNames() {
		out = append(out, map[string]string{"name": t[0], "description": t[1]})
	}
	return out
}
