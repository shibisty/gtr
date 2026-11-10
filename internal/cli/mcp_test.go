package cli

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPRoot(t *testing.T) {
	h := newHarness(t)
	ws := t.TempDir()
	os.WriteFile(filepath.Join(ws, "gtr.json"), []byte(`{"name": "ws", "workspaces": ["core"]}`), 0o644)
	os.MkdirAll(filepath.Join(ws, "core", "sub"), 0o755)
	os.WriteFile(filepath.Join(ws, "core", "gtr.json"), []byte(`{"name": "core"}`), 0o644)
	plain := t.TempDir()
	os.WriteFile(filepath.Join(plain, "gtr.json"), []byte(`{"name": "app"}`), 0o644)
	os.MkdirAll(filepath.Join(plain, "x"), 0o755)
	empty := t.TempDir()

	for _, tc := range []struct{ root, env, cwd, want string }{
		{filepath.Join(ws, "core", "sub"), "", "", ws}, // a member: the workspace root
		{filepath.Join(plain, "x"), "", "", plain},
		{"${CLAUDE_PROJECT_DIR}", filepath.Join(plain, "x"), "", plain}, // not substituted: the variable
		{"", "", empty, empty},                                          // no gtr.json: the directory
	} {
		h.env["CLAUDE_PROJECT_DIR"] = tc.env
		if tc.cwd != "" {
			h.cwd = tc.cwd
		}
		got, err := h.app.mcpRoot(tc.root)
		if err != nil || got != tc.want {
			t.Errorf("mcpRoot(%q): %q, %v; want %q", tc.root, got, err, tc.want)
		}
	}
	if _, err := h.app.mcpRoot(filepath.Join(empty, "missing")); err == nil {
		t.Fatal("a missing root")
	}
	for v, want := range map[string]bool{"": true, "true": true, "1": true, "false": false, "${user_config.allow_publish}": false} {
		if got := flagValue(v, v != ""); got != want {
			t.Errorf("flagValue(%q) = %v", v, got)
		}
	}
}

func TestMCPCommand(t *testing.T) {
	h := newHarness(t)
	h.cwd = t.TempDir()
	if err := h.run("mcp", "--print-config"); err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(h.stdout()), &cfg); err != nil {
		t.Fatal(err, h.stdout())
	}
	if s := cfg.MCPServers["gtr"]; s.Command != h.exe || strings.Join(s.Args, " ") != "mcp --root "+h.cwd {
		t.Fatal(s)
	}
	if !strings.Contains(h.stderr(), "claude mcp add gtr -- gtr mcp") {
		t.Fatal(h.stderr())
	}
	if err := h.run("mcp", "--bogus"); !errors.Is(err, ErrUsage) {
		t.Fatal(err)
	}

	// serving: stdin ends, the server stops
	h.sout.Reset()
	h.app.In = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}` + "\n")
	if err := h.run("mcp", "--allow-publish"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.stdout(), `"protocolVersion":"2025-11-25"`) || !strings.Contains(h.stderr(), "serving") {
		t.Fatal(h.stdout(), h.stderr())
	}
}

func TestMCPBundle(t *testing.T) {
	h := newHarness(t)
	out := filepath.Join(t.TempDir(), "gtr.mcpb")
	if err := h.run("mcp", "bundle", "-o", out); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	files := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(data)
	}
	if files["server/gtr"] != "gtr-old" {
		t.Fatal("the binary is not in the bundle", files)
	}
	var m struct {
		Version string `json:"version"`
		Server  struct {
			EntryPoint string `json:"entry_point"`
			MCPConfig  struct {
				Command string   `json:"command"`
				Args    []string `json:"args"`
			} `json:"mcp_config"`
		} `json:"server"`
		Tools      []map[string]string `json:"tools"`
		UserConfig map[string]any      `json:"user_config"`
	}
	if err := json.Unmarshal([]byte(files["manifest.json"]), &m); err != nil {
		t.Fatal(err)
	}
	if m.Version != "0.1.0" || m.Server.EntryPoint != "server/gtr" || m.Server.MCPConfig.Command != "${__dirname}/server/gtr" ||
		strings.Join(m.Server.MCPConfig.Args, " ") != "mcp --root ${user_config.project} --allow-publish=${user_config.allow_publish}" ||
		len(m.Tools) < 10 || m.UserConfig["project"] == nil {
		t.Fatalf("%+v", m)
	}
}
