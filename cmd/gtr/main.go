package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"

	"gtr/internal/cli"
	"gtr/internal/goinstall"
	"gtr/internal/home"
	"gtr/internal/release"
	"gtr/internal/runner"
	"gtr/internal/selfupdate"
)

// Version is set at build time: -ldflags "-X main.Version=0.0.1".
var Version = "dev"

func main() {
	if exe, err := os.Executable(); err == nil {
		selfupdate.Cleanup(exe)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func newApp(stdout, stderr io.Writer) *cli.App {
	return &cli.App{
		Version:    Version,
		Out:        stdout,
		Err:        stderr,
		GOOS:       runtime.GOOS,
		Home:       home.New,
		Getenv:     os.Getenv,
		Getwd:      os.Getwd,
		Environ:    os.Environ,
		Executable: os.Executable,
		NewGo: func(l home.Layout) *goinstall.Installer {
			return &goinstall.Installer{Home: l, Log: stderr}
		},
		NewSource: func(l home.Layout, app, repo string) *release.Source {
			return &release.Source{App: app, Repo: repo, API: os.Getenv("GTR_GITHUB_API"), Token: githubToken(), Home: l, Log: stderr}
		},
		Exec:   runner.Run,
		ExecGo: runner.RunGo,
	}
}

// run dispatches on the program name: started as "go" or "gofmt" (a shim in
// ~/.gtr/bin), gtr runs that tool of the project's Go version.
func run(ctx context.Context, argv []string, stdout, stderr io.Writer) int {
	app := newApp(stdout, stderr)
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(argv[0])), ".exe")
	var err error
	if isShim(name) {
		// No signal.Ignore here: an ignored SIGINT is inherited by the child,
		// and `go test`/`go run` could no longer be stopped with Ctrl+C. The
		// child gets Ctrl+C from the terminal; gtr's NotifyContext only keeps
		// gtr itself alive until the child exits.
		err = app.Shim(ctx, name, argv[1:])
	} else {
		err = app.Run(ctx, argv[1:])
	}
	return exitCode(err, stderr, name)
}

// githubToken returns the token for the GitHub API in use. With
// GTR_GITHUB_API (GitHub Enterprise or a mirror) it is GTR_GITHUB_TOKEN, so a
// github.com token is never sent to another server.
func githubToken() string {
	if os.Getenv("GTR_GITHUB_API") != "" {
		return os.Getenv("GTR_GITHUB_TOKEN")
	}
	return os.Getenv("GITHUB_TOKEN")
}

func isShim(name string) bool {
	for _, s := range cli.ShimNames {
		if name == s {
			return true
		}
	}
	return false
}

func exitCode(err error, stderr io.Writer, name string) int {
	var code cli.ExitCode
	switch {
	case err == nil:
		return 0
	case errors.As(err, &code):
		return int(code)
	case errors.Is(err, cli.ErrUsage):
		fmt.Fprintln(stderr, "Error:", err)
		fmt.Fprintln(stderr, "Run `gtr help` for usage.")
		return 2
	default:
		prefix := "Error:"
		if isShim(name) {
			prefix = "gtr (" + name + " shim):"
		}
		fmt.Fprintln(stderr, prefix, err)
		return 1
	}
}
