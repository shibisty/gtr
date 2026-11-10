package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeManager is a gtr-manager that logs its arguments and directory and
// answers list --json and test like the real one.
const fakeManager = `package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	wd, _ := os.Getwd()
	f, _ := os.OpenFile(os.Getenv("FAKE_MANAGER_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	fmt.Fprintf(f, "%s|%s|%s\n", wd, strings.Join(os.Args[1:], " "), os.Getenv("GTR_MCP"))
	f.Close()
	switch os.Args[1] {
	case "list":
		fmt.Println(` + "`" + `[{"name": "faker", "version": "0.1.0"}]` + "`" + `)
	case "test":
		fmt.Println("ok  \tapp\t0.010s\tcoverage: 90.0% of statements")
	case "add":
		fmt.Fprintln(os.Stderr, "gtr: no package faker2 in the registry")
		os.Exit(1)
	default:
		fmt.Println("done", os.Args[1])
	}
}
`

// TestMCPE2E starts the built gtr as `gtr mcp` in a project, as Claude
// Code would, and calls tools through the launcher into gtr-manager (a
// fake one, linked with gtr self link).
func TestMCPE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("end-to-end test")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not in PATH")
	}
	tmp := t.TempDir()
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bin := filepath.Join(tmp, "gtr"+exe)
	if out, err := exec.Command(goBin, "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	src := filepath.Join(tmp, "fake")
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "main.go"), []byte(fakeManager), 0o644)
	os.WriteFile(filepath.Join(src, "go.mod"), []byte("module fake\n\ngo 1.22\n"), 0o644)
	manager := filepath.Join(tmp, "gtr-manager"+exe)
	cmd := exec.Command(goBin, "build", "-o", manager, ".")
	cmd.Dir = src
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build the fake gtr-manager: %v\n%s", err, out)
	}

	gtrHome := filepath.Join(tmp, "gtrhome")
	os.MkdirAll(gtrHome, 0o755)
	cfg, _ := json.Marshal(map[string]any{"links": map[string]string{"gtr": manager}})
	os.WriteFile(filepath.Join(gtrHome, "config.json"), cfg, 0o644)
	project := filepath.Join(tmp, "app")
	os.MkdirAll(filepath.Join(project, "sub"), 0o755)
	os.WriteFile(filepath.Join(project, "gtr.json"), []byte(`{"name": "app", "version": "0.1.0"}`), 0o644)
	logFile := filepath.Join(tmp, "manager.log")

	srv := exec.Command(bin, "mcp")
	srv.Dir = filepath.Join(project, "sub") // the root is found above
	srv.Env = []string{"PATH=" + os.Getenv("PATH"), "SYSTEMROOT=" + os.Getenv("SYSTEMROOT"), "HOME=" + tmp, "USERPROFILE=" + tmp,
		"GTR_HOME=" + gtrHome, "FAKE_MANAGER_LOG=" + logFile, "GTR_GITHUB_API=http://127.0.0.1:1"}
	stdin, _ := srv.StdinPipe()
	stdout, _ := srv.StdoutPipe()
	var stderr strings.Builder
	srv.Stderr = &stderr
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Process.Kill()
	lines := bufio.NewScanner(stdout)
	lines.Buffer(make([]byte, 1<<20), 1<<20)
	id := 0
	rpc := func(method string, params any) map[string]any {
		t.Helper()
		id++
		data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		io.WriteString(stdin, string(data)+"\n")
		done := make(chan map[string]any, 1)
		go func() {
			for lines.Scan() {
				var m map[string]any
				json.Unmarshal(lines.Bytes(), &m)
				if m["id"] == float64(id) {
					done <- m
					return
				}
			}
			done <- nil
		}()
		select {
		case m := <-done:
			if m == nil || m["result"] == nil {
				t.Fatalf("%s: %v\nstderr: %s", method, m, stderr.String())
			}
			return m["result"].(map[string]any)
		case <-time.After(30 * time.Second):
			t.Fatalf("%s: no answer\nstderr: %s", method, stderr.String())
		}
		return nil
	}
	if r := rpc("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "e2e", "version": "1"}}); r["protocolVersion"] != "2025-06-18" {
		t.Fatal(r)
	}
	io.WriteString(stdin, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n")
	tool := func(name string, args map[string]any) map[string]any {
		t.Helper()
		return rpc("tools/call", map[string]any{"name": name, "arguments": args})
	}
	r := tool("list", nil)
	sc := r["structuredContent"].(map[string]any)
	if r["isError"] != false || sc["packages"].([]any)[0].(map[string]any)["name"] != "faker" {
		t.Fatal(r)
	}
	r = tool("install", nil)
	if r["isError"] != false || !strings.Contains(r["structuredContent"].(map[string]any)["output"].(string), "done install") {
		t.Fatal(r)
	}
	r = tool("add", map[string]any{"packages": []any{"faker2"}})
	if r["isError"] != true || !strings.Contains(r["content"].([]any)[0].(map[string]any)["text"].(string), "no package faker2") {
		t.Fatal(r)
	}
	r = tool("test", map[string]any{"coverage": true})
	if p := r["structuredContent"].(map[string]any)["packages"].([]any); p[0].(map[string]any)["coverage"] != 90.0 {
		t.Fatal(r)
	}
	r = tool("project_info", nil)
	if r["structuredContent"].(map[string]any)["gtr_json"].(map[string]any)["name"] != "app" {
		t.Fatal(r)
	}
	stdin.Close()
	if err := srv.Wait(); err != nil {
		t.Fatalf("gtr mcp: %v\n%s", err, stderr.String())
	}

	log, _ := os.ReadFile(logFile)
	root, _ := filepath.EvalSymlinks(project)
	want := []string{"list --json", "install", "add faker2", "test --coverage"}
	got := strings.Split(strings.TrimSpace(string(log)), "\n")
	if len(got) != len(want) {
		t.Fatalf("gtr-manager ran:\n%s", log)
	}
	for i, line := range got {
		parts := strings.Split(line, "|")
		dir, _ := filepath.EvalSymlinks(parts[0])
		if dir != root || parts[1] != want[i] || parts[2] != "1" {
			t.Fatalf("call %d: %q, want %s in %s with GTR_MCP=1", i, line, want[i], root)
		}
	}
}
