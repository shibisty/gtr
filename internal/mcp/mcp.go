// Package mcp is `gtr mcp`: a Model Context Protocol server on stdio, so
// that an assistant (Claude Code, Claude Desktop) works with a project
// through gtr instead of the bare go command: it installs packages, runs
// scripts and tests and reads gtr.json and gtr.lock, and sees the same
// error hints as a person in a terminal (ADR-0011).
//
// The server is dual-era: a client that opens with `initialize` gets the
// session semantics of protocol revisions up to 2025-11-25; a request
// that carries `io.modelcontextprotocol/protocolVersion` in its _meta is
// served statelessly by revision 2026-07-28 (server/discover, resultType).
// Messages are newline-delimited JSON-RPC 2.0; stdout carries only
// protocol messages, diagnostics go to Log (stderr).
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Protocol revisions the server speaks. Modern ones carry their version on
// every request; legacy ones negotiate it with initialize.
var (
	ModernVersions = []string{"2026-07-28"}
	LegacyVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}
)

const (
	metaVersion    = "io.modelcontextprotocol/protocolVersion"
	metaServerInfo = "io.modelcontextprotocol/serverInfo"
)

// JSON-RPC and MCP error codes.
const (
	codeParse              = -32700
	codeInvalidRequest     = -32600
	codeMethodNotFound     = -32601
	codeInvalidParams      = -32602
	codeInternal           = -32603
	codeResourceNotFound   = -32002 // legacy revisions; modern ones use codeInvalidParams
	codeUnsupportedVersion = -32022
)

// Options configure a Server.
type Options struct {
	// Root is the project the server works in; tools refuse directories
	// outside it.
	Root string
	// Version is gtr's version (serverInfo).
	Version string
	// AllowPublish lets the publish tool publish (a dry run is always
	// allowed); AllowSelfInstall adds the self_install tool. Both are off
	// unless the user turns them on in the client's settings.
	AllowPublish     bool
	AllowSelfInstall bool
	// Runner runs gtr commands (default: ExecRunner of this executable).
	Runner Runner
	// Log receives diagnostics (default: discarded).
	Log io.Writer
}

// Server is one MCP server.
type Server struct {
	o     Options
	root  string // Root, absolute, symlinks resolved
	tools []*tool

	wmu sync.Mutex
	enc *json.Encoder

	mu       sync.Mutex
	inflight map[string]context.CancelFunc // request id → cancel
	legacy   string                        // negotiated legacy version ("" before initialize)
	wg       sync.WaitGroup
}

// New returns a server for o.Root.
func New(o Options) (*Server, error) {
	if o.Root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		o.Root = wd
	}
	root, err := filepath.Abs(o.Root)
	if err != nil {
		return nil, err
	}
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("mcp: the project root %s is not a directory", o.Root)
	}
	if o.Log == nil {
		o.Log = io.Discard
	}
	if o.Runner == nil {
		return nil, errors.New("mcp: no runner")
	}
	s := &Server{o: o, root: root, inflight: map[string]context.CancelFunc{}}
	s.tools = s.toolTable()
	return s, nil
}

// ToolNames lists the tools' names and descriptions.
func (s *Server) ToolNames() [][2]string {
	out := make([][2]string, len(s.tools))
	for i, t := range s.tools {
		out[i] = [2]string{t.name, t.description}
	}
	return out
}

// Root is the project directory the server works in.
func (s *Server) Root() string { return s.root }

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *rpcError) Error() string { return e.Message }

func errorf(code int, format string, args ...any) *rpcError {
	return &rpcError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// request is what a handler gets.
type request struct {
	id       json.RawMessage
	method   string
	params   json.RawMessage
	modern   bool   // the request carries its protocol version
	version  string // that version, or the negotiated legacy one
	progress any    // _meta.progressToken
}

// Serve reads requests from in and writes responses to out until in ends
// or ctx is done; requests still running are cancelled then.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.enc = json.NewEncoder(out)
	s.enc.SetEscapeHTML(false)
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64*1024), 16<<20)
	lines := make(chan []byte)
	readErr := make(chan error, 1)
	go func() {
		for sc.Scan() {
			select {
			case lines <- append([]byte(nil), sc.Bytes()...):
			case <-ctx.Done():
				return
			}
		}
		readErr <- sc.Err()
	}()
	for {
		select {
		case <-ctx.Done():
			s.wg.Wait()
			return ctx.Err()
		case err := <-readErr:
			cancel()
			s.wg.Wait()
			return err
		case line := <-lines:
			s.handleLine(ctx, line)
		}
	}
}

func (s *Server) send(m message) {
	m.JSONRPC = "2.0"
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if err := s.enc.Encode(m); err != nil {
		fmt.Fprintln(s.o.Log, "gtr mcp: writing a message:", err)
	}
}

func (s *Server) reply(id json.RawMessage, result any, err error) {
	if len(id) == 0 {
		return // a notification: no response
	}
	if err != nil {
		var re *rpcError
		if !errors.As(err, &re) {
			re = &rpcError{Code: codeInternal, Message: err.Error()}
		}
		s.send(message{ID: id, Error: re})
		return
	}
	if result == nil {
		result = map[string]any{}
	}
	s.send(message{ID: id, Result: result})
}

func (s *Server) handleLine(ctx context.Context, line []byte) {
	line = []byte(strings.TrimSpace(string(line)))
	if len(line) == 0 {
		return
	}
	if line[0] == '[' {
		// JSON-RPC batches were removed from MCP in 2025-06-18.
		s.send(message{ID: json.RawMessage("null"), Error: errorf(codeInvalidRequest, "batches are not supported")})
		return
	}
	var m message
	if err := json.Unmarshal(line, &m); err != nil {
		s.send(message{ID: json.RawMessage("null"), Error: errorf(codeParse, "parse error: %v", err)})
		return
	}
	if m.Method == "" {
		if len(m.ID) > 0 && m.Result == nil && m.Error == nil {
			s.reply(m.ID, nil, errorf(codeInvalidRequest, "no method"))
		}
		return // a response to a request of ours: the server sends none
	}
	req, err := s.parse(m)
	if err != nil {
		s.reply(m.ID, nil, err)
		return
	}
	if m.Method == "tools/call" && len(m.ID) > 0 {
		// Tools run concurrently, so a long test does not block list or
		// cancel requests.
		cctx, cancel := context.WithCancel(ctx)
		key := string(m.ID)
		s.mu.Lock()
		s.inflight[key] = cancel
		s.mu.Unlock()
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer func() {
				cancel()
				s.mu.Lock()
				delete(s.inflight, key)
				s.mu.Unlock()
			}()
			res, err := s.callTool(cctx, req)
			s.reply(m.ID, s.complete(req, res), err)
		}()
		return
	}
	res, err := s.dispatch(ctx, req)
	if err == nil {
		res = s.complete(req, res)
	}
	s.reply(m.ID, res, err)
}

// parse reads the protocol version of a request: modern requests carry it
// in _meta; everything else is legacy.
func (s *Server) parse(m message) (*request, error) {
	req := &request{id: m.ID, method: m.Method, params: m.Params}
	var p struct {
		Meta map[string]json.RawMessage `json:"_meta"`
	}
	if len(m.Params) > 0 && string(m.Params) != "null" {
		if err := json.Unmarshal(m.Params, &p); err != nil {
			return nil, errorf(codeInvalidParams, "params must be an object")
		}
	}
	if raw, ok := p.Meta[metaVersion]; ok {
		var v string
		json.Unmarshal(raw, &v)
		if !contains(ModernVersions, v) {
			return nil, &rpcError{Code: codeUnsupportedVersion, Message: "Unsupported protocol version",
				Data: map[string]any{"supported": supportedVersions(), "requested": v}}
		}
		req.modern, req.version = true, v
	} else {
		s.mu.Lock()
		req.version = s.legacy
		s.mu.Unlock()
	}
	if raw, ok := p.Meta["progressToken"]; ok {
		var tok any
		if json.Unmarshal(raw, &tok) == nil {
			req.progress = tok
		}
	}
	return req, nil
}

func supportedVersions() []string {
	return append(append([]string(nil), ModernVersions...), LegacyVersions...)
}

// complete adds what every result of a modern revision carries; legacy
// clients ignore the extra fields.
func (s *Server) complete(req *request, res any) any {
	m, ok := res.(map[string]any)
	if !ok || m == nil {
		return res
	}
	m["resultType"] = "complete"
	if req.modern {
		meta, _ := m["_meta"].(map[string]any)
		if meta == nil {
			meta = map[string]any{}
		}
		meta[metaServerInfo] = s.serverInfo()
		m["_meta"] = meta
	}
	return m
}

func (s *Server) serverInfo() map[string]any {
	return map[string]any{"name": "gtr", "title": "gtr", "version": s.o.Version}
}

func (s *Server) capabilities() map[string]any {
	return map[string]any{"tools": map[string]any{}, "resources": map[string]any{}}
}

const instructions = `gtr is the package manager and launcher of this Go project (gtr.json, gtr.lock).
Use these tools instead of running go get, go mod tidy or editing go.mod: go.mod is generated
from gtr.json. add/remove/update change gtr.json and gtr.lock; test runs go test with the
project's Go version and build tags; run runs a script from gtr.json. Errors carry gtr hints.`

func (s *Server) dispatch(ctx context.Context, req *request) (any, error) {
	switch req.method {
	case "initialize":
		return s.initialize(req)
	case "server/discover":
		return map[string]any{
			"supportedVersions": supportedVersions(),
			"capabilities":      s.capabilities(),
			"instructions":      instructions,
			"_meta":             map[string]any{metaServerInfo: s.serverInfo()},
			"ttlMs":             3600000,
			"cacheScope":        "private",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "notifications/initialized", "notifications/roots/list_changed":
		return nil, nil
	case "notifications/cancelled":
		var p struct {
			RequestID json.RawMessage `json:"requestId"`
		}
		json.Unmarshal(req.params, &p)
		s.mu.Lock()
		if cancel := s.inflight[string(p.RequestID)]; cancel != nil {
			cancel()
		}
		s.mu.Unlock()
		return nil, nil
	case "tools/list":
		list := make([]any, 0, len(s.tools))
		for _, t := range s.tools {
			list = append(list, t.describe())
		}
		return map[string]any{"tools": list, "ttlMs": 3600000, "cacheScope": "private"}, nil
	case "tools/call":
		return s.callTool(ctx, req) // a notification: run, but nobody waits
	case "resources/list":
		return map[string]any{"resources": s.resourceList(), "ttlMs": 60000, "cacheScope": "private"}, nil
	case "resources/templates/list":
		return map[string]any{"resourceTemplates": []any{}, "ttlMs": 3600000, "cacheScope": "private"}, nil
	case "resources/read":
		return s.readResource(req)
	case "prompts/list":
		return map[string]any{"prompts": []any{}, "ttlMs": 3600000, "cacheScope": "private"}, nil
	}
	if strings.HasPrefix(req.method, "notifications/") {
		return nil, nil
	}
	return nil, errorf(codeMethodNotFound, "method not found: %s", req.method)
}

func (s *Server) initialize(req *request) (any, error) {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(req.params, &p); err != nil {
		return nil, errorf(codeInvalidParams, "initialize: %v", err)
	}
	v := p.ProtocolVersion
	if !contains(LegacyVersions, v) {
		v = LegacyVersions[0] // the client decides whether it can speak ours
	}
	s.mu.Lock()
	s.legacy = v
	s.mu.Unlock()
	return map[string]any{
		"protocolVersion": v,
		"capabilities":    s.capabilities(),
		"serverInfo":      s.serverInfo(),
		"instructions":    instructions,
	}, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
