package mcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Runner runs a gtr command in dir. line, when not nil, gets each output
// line as it comes. A command that ran and failed is a Result with its
// exit code, not an error.
type Runner interface {
	Run(ctx context.Context, dir string, args []string, line func(string)) (Result, error)
}

// Result is the outcome of a command.
type Result struct {
	Output    string // stdout and stderr, interleaved, possibly shortened
	ExitCode  int
	Truncated bool
}

// Output limits: the start and the end of long output are kept (the end
// has the test summary and the error).
const (
	headLimit = 8 << 10
	tailLimit = 56 << 10
)

// ExecRunner runs the gtr executable itself, so a tool goes through the
// same launcher (the project's Go and gtr-manager) as a person's command.
type ExecRunner struct {
	Exe string
	Env []string // default: os.Environ()
}

// Run implements Runner.
func (r ExecRunner) Run(ctx context.Context, dir string, args []string, line func(string)) (Result, error) {
	cmd := exec.Command(r.Exe, args...)
	cmd.Dir = dir
	env := r.Env
	if env == nil {
		env = os.Environ()
	}
	// No terminal: no colors, no prompts (stdin is empty).
	cmd.Env = append(append([]string(nil), env...), "GTR_MCP=1", "NO_COLOR=1", "TERM=dumb")
	cmd.Stdin = nil
	out := &capture{line: line}
	cmd.Stdout, cmd.Stderr = out, out
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		return Result{}, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		killTree(cmd) // gtr-manager and go test below the launcher too
		select {
		case err = <-done:
		case <-time.After(5 * time.Second):
			err = errors.New("the process did not stop")
		}
	}
	out.flush()
	res := Result{Output: out.String(), Truncated: out.cut > 0}
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		res.ExitCode = ee.ExitCode()
		if res.ExitCode < 0 {
			res.ExitCode = -1 // killed
		}
	default:
		return res, err
	}
	return res, nil
}

// capture keeps the head and the tail of the output and reports lines.
type capture struct {
	mu   sync.Mutex
	head bytes.Buffer
	tail []byte
	cut  int64
	part []byte // the unfinished line
	line func(string)
}

func (c *capture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := len(p)
	if c.line != nil {
		c.part = append(c.part, p...)
		for {
			i := bytes.IndexByte(c.part, '\n')
			if i < 0 {
				break
			}
			c.line(string(bytes.TrimRight(c.part[:i], "\r")))
			c.part = c.part[i+1:]
		}
	}
	if room := headLimit - c.head.Len(); room > 0 {
		k := min(room, len(p))
		c.head.Write(p[:k])
		p = p[k:]
	}
	c.tail = append(c.tail, p...)
	if over := len(c.tail) - tailLimit; over > 0 {
		c.cut += int64(over)
		c.tail = append(c.tail[:0], c.tail[over:]...)
	}
	return n, nil
}

func (c *capture) flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.line != nil && len(c.part) > 0 {
		c.line(string(c.part))
		c.part = nil
	}
}

func (c *capture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cut == 0 {
		return c.head.String() + string(c.tail)
	}
	return c.head.String() + fmt.Sprintf("\n… %d bytes of output left out …\n", c.cut) + string(c.tail)
}
