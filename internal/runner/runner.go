// Package runner starts child processes with the environment of the
// selected Go version.
package runner

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"gtr/internal/goenv"
)

// GoEnv returns base with the selected Go first in PATH:
//
//	PATH           <goroot>/bin prepended
//	GOROOT         <goroot>
//	GTR_GO_VERSION <version> — nested gtr calls and shims keep the same Go
//	GOTOOLCHAIN    local, unless already set — Go must not download another
//	               toolchain behind gtr's back
//
// An empty goroot returns base unchanged.
func GoEnv(base []string, goroot, version string) []string {
	if goroot == "" {
		return base
	}
	env := make([]string, 0, len(base)+3)
	pathSet, toolchainSet := false, false
	for _, kv := range base {
		k, v, _ := strings.Cut(kv, "=")
		switch {
		case equalKey(k, "PATH"):
			kv = k + "=" + filepath.Join(goroot, "bin") + string(os.PathListSeparator) + v
			pathSet = true
		case equalKey(k, "GOROOT"), equalKey(k, "GTR_GO_VERSION"):
			continue
		case equalKey(k, "GOTOOLCHAIN"):
			toolchainSet = true
		}
		env = append(env, kv)
	}
	if !pathSet {
		env = append(env, "PATH="+filepath.Join(goroot, "bin"))
	}
	if !toolchainSet {
		env = append(env, "GOTOOLCHAIN=local")
	}
	return append(env, "GOROOT="+goroot, "GTR_GO_VERSION="+version)
}

// equalKey compares environment variable names (case-insensitive on Windows).
func equalKey(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// Run starts bin with args and env, connected to this process's standard
// streams, and returns its exit code. Errors starting the process are
// returned as err.
func Run(bin string, args, env []string) (int, error) {
	return run(bin, args, env, os.Stderr)
}

// RunGo runs a go tool like Run, adding gtr hints to its error output
// (goenv.Hints). Stderr stays untouched when it is a terminal and the
// command runs user programs (go run), which could notice the difference.
func RunGo(bin string, args, env []string) (int, error) {
	if goenv.IsTerminal(os.Stderr) && goenv.RunsPrograms(args) {
		return run(bin, args, env, os.Stderr)
	}
	h := goenv.NewHints(os.Stderr)
	defer h.Close()
	return run(bin, args, env, h)
}

func run(bin string, args, env []string, stderr io.Writer) (int, error) {
	cmd := exec.Command(bin, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, stderr
	cmd.Env = env
	// With a hints pipe, a background process holding it open must not keep
	// gtr waiting after the command exits.
	cmd.WaitDelay = 500 * time.Millisecond
	err := cmd.Run()
	if errors.Is(err, exec.ErrWaitDelay) {
		err = nil // exited successfully; a background process still held stderr
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		// Killed by a signal (Ctrl+C): exit like a shell would, 128+signal.
		if ws, ok := exit.Sys().(interface {
			Signaled() bool
			Signal() syscall.Signal
		}); ok && ws.Signaled() {
			return 128 + int(ws.Signal()), nil
		}
		return exit.ExitCode(), nil
	}
	if err != nil {
		return 1, err
	}
	return 0, nil
}
