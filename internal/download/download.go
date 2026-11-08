// Package download fetches large files (Go archives, release assets) with a
// SHA-256 check, resuming after a dropped connection and showing progress on
// a terminal.
package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Getter downloads files.
type Getter struct {
	Client *http.Client
	// Header adds request headers (Accept, Authorization); may be nil.
	Header func(req *http.Request)
	// Log receives progress when it is a terminal; nil: no progress.
	Log io.Writer
	// Attempts is the number of tries for one file (default 4). A retry
	// continues where the previous try stopped when the server supports
	// Range requests.
	Attempts int
	// Backoff is the pause before the first retry, doubled for each next
	// one (default 1s).
	Backoff time.Duration
}

// StatusError is a non-retryable HTTP response.
type StatusError struct {
	URL    string
	Status string
	Code   int
}

func (e *StatusError) Error() string { return "server responded " + e.Status }

// File downloads url to dst and checks that its SHA-256 is wantSHA (hex). The
// file appears at dst only when complete and verified.
func (g *Getter) File(ctx context.Context, url, dst, wantSHA string) error {
	attempts := g.Attempts
	if attempts <= 0 {
		attempts = 4
	}
	backoff := g.Backoff
	if backoff == 0 {
		backoff = time.Second
	}
	client := g.Client
	if client == nil {
		client = http.DefaultClient
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".*.part")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after the rename
	defer tmp.Close()

	p := newProgress(g.Log, filepath.Base(dst))
	var written int64
	for attempt := 1; ; attempt++ {
		var retry bool
		written, retry, err = g.try(ctx, client, url, tmp, written, p)
		if err == nil {
			break
		}
		if !retry || attempt >= attempts || ctx.Err() != nil {
			p.done()
			return err
		}
		p.note(fmt.Sprintf("connection problem (%v), retrying", err))
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			p.done()
			return ctx.Err()
		}
		backoff *= 2
	}
	p.done()
	if err := tmp.Close(); err != nil {
		return err
	}
	got, err := fileSHA256(tmp.Name())
	if err != nil {
		return err
	}
	if got != strings.ToLower(wantSHA) {
		return fmt.Errorf("SHA-256 mismatch (expected %s, got %s)", wantSHA, got)
	}
	return os.Rename(tmp.Name(), dst)
}

// try downloads from offset have; it returns the new size of the file and
// whether a failure is worth retrying.
func (g *Getter) try(ctx context.Context, client *http.Client, url string, f *os.File, have int64, p *progress) (int64, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return have, false, err
	}
	if g.Header != nil {
		g.Header(req)
	}
	if have > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(have, 10)+"-")
	}
	resp, err := client.Do(req)
	if err != nil {
		return have, true, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusPartialContent && have > 0 && rangeStart(resp.Header.Get("Content-Range")) == have:
		// continue at the end of the file
	case have > 0 && (resp.StatusCode == http.StatusPartialContent || resp.StatusCode == http.StatusRequestedRangeNotSatisfiable):
		// The server did not resume where we stopped (another offset, or the
		// file changed): start over without Range.
		if err := f.Truncate(0); err != nil {
			return 0, false, err
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return 0, false, err
		}
		return 0, true, fmt.Errorf("the server could not resume at byte %d (%s)", have, resp.Status)
	case resp.StatusCode == http.StatusOK:
		// the whole file (also when the server ignores Range)
		if err := f.Truncate(0); err != nil {
			return 0, false, err
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return 0, false, err
		}
		have = 0
	case resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests:
		return have, true, &StatusError{URL: url, Status: resp.Status, Code: resp.StatusCode}
	default:
		return have, false, &StatusError{URL: url, Status: resp.Status, Code: resp.StatusCode}
	}
	total := int64(-1)
	if resp.ContentLength >= 0 {
		total = have + resp.ContentLength
	}
	p.start(have, total)
	n, err := io.Copy(f, io.TeeReader(resp.Body, p))
	have += n
	if err != nil {
		return have, !errors.Is(err, context.Canceled), err
	}
	if total >= 0 && have != total {
		return have, true, io.ErrUnexpectedEOF
	}
	return have, false, nil
}

// rangeStart parses "bytes 100-199/200" → 100; -1 when malformed.
func rangeStart(cr string) int64 {
	rest, ok := strings.CutPrefix(strings.TrimSpace(cr), "bytes ")
	if !ok {
		return -1
	}
	start, _, ok := strings.Cut(rest, "-")
	if !ok {
		return -1
	}
	n, err := strconv.ParseInt(start, 10, 64)
	if err != nil {
		return -1
	}
	return n
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// progress draws "name  12.3/67.0 MB  18%" on one terminal line.
type progress struct {
	out     io.Writer // notes (retries); nil: silent
	w       io.Writer // the terminal for the progress line; nil: none
	name    string
	n       int64
	total   int64
	last    time.Time
	printed bool
}

func newProgress(out io.Writer, name string) *progress {
	var w io.Writer
	if f, ok := out.(*os.File); ok && isTerminal(f) {
		w = out
	}
	return &progress{out: out, w: w, name: name}
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func (p *progress) start(have, total int64) { p.n, p.total = have, total }

func (p *progress) Write(b []byte) (int, error) {
	p.n += int64(len(b))
	if p.w != nil && time.Since(p.last) > 200*time.Millisecond {
		p.last = time.Now()
		p.draw()
	}
	return len(b), nil
}

func (p *progress) draw() {
	mb := func(n int64) string { return strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64) }
	line := "  " + p.name + "  " + mb(p.n) + " MB"
	if p.total > 0 {
		line = fmt.Sprintf("  %s  %s/%s MB  %d%%", p.name, mb(p.n), mb(p.total), p.n*100/p.total)
	}
	fmt.Fprintf(p.w, "\r%-70s", line)
	p.printed = true
}

func (p *progress) note(msg string) {
	if p.out == nil {
		return
	}
	if p.printed {
		fmt.Fprintln(p.w)
		p.printed = false
	}
	fmt.Fprintf(p.out, "  %s: %s\n", p.name, msg)
}

func (p *progress) done() {
	if p.w != nil && p.printed {
		p.draw()
		fmt.Fprintln(p.w)
		p.printed = false
	}
}
