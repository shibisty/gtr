package download

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func payload() []byte { return bytes.Repeat([]byte("0123456789abcdef"), 4096) } // 64 KiB

// flaky serves the payload, cutting the first `cuts` responses in half.
// With ranges=false it ignores Range headers.
func flaky(t *testing.T, cuts int32, ranges bool) (*httptest.Server, *int32, *[]string) {
	t.Helper()
	data := payload()
	var calls int32
	var seen []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		mu.Lock()
		seen = append(seen, r.Header.Get("Range"))
		mu.Unlock()
		start := 0
		if rg := r.Header.Get("Range"); ranges && rg != "" {
			start, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(data)-1, len(data)))
			w.Header().Set("Content-Length", strconv.Itoa(len(data)-start))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		}
		body := data[start:]
		if n <= cuts {
			w.Write(body[:len(body)/2]) // then drop the connection
			if hj, ok := w.(http.Hijacker); ok {
				c, _, _ := hj.Hijack()
				c.Close()
			}
			return
		}
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls, &seen
}

func TestResumeAfterDrop(t *testing.T) {
	srv, calls, seen := flaky(t, 2, true)
	dst := filepath.Join(t.TempDir(), "go.tar.gz")
	var log bytes.Buffer
	g := &Getter{Backoff: time.Millisecond, Log: &log}
	if err := g.File(context.Background(), srv.URL, dst, sum(payload())); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if !bytes.Equal(got, payload()) {
		t.Fatal("content differs")
	}
	if *calls != 3 {
		t.Fatalf("calls = %d, want 3", *calls)
	}
	if (*seen)[0] != "" || !strings.HasPrefix((*seen)[1], "bytes=") || (*seen)[1] == "bytes=0-" {
		t.Fatalf("Range headers = %q, want a resumed request", *seen)
	}
	if strings.Count(log.String(), "retrying") != 2 {
		t.Fatalf("log = %q, want two retry notes", log.String())
	}
}

func TestRestartWhenRangesUnsupported(t *testing.T) {
	srv, _, _ := flaky(t, 1, false)
	dst := filepath.Join(t.TempDir(), "f")
	g := &Getter{Backoff: time.Millisecond}
	if err := g.File(context.Background(), srv.URL, dst, sum(payload())); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dst); !bytes.Equal(got, payload()) {
		t.Fatal("content differs after a restart from zero")
	}
}

func TestGivesUp(t *testing.T) {
	srv, calls, _ := flaky(t, 100, true)
	dir := t.TempDir()
	g := &Getter{Backoff: time.Millisecond, Attempts: 3}
	if err := g.File(context.Background(), srv.URL, filepath.Join(dir, "f"), sum(payload())); err == nil {
		t.Fatal("want an error")
	}
	if *calls != 3 {
		t.Fatalf("calls = %d, want 3", *calls)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("left files behind: %v", entries)
	}
}

func TestStatusAndChecksum(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		switch r.URL.Path {
		case "/missing":
			http.NotFound(w, r)
		case "/busy":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.Write([]byte("hello"))
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	g := &Getter{Backoff: time.Millisecond, Attempts: 2}
	err := g.File(context.Background(), srv.URL+"/missing", filepath.Join(dir, "a"), sum(nil))
	if err == nil || !strings.Contains(err.Error(), "404") || calls != 1 {
		t.Fatalf("404: err = %v after %d calls; want one call", err, calls)
	}
	calls = 0
	err = g.File(context.Background(), srv.URL+"/busy", filepath.Join(dir, "b"), sum(nil))
	if err == nil || !strings.Contains(err.Error(), "503") || calls != 2 {
		t.Fatalf("503: err = %v after %d calls; want two calls", err, calls)
	}
	err = g.File(context.Background(), srv.URL+"/ok", filepath.Join(dir, "c"), sum([]byte("other")))
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("err = %v, want a mismatch", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "c")); err == nil {
		t.Fatal("an unverified file was kept")
	}
	if err := g.File(context.Background(), srv.URL+"/ok", filepath.Join(dir, "d"), strings.ToUpper(sum([]byte("hello")))); err != nil {
		t.Fatalf("upper-case checksum: %v", err)
	}
}

func TestHeaderAndCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer x" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	g := &Getter{Header: func(r *http.Request) { r.Header.Set("Authorization", "Bearer x") }}
	if err := g.File(context.Background(), srv.URL, filepath.Join(dir, "a"), sum([]byte("ok"))); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.File(ctx, srv.URL, filepath.Join(dir, "b"), sum([]byte("ok"))); err == nil {
		t.Fatal("cancelled download succeeded")
	}
}

func TestRangeStart(t *testing.T) {
	for in, want := range map[string]int64{"bytes 100-199/200": 100, "bytes 0-1/2": 0, "": -1, "items 1-2/3": -1, "bytes x-1/2": -1} {
		if got := rangeStart(in); got != want {
			t.Errorf("rangeStart(%q) = %d, want %d", in, got, want)
		}
	}
}

// A server that answers a resume with another range (or 416) makes the
// download start over instead of failing.
func TestResumeMismatch(t *testing.T) {
	data := payload()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		switch {
		case n == 1: // half, then a dropped connection
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.Write(data[:len(data)/2])
			if hj, ok := w.(http.Hijacker); ok {
				c, _, _ := hj.Hijack()
				c.Close()
			}
		case r.Header.Get("Range") != "":
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(data)-1, len(data))) // wrong start
			w.WriteHeader(http.StatusPartialContent)
			w.Write(data)
		default:
			w.Write(data)
		}
	}))
	defer srv.Close()
	dst := filepath.Join(t.TempDir(), "f")
	if err := (&Getter{Backoff: time.Millisecond}).File(context.Background(), srv.URL, dst, sum(data)); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dst); !bytes.Equal(got, data) || calls != 3 {
		t.Fatalf("calls %d, equal %v", calls, bytes.Equal(got, data))
	}
}
