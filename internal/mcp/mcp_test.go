package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRunner records commands and answers with canned output.
type fakeRunner struct {
	mu    sync.Mutex
	calls []call
	reply func(dir string, args []string) Result
	block chan struct{} // when set, Run waits for it or ctx
}

type call struct {
	dir  string
	args []string
}

func (f *fakeRunner) Run(ctx context.Context, dir string, args []string, line func(string)) (Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, call{dir, args})
	f.mu.Unlock()
	if f.block != nil {
		if line != nil {
			line("started")
		}
		select {
		case <-f.block:
		case <-ctx.Done():
			return Result{Output: "stopped", ExitCode: -1}, nil
		}
	}
	if f.reply != nil {
		return f.reply(dir, args), nil
	}
	return Result{Output: "ok\n"}, nil
}

func (f *fakeRunner) last() call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[len(f.calls)-1]
}

// client talks to a server over pipes.
type client struct {
	t    *testing.T
	in   *io.PipeWriter
	out  *bufio.Scanner
	done chan error
	mu   sync.Mutex
	got  []map[string]any // messages read, not yet claimed
}

func start(t *testing.T, o Options) (*client, *Server) {
	t.Helper()
	if o.Root == "" {
		o.Root = t.TempDir()
	}
	s, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := &client{t: t, in: inW, out: bufio.NewScanner(outR), done: make(chan error, 1)}
	go func() { c.done <- s.Serve(context.Background(), inR, outW); outW.Close() }()
	t.Cleanup(func() { inW.Close() })
	return c, s
}

func (c *client) send(v any) {
	c.t.Helper()
	data, _ := json.Marshal(v)
	if _, err := c.in.Write(append(data, '\n')); err != nil {
		c.t.Fatal(err)
	}
}

// next reads the next message.
func (c *client) next() map[string]any {
	c.t.Helper()
	ch := make(chan map[string]any, 1)
	go func() {
		if !c.out.Scan() {
			ch <- nil
			return
		}
		var m map[string]any
		json.Unmarshal(c.out.Bytes(), &m)
		ch <- m
	}()
	select {
	case m := <-ch:
		if m == nil {
			c.t.Fatal("server closed its output")
		}
		return m
	case <-time.After(5 * time.Second):
		c.t.Fatal("no message from the server")
	}
	return nil
}

// response reads messages until the response with id.
func (c *client) response(id float64) map[string]any {
	c.t.Helper()
	for i, m := range c.got {
		if m["id"] == id {
			c.got = append(c.got[:i], c.got[i+1:]...)
			return m
		}
	}
	for {
		m := c.next()
		if m["id"] == id {
			return m
		}
		c.got = append(c.got, m)
	}
}

func (c *client) call(id float64, method string, params any) map[string]any {
	c.t.Helper()
	c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	return c.response(id)
}

func result(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	r, ok := m["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", m)
	}
	return r
}

func TestLegacyHandshake(t *testing.T) {
	c, _ := start(t, Options{Version: "1.2.3", Runner: &fakeRunner{}})
	r := result(t, c.call(1, "initialize", map[string]any{"protocolVersion": "2025-06-18",
		"capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"}}))
	if r["protocolVersion"] != "2025-06-18" || r["serverInfo"].(map[string]any)["version"] != "1.2.3" || r["instructions"] == "" {
		t.Fatal(r)
	}
	// an unknown version gets the newest legacy one
	r = result(t, c.call(2, "initialize", map[string]any{"protocolVersion": "2099-01-01"}))
	if r["protocolVersion"] != LegacyVersions[0] {
		t.Fatal(r)
	}
	c.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	if r := result(t, c.call(3, "ping", nil)); len(r) != 1 || r["resultType"] != "complete" {
		t.Fatal(r)
	}
	if e := c.call(4, "nope", nil)["error"].(map[string]any); e["code"] != float64(codeMethodNotFound) {
		t.Fatal(e)
	}
}

func TestModernRequests(t *testing.T) {
	c, _ := start(t, Options{Version: "1.2.3", Runner: &fakeRunner{}})
	meta := map[string]any{"_meta": map[string]any{metaVersion: "2026-07-28"}}
	r := result(t, c.call(1, "server/discover", meta))
	if !reflect.DeepEqual(r["supportedVersions"], []any{"2026-07-28", "2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}) ||
		r["resultType"] != "complete" || r["_meta"].(map[string]any)[metaServerInfo] == nil {
		t.Fatal(r)
	}
	r = result(t, c.call(2, "tools/list", meta))
	if r["ttlMs"] == nil || r["cacheScope"] != "private" || r["_meta"] == nil {
		t.Fatal(r)
	}
	e := c.call(3, "tools/list", map[string]any{"_meta": map[string]any{metaVersion: "1900-01-01"}})["error"].(map[string]any)
	if e["code"] != float64(codeUnsupportedVersion) || e["data"].(map[string]any)["requested"] != "1900-01-01" {
		t.Fatal(e)
	}
	// resource not found: Invalid Params in the modern revision, -32002 before
	e = c.call(4, "resources/read", map[string]any{"uri": "gtr://nope", "_meta": meta["_meta"]})["error"].(map[string]any)
	if e["code"] != float64(codeInvalidParams) {
		t.Fatal(e)
	}
	e = c.call(5, "resources/read", map[string]any{"uri": "gtr://nope"})["error"].(map[string]any)
	if e["code"] != float64(codeResourceNotFound) {
		t.Fatal(e)
	}
}

func TestBadMessages(t *testing.T) {
	c, _ := start(t, Options{Runner: &fakeRunner{}})
	c.in.Write([]byte("{not json\n"))
	if e := c.next()["error"].(map[string]any); e["code"] != float64(codeParse) {
		t.Fatal(e)
	}
	c.in.Write([]byte(`[{"jsonrpc":"2.0","id":1,"method":"ping"}]` + "\n"))
	if e := c.next()["error"].(map[string]any); e["code"] != float64(codeInvalidRequest) {
		t.Fatal(e)
	}
	if e := c.call(2, "tools/call", map[string]any{"name": "nope"})["error"].(map[string]any); e["code"] != float64(codeInvalidParams) {
		t.Fatal(e)
	}
}

func TestToolsList(t *testing.T) {
	c, _ := start(t, Options{Runner: &fakeRunner{}})
	tools := result(t, c.call(1, "tools/list", nil))["tools"].([]any)
	var names []string
	byName := map[string]map[string]any{}
	for _, x := range tools {
		m := x.(map[string]any)
		names = append(names, m["name"].(string))
		byName[m["name"].(string)] = m
	}
	want := "project_info list why outdated search info env doctor add remove install update sync link unlink version_bump run test build publish"
	if strings.Join(names, " ") != want {
		t.Fatal(names)
	}
	if a := byName["list"]["annotations"].(map[string]any); a["readOnlyHint"] != true {
		t.Fatal(a)
	}
	if a := byName["remove"]["annotations"].(map[string]any); a["readOnlyHint"] != false || a["destructiveHint"] != true {
		t.Fatal(a)
	}
	schema := byName["add"]["inputSchema"].(map[string]any)
	if schema["required"].([]any)[0] != "packages" || schema["properties"].(map[string]any)["dir"] == nil {
		t.Fatal(schema)
	}
	// self_install only when allowed
	c2, _ := start(t, Options{Runner: &fakeRunner{}, AllowSelfInstall: true})
	tools = result(t, c2.call(1, "tools/list", nil))["tools"].([]any)
	if tools[len(tools)-1].(map[string]any)["name"] != "self_install" {
		t.Fatal(tools)
	}
}

func callTool(t *testing.T, c *client, id float64, name string, args map[string]any) map[string]any {
	t.Helper()
	return result(t, c.call(id, "tools/call", map[string]any{"name": name, "arguments": args}))
}

func text(r map[string]any) string {
	return r["content"].([]any)[0].(map[string]any)["text"].(string)
}

func TestToolCommands(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "core"), 0o755)
	f := &fakeRunner{}
	c, s := start(t, Options{Root: root, Runner: f})
	for i, tc := range []struct {
		tool string
		args map[string]any
		want string
	}{
		{"list", nil, "list --json"},
		{"list", map[string]any{"tree": true, "depth": 2}, "list --tree --depth 2"},
		{"why", map[string]any{"package": "orm"}, "why orm"},
		{"outdated", map[string]any{"all": true}, "outdated --json --all"},
		{"search", map[string]any{"query": "faker"}, "search faker"},
		{"doctor", map[string]any{"offline": true}, "doctor --offline"},
		{"add", map[string]any{"packages": []any{"faker", "github:a/b#^0.2"}, "dev": true}, "add faker github:a/b#^0.2 -D"},
		{"add", map[string]any{"packages": "faker orm"}, "add faker orm"},
		{"remove", map[string]any{"packages": []any{"faker"}}, "remove faker"},
		{"install", map[string]any{"ci": true, "offline": true}, "ci --offline"},
		{"update", nil, "update"},
		{"sync", map[string]any{"force": true}, "sync --force"},
		{"link", map[string]any{"target": "../orm"}, "link ../orm"},
		{"unlink", map[string]any{"package": "orm", "global": true}, "unlink orm --global"},
		{"version_bump", map[string]any{"bump": "prerelease", "preid": "rc", "no_git": true}, "version prerelease --preid rc --no-git"},
		{"run", map[string]any{"script": "migrate", "args": []any{"--seed"}, "no_env": true}, "run --no-env migrate -- --seed"},
		{"test", map[string]any{"kind": "integration", "coverage": true, "run": "TestX", "race": false, "recursive": true}, "test --integration --run TestX --no-race --coverage -r"},
		{"build", map[string]any{"targets": []any{"linux/amd64"}, "archive": true}, "build -t linux/amd64 --archive"},
		{"publish", nil, "publish --dry-run"},
		{"publish", map[string]any{"dry_run": true, "skip_tests": true}, "publish --dry-run --skip-tests"},
	} {
		r := callTool(t, c, float64(i+1), tc.tool, tc.args)
		if r["isError"] != false {
			t.Fatalf("%s: %v", tc.tool, r)
		}
		if got := strings.Join(f.last().args, " "); got != tc.want {
			t.Fatalf("%s: gtr %s, want gtr %s", tc.tool, got, tc.want)
		}
		if f.last().dir != s.Root() {
			t.Fatal(f.last().dir)
		}
	}
	// dir: a member, not outside the root
	callTool(t, c, 100, "test", map[string]any{"dir": "core"})
	if f.last().dir != filepath.Join(s.Root(), "core") {
		t.Fatal(f.last().dir)
	}
	n := len(f.calls)
	for i, d := range []string{"..", "../x", "/etc", "core/../..", "missing"} {
		r := callTool(t, c, float64(200+i), "list", map[string]any{"dir": d})
		if r["isError"] != true {
			t.Fatalf("dir %q: %v", d, r)
		}
	}
	// wrong arguments are tool errors the model can fix
	for i, a := range []struct {
		tool string
		args map[string]any
	}{
		{"add", nil}, {"why", map[string]any{"package": ""}}, {"list", map[string]any{"bogus": 1}},
		{"test", map[string]any{"kind": "smoke"}},
		{"publish", map[string]any{"dry_run": false}},
	} {
		r := callTool(t, c, float64(300+i), a.tool, a.args)
		if r["isError"] != true {
			t.Fatalf("%s %v: %v", a.tool, a.args, r)
		}
	}
	if len(f.calls) != n {
		t.Fatal("a refused call ran a command:", f.calls[n:])
	}
}

func TestSymlinkOutsideRoot(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skip("no symlinks:", err)
	}
	c, _ := start(t, Options{Root: root, Runner: &fakeRunner{}})
	if r := callTool(t, c, 1, "list", map[string]any{"dir": "link"}); r["isError"] != true {
		t.Fatal(r)
	}
}

func TestPublishAllowed(t *testing.T) {
	f := &fakeRunner{}
	c, _ := start(t, Options{Runner: f, AllowPublish: true})
	callTool(t, c, 1, "publish", map[string]any{"dry_run": false, "allow_dirty": true})
	if got := strings.Join(f.last().args, " "); got != "publish --allow-dirty" {
		t.Fatal(got)
	}
}

func TestToolResults(t *testing.T) {
	f := &fakeRunner{reply: func(dir string, args []string) Result {
		switch args[0] {
		case "list":
			return Result{Output: `[{"name": "orm", "version": "0.1.0"}]` + "\n"}
		case "test":
			return Result{ExitCode: 1, Output: strings.Join([]string{
				"--- FAIL: TestSave (0.00s)",
				"FAIL",
				"FAIL\tapp/models\t0.012s",
				"ok  \tapp/http\t0.100s\tcoverage: 81.5% of statements",
				"?   \tapp/cmd\t[no test files]",
				"",
				"coverage app/http                  81.5%  (min 70.0%, ok)",
				"coverage app/models                40.0%  (min 70.0%, BELOW)",
			}, "\n")}
		case "env":
			return Result{Output: `{"GTR_GO_VERSION": "1.26.9"}`}
		}
		return Result{}
	}}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "gtr.json"), []byte(`{"name": "app", "version": "0.1.0", "scripts": {"test": "x", "dev": "y"}, "workspaces": ["libs/*"]}`), 0o644)
	os.MkdirAll(filepath.Join(root, "libs", "a"), 0o755)
	os.WriteFile(filepath.Join(root, "libs", "a", "gtr.json"), []byte(`{"name": "a", "version": "0.2.0"}`), 0o644)
	os.MkdirAll(filepath.Join(root, "libs", "notes"), 0o755)
	c, _ := start(t, Options{Root: root, Runner: f, Version: "9.9.9"})

	r := callTool(t, c, 1, "list", nil)
	sc := r["structuredContent"].(map[string]any)
	if sc["packages"].([]any)[0].(map[string]any)["name"] != "orm" || sc["exit_code"] != float64(0) || sc["command"] != "gtr list --json" {
		t.Fatal(sc)
	}

	r = callTool(t, c, 2, "test", map[string]any{"coverage": true})
	sc = r["structuredContent"].(map[string]any)
	if r["isError"] != true || !strings.Contains(text(r), "[exit code 1]") {
		t.Fatal(r)
	}
	pk := sc["packages"].([]any)
	if len(pk) != 3 || pk[0].(map[string]any)["status"] != "fail" || pk[1].(map[string]any)["coverage"] != 81.5 ||
		pk[2].(map[string]any)["status"] != "no tests" {
		t.Fatal(pk)
	}
	cov := sc["coverage"].([]any)
	if cov[1].(map[string]any)["ok"] != false || cov[1].(map[string]any)["min"] != 70.0 {
		t.Fatal(cov)
	}
	if sc["failed_tests"].([]any)[0] != "TestSave" {
		t.Fatal(sc["failed_tests"])
	}

	r = callTool(t, c, 3, "project_info", nil)
	sc = r["structuredContent"].(map[string]any)
	if sc["gtr_json"].(map[string]any)["name"] != "app" || sc["locked"] != false || sc["gtr_version"] != "9.9.9" ||
		!reflect.DeepEqual(sc["scripts"], []any{"dev", "test"}) || sc["env"].(map[string]any)["GTR_GO_VERSION"] != "1.26.9" {
		t.Fatal(sc)
	}
	if m := sc["members"].([]any); len(m) != 1 || m[0].(map[string]any)["dir"] != "libs/a" {
		t.Fatal(m)
	}
	// in a member, project_info shows the member
	sc = callTool(t, c, 4, "project_info", map[string]any{"dir": "libs/a"})["structuredContent"].(map[string]any)
	if sc["dir"] != "libs/a" || sc["gtr_json"].(map[string]any)["name"] != "a" {
		t.Fatal(sc)
	}
}

func TestResources(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "gtr.json"), []byte(`{"name": "app"}`), 0o644)
	c, _ := start(t, Options{Root: root, Runner: &fakeRunner{}})
	list := result(t, c.call(1, "resources/list", nil))["resources"].([]any)
	var uris []string
	for _, r := range list {
		uris = append(uris, r.(map[string]any)["uri"].(string))
	}
	if strings.Join(uris, " ") != "gtr://gtr.json gtr://schema" { // no gtr.lock yet
		t.Fatal(uris)
	}
	r := result(t, c.call(2, "resources/read", map[string]any{"uri": "gtr://gtr.json"}))
	if r["contents"].([]any)[0].(map[string]any)["text"] != `{"name": "app"}` {
		t.Fatal(r)
	}
	r = result(t, c.call(3, "resources/read", map[string]any{"uri": "gtr://schema"}))
	if !strings.Contains(r["contents"].([]any)[0].(map[string]any)["text"].(string), `"$schema"`) {
		t.Fatal(r)
	}
}

func TestCancelAndProgress(t *testing.T) {
	f := &fakeRunner{block: make(chan struct{})}
	c, _ := start(t, Options{Runner: f})
	c.send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{
		"name": "test", "arguments": map[string]any{}, "_meta": map[string]any{"progressToken": "p1"}}})
	// progress arrives while the tool runs, and other requests are answered
	p := c.next()
	if p["method"] != "notifications/progress" || p["params"].(map[string]any)["progressToken"] != "p1" ||
		p["params"].(map[string]any)["message"] != "started" {
		t.Fatal(p)
	}
	if r := result(t, c.call(2, "ping", nil)); r == nil {
		t.Fatal("ping")
	}
	c.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/cancelled", "params": map[string]any{"requestId": 1}})
	r := result(t, c.response(1))
	if r["isError"] != true || !strings.Contains(text(r), "cancelled") {
		t.Fatal(r)
	}
}

func TestTimeout(t *testing.T) {
	f := &fakeRunner{block: make(chan struct{})}
	c, _ := start(t, Options{Runner: f})
	start := time.Now()
	r := callTool(t, c, 1, "run", map[string]any{"script": "dev", "timeout_seconds": 1})
	if r["isError"] != true || r["structuredContent"].(map[string]any)["timed_out"] != true ||
		!strings.Contains(text(r), "time limit") || time.Since(start) > 4*time.Second {
		t.Fatal(r)
	}
}

func TestInputEnds(t *testing.T) {
	c, _ := start(t, Options{Runner: &fakeRunner{}})
	c.in.Close()
	select {
	case err := <-c.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return")
	}
}

func TestCapture(t *testing.T) {
	var lines []string
	c := &capture{line: func(s string) { lines = append(lines, s) }}
	c.Write([]byte("a\r\nb"))
	c.Write([]byte("c\nd"))
	c.flush()
	if strings.Join(lines, "|") != "a|bc|d" || c.String() != "a\r\nbc\nd" {
		t.Fatal(lines, c.String())
	}
	big := &capture{}
	chunk := strings.Repeat("x", 1000) + "\n"
	for i := 0; i < 100; i++ {
		big.Write([]byte(chunk))
	}
	big.Write([]byte("THE END\n"))
	s := big.String()
	if len(s) > headLimit+tailLimit+100 || !strings.HasSuffix(s, "THE END\n") || !strings.Contains(s, "bytes of output left out") {
		t.Fatal(len(s))
	}
}
