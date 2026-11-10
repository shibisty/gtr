// Package cli parses and dispatches gtr commands.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gtr/internal/config"
	"gtr/internal/goenv"
	"gtr/internal/goinstall"
	"gtr/internal/home"
	"gtr/internal/manager"
	"gtr/internal/project"
	"gtr/internal/release"
	"gtr/internal/resolve"
	"gtr/internal/runner"
	"gtr/internal/selfupdate"
	"gtr/internal/semver"
)

// ErrUsage reports invalid usage; main prints a hint and exits with code 2.
var ErrUsage = errors.New("invalid usage")

// ExitCode is returned when a child process (gtr-manager, go) exited with a
// non-zero code; main exits with the same code without printing anything.
type ExitCode int

func (e ExitCode) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

// projectCommands are implemented by gtr-manager; gtr forwards them.
var projectCommands = map[string]bool{
	"new": true, "init": true,
	"install": true, "i": true, "add": true, "require": true,
	"uninstall": true, "ui": true, "u": true, "remove": true, "rm": true,
	"update": true, "up": true, "upgrade": true, "ci": true, "sync": true,
	"run": true, "start": true,
	"list": true, "ls": true, "why": true, "outdated": true,
	"build": true, "test": true, "dev": true, "publish": true, "link": true, "unlink": true,
	"login": true, "logout": true, "whoami": true, "search": true, "info": true, "view": true,
	"yank": true, "unyank": true, "owner": true, "owners": true,
	// "version" with arguments (gtr version patch); alone it prints gtr's.
}

// ShimNames are the Go tools gtr can stand in for (see `gtr self shims`).
var ShimNames = []string{"go", "gofmt"}

// App is gtr with its dependencies, which tests replace.
type App struct {
	Version    string
	In         io.Reader // gtr mcp reads requests from it (nil: os.Stdin)
	Out        io.Writer
	Err        io.Writer
	GOOS       string
	Home       func() (home.Layout, error)
	Getenv     func(string) string
	Getwd      func() (string, error)
	Environ    func() []string
	Executable func() (string, error)
	// NewGo returns the Go installer for a home.
	NewGo func(l home.Layout) *goinstall.Installer
	// NewSource returns a GitHub release source for app ("gtr-manager", "gtr").
	NewSource func(l home.Layout, app, repo string) *release.Source
	// Exec runs a child process and returns its exit code.
	Exec func(bin string, args, env []string) (int, error)
	// ExecGo runs the go tool in a gtr project (with error hints); nil: Exec.
	ExecGo func(bin string, args, env []string) (int, error)
	// HTTP is used by `gtr doctor` for network checks; nil: a client with a
	// 10-second timeout.
	HTTP *http.Client
}

func (a *App) exe(name string) string {
	if a.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func (a *App) resolver(l home.Layout) *resolve.Resolver {
	return &resolve.Resolver{
		Home:             l,
		Getenv:           a.Getenv,
		GoInstaller:      a.NewGo(l),
		ManagerInstaller: a.managerInstaller(l),
		ManagerBinary:    a.exe("gtr-manager"),
		Log:              a.Err,
	}
}

func (a *App) managerInstaller(l home.Layout) *manager.Installer {
	// Install messages go to stderr: stdout belongs to the forwarded command.
	return &manager.Installer{Home: l, Source: a.NewSource(l, "gtr-manager", manager.Repo), Log: a.Err}
}

// Run runs the command given by args (without the program name).
func (a *App) Run(ctx context.Context, args []string) error {
	args, err := a.globalFlags(args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		Help(a.Out, a.Version)
		return nil
	}
	switch cmd := args[0]; cmd {
	case "help", "-h", "--help":
		Help(a.Out, a.Version)
		return nil
	case "version", "-v", "--version":
		if cmd == "version" && len(args) > 1 {
			return a.forward(ctx, args) // gtr version patch: the package version
		}
		fmt.Fprintln(a.Out, "gtr", a.Version)
		return nil
	case "self":
		return a.self(ctx, args[1:])
	case "env":
		return a.env(ctx, args[1:])
	case "doctor":
		return a.doctor(ctx, args[1:])
	case "mcp":
		return a.mcp(ctx, args[1:])
	default:
		if projectCommands[cmd] {
			return a.forward(ctx, args)
		}
		// In a project, gtr-manager runs a script of that name (gtr migrate
		// → gtr run migrate) or suggests the command that was meant.
		if wd, err := a.Getwd(); err == nil && !strings.HasPrefix(cmd, "-") {
			if p, _ := project.Find(wd); p != nil {
				return a.forward(ctx, args)
			}
		}
		if s := suggest(cmd); s != "" {
			return fmt.Errorf("%w: unknown command %q; did you mean `gtr %s`?", ErrUsage, cmd, s)
		}
		return fmt.Errorf("%w: unknown command %q", ErrUsage, cmd)
	}
}

// forward runs a project command in gtr-manager with the project's Go.
func (a *App) forward(ctx context.Context, args []string) error {
	l, err := a.Home()
	if err != nil {
		return err
	}
	dir, err := a.Getwd()
	if err != nil {
		return err
	}
	r := a.resolver(l)
	m, err := r.Manager(ctx, dir)
	if err != nil {
		return err
	}
	env := a.Environ()
	if g, err := r.Go(ctx, dir); err != nil {
		return err
	} else if g != nil {
		env = runner.GoEnv(env, g.Path, g.Version)
	}
	code, err := a.Exec(m.Path, args, env)
	if err != nil {
		return fmt.Errorf("run gtr-manager (%s): %w", m.Path, err)
	}
	if code != 0 {
		return ExitCode(code)
	}
	return nil
}

// Shim runs the Go tool name ("go", "gofmt") of the version selected for
// the current directory. main calls it when gtr is started as a shim.
func (a *App) Shim(ctx context.Context, name string, args []string) error {
	l, err := a.Home()
	if err != nil {
		return err
	}
	dir, err := a.Getwd()
	if err != nil {
		return err
	}
	g, err := a.resolver(l).Go(ctx, dir)
	if err != nil {
		return err
	}
	self, _ := a.Executable()
	var bin string
	env := a.Environ()
	if g == nil {
		// Nothing selected: behave as if the shim were not there and run the
		// system Go from PATH, skipping the shims themselves.
		bin = a.lookPathSkipping(a.exe(name), filepath.Join(l.Root, "bin"), self)
		if bin == "" {
			return fmt.Errorf("no Go version selected for %s and no other %s in PATH: add engines.go to gtr.json or run `gtr self use go <version>`", dir, name)
		}
	} else {
		bin = resolve.GoBin(g.Path, name, a.GOOS)
		env = runner.GoEnv(env, g.Path, g.Version)
	}
	if self != "" && samePath(self, bin) {
		return fmt.Errorf("%s resolves to the gtr shim itself", bin)
	}
	execute := a.Exec
	if name == "go" {
		// In a project whose go.mod gtr generated, go must not download or
		// pick up a go.work: everything is in gtr_modules (ADR-0001).
		// go uses the nearest go.mod: a nested module of its own (tools/go.mod)
		// is not part of the gtr project.
		modDir := dir
		if len(args) > 1 && args[0] == "-C" {
			modDir = args[1]
			if !filepath.IsAbs(modDir) {
				modDir = filepath.Join(dir, modDir)
			}
		}
		if mode, work := goMode(modDir); mode != "" {
			env = goenv.Env(env, work) // work is set in a gtr workspace (ADR-0009)
			if a.ExecGo != nil {
				execute = a.ExecGo
			}
		}
	}
	code, err := execute(bin, args, env)
	if err != nil {
		return fmt.Errorf("run %s: %w", bin, err)
	}
	if code != 0 {
		return ExitCode(code)
	}
	return nil
}

// lookPathSkipping finds file in PATH, skipping skipDir and any file that is
// self (a shim copied or linked elsewhere).
func (a *App) lookPathSkipping(file, skipDir, self string) string {
	for _, dir := range filepath.SplitList(a.Getenv("PATH")) {
		if dir == "" || a.samePathString(dir, skipDir) {
			continue
		}
		p := filepath.Join(dir, file)
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			continue
		}
		if self != "" && samePath(p, self) {
			continue
		}
		return p
	}
	return ""
}

func (a *App) samePathString(x, y string) bool {
	x, y = filepath.Clean(x), filepath.Clean(y)
	if a.GOOS == "windows" {
		return strings.EqualFold(x, y)
	}
	return x == y
}

func samePath(a, b string) bool {
	sa, ea := os.Stat(a)
	sb, eb := os.Stat(b)
	return ea == nil && eb == nil && os.SameFile(sa, sb)
}

func (a *App) self(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: specify a self subcommand (install, uninstall, use, list, current, link, unlink, shims, update, prune)", ErrUsage)
	}
	l, err := a.Home()
	if err != nil {
		return err
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "install":
		tool, req, err := toolArgs(rest, false)
		if err != nil {
			return err
		}
		_, err = a.install(ctx, l, tool, req)
		return err
	case "uninstall":
		tool, v, err := toolArgs(rest, true)
		if err != nil {
			return err
		}
		return a.uninstall(l, tool, v)
	case "use":
		tool, req, err := toolArgs(rest, true)
		if err != nil {
			return err
		}
		return a.use(ctx, l, tool, req)
	case "list", "ls":
		tool, remote, all := "", false, false
		for _, arg := range rest {
			switch {
			case arg == "--remote" || arg == "-r":
				remote = true
			case arg == "--all" || arg == "-a":
				all = true
			case (arg == "go" || arg == "gtr") && tool == "":
				tool = arg
			default:
				return fmt.Errorf("%w: self list [go|gtr] [--remote [--all]]", ErrUsage)
			}
		}
		if all && !remote {
			return fmt.Errorf("%w: --all goes with --remote", ErrUsage)
		}
		if remote {
			return a.listRemote(ctx, l, tool, all)
		}
		return a.list(l, tool)
	case "prune":
		return a.prune(ctx, l, rest)
	case "current":
		return a.current(ctx, l)
	case "link":
		if len(rest) != 2 || rest[0] != "gtr" {
			return fmt.Errorf("%w: self link gtr <path to gtr-manager executable>", ErrUsage)
		}
		return a.link(l, rest[1])
	case "unlink":
		if len(rest) != 1 || rest[0] != "gtr" {
			return fmt.Errorf("%w: self unlink gtr", ErrUsage)
		}
		return a.unlink(l)
	case "shims":
		if len(rest) != 0 {
			return fmt.Errorf("%w: self shims takes no arguments", ErrUsage)
		}
		return a.shims(l)
	case "update":
		if len(rest) > 1 {
			return fmt.Errorf("%w: self update [version]", ErrUsage)
		}
		req := ""
		if len(rest) == 1 {
			req = rest[0]
		}
		return a.update(ctx, l, req)
	default:
		return fmt.Errorf("%w: unknown self subcommand %q", ErrUsage, sub)
	}
}

// toolArgs parses "<go|gtr> [version]".
func toolArgs(args []string, needVersion bool) (tool, version string, err error) {
	if len(args) == 0 || (args[0] != "go" && args[0] != "gtr") {
		return "", "", fmt.Errorf("%w: specify go or gtr, e.g. `gtr self install go 1.26`", ErrUsage)
	}
	if len(args) > 2 {
		return "", "", fmt.Errorf("%w: unexpected arguments: %s (quote ranges: \">=1.26 <1.28\")", ErrUsage, strings.Join(args[2:], " "))
	}
	if len(args) == 2 {
		return args[0], args[1], nil
	}
	if needVersion {
		return "", "", fmt.Errorf("%w: specify a %s version, e.g. 1.26.9 or ^1.26", ErrUsage, args[0])
	}
	return args[0], "", nil
}

// install installs a version of tool. For Go without a version it installs
// what the current project requires.
func (a *App) install(ctx context.Context, l home.Layout, tool, req string) (string, error) {
	if tool == "gtr" {
		if req == "" {
			req = "*"
		}
		return a.managerInstaller(l).Install(ctx, req)
	}
	if req == "" {
		dir, err := a.Getwd()
		if err != nil {
			return "", err
		}
		g, err := a.resolver(l).Go(ctx, dir) // installs if needed
		if err != nil {
			return "", err
		}
		if g == nil {
			return "", fmt.Errorf("%w: specify a Go version (no gtr.json with engines.go here)", ErrUsage)
		}
		fmt.Fprintf(a.Out, "Go %s (%s)\n", g.Version, g.Source)
		return g.Version, nil
	}
	in := a.NewGo(l)
	in.Log = a.Out
	return in.Install(ctx, req)
}

func (a *App) uninstall(l home.Layout, tool, version string) error {
	v, err := semver.Parse(version)
	if err != nil {
		return err
	}
	// Directory names: Go keeps its release names ("1.20", "1.27rc1"),
	// gtr-manager uses normalized SemVer ("0.2.0").
	version = strings.TrimPrefix(strings.TrimPrefix(version, "v"), "go")
	dir := l.GoVersion(version)
	if tool == "gtr" {
		version = v.String()
		dir = filepath.Join(l.ManagerDir(), version)
	}
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("%s %s is not installed", toolName(tool), version)
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	c, err := config.Load(l)
	if err != nil {
		return err
	}
	if tool == "go" && c.Go == version || tool == "gtr" && c.Gtr == version {
		if tool == "go" {
			c.Go = ""
		} else {
			c.Gtr = ""
		}
		if err := config.Save(l, c); err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "%s %s was the default; no default is set now\n", toolName(tool), version)
	}
	fmt.Fprintf(a.Out, "Removed %s %s\n", toolName(tool), version)
	return nil
}

// use sets the global default version, installing it if needed.
func (a *App) use(ctx context.Context, l home.Layout, tool, req string) error {
	rng, err := semver.ParseRange(req)
	if err != nil {
		return err
	}
	var installed []string
	if tool == "go" {
		installed, err = goinstall.Installed(l)
	} else {
		installed, err = manager.Installed(l, a.exe("gtr-manager"))
	}
	if err != nil {
		return err
	}
	v, ok := rng.Best(installed)
	if !ok {
		if v, err = a.install(ctx, l, tool, req); err != nil {
			return err
		}
	}
	c, err := config.Load(l)
	if err != nil {
		return err
	}
	if tool == "go" {
		c.Go = v
	} else {
		c.Gtr = v
	}
	if err := config.Save(l, c); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Default %s: %s\n", toolName(tool), v)
	if tool == "gtr" && c.Links["gtr"] != "" {
		fmt.Fprintf(a.Out, "Note: a linked build (%s) still takes precedence; run `gtr self unlink gtr` to use %s.\n", c.Links["gtr"], v)
	}
	return nil
}

func (a *App) list(l home.Layout, tool string) error {
	c, err := config.Load(l)
	if err != nil {
		return err
	}
	if tool == "" || tool == "go" {
		vs, err := goinstall.Installed(l)
		if err != nil {
			return err
		}
		printList(a.Out, "Go", l.GoDir(), vs, c.Go)
	}
	if tool == "" || tool == "gtr" {
		vs, err := manager.Installed(l, a.exe("gtr-manager"))
		if err != nil {
			return err
		}
		printList(a.Out, "gtr-manager", l.ManagerDir(), vs, c.Gtr)
		if link := c.Links["gtr"]; link != "" {
			fmt.Fprintf(a.Out, "  linked: %s (takes precedence)\n", link)
		}
	}
	return nil
}

func printList(w io.Writer, name, dir string, versions []string, def string) {
	if len(versions) == 0 {
		fmt.Fprintf(w, "%s: none installed (%s)\n", name, dir)
		return
	}
	fmt.Fprintf(w, "%s (%s):\n", name, dir)
	for _, v := range versions {
		mark := " "
		if v == def {
			mark = "*"
		}
		fmt.Fprintf(w, "%s %s\n", mark, v)
	}
}

// current shows which versions apply here and why. Missing versions are
// installed, exactly as the next project command would do.
func (a *App) current(ctx context.Context, l home.Layout) error {
	dir, err := a.Getwd()
	if err != nil {
		return err
	}
	r := a.resolver(l)
	switch g, err := r.Go(ctx, dir); {
	case err != nil:
		fmt.Fprintf(a.Out, "Go:          error: %v\n", err)
	case g == nil:
		fmt.Fprintln(a.Out, "Go:          none selected (system Go from PATH is used)")
	default:
		fmt.Fprintf(a.Out, "Go:          %s  from %s\n", g.Version, g.Source)
	}
	switch m, err := r.Manager(ctx, dir); {
	case err != nil:
		fmt.Fprintf(a.Out, "gtr-manager: error: %v\n", err)
	default:
		fmt.Fprintf(a.Out, "gtr-manager: %s  from %s\n", m.Version, m.Source)
	}
	return nil
}

func (a *App) link(l home.Layout, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if st, err := os.Stat(abs); err != nil || st.IsDir() {
		return fmt.Errorf("%s is not a file; build gtr-manager first (e.g. `go build -o build/gtr-manager ./cmd/gtr-manager` in gtr-manager/)", abs)
	}
	c, err := config.Load(l)
	if err != nil {
		return err
	}
	if c.Links == nil {
		c.Links = map[string]string{}
	}
	c.Links["gtr"] = abs
	if err := config.Save(l, c); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "gtr-manager linked: %s\n", abs)
	return nil
}

func (a *App) unlink(l home.Layout) error {
	c, err := config.Load(l)
	if err != nil {
		return err
	}
	if c.Links["gtr"] == "" {
		return errors.New("gtr-manager is not linked")
	}
	delete(c.Links, "gtr")
	if err := config.Save(l, c); err != nil {
		return err
	}
	fmt.Fprintln(a.Out, "gtr-manager unlinked")
	return nil
}

// shims creates ~/.gtr/bin/go and gofmt: copies of (or hard links to) gtr
// that run the Go tool of the project's version.
func (a *App) shims(l home.Layout) error {
	self, err := a.Executable()
	if err != nil {
		return err
	}
	return a.placeShims(l, self)
}

// nearestGoMod returns the directory of the go.mod the go command would use
// in dir, or "".
func nearestGoMod(dir string) string {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for {
		if st, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !st.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// placeShims creates the shims from the gtr executable at self.
func (a *App) placeShims(l home.Layout, self string) error {
	bin := filepath.Join(l.Root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return err
	}
	for _, name := range ShimNames {
		if err := placeShim(self, filepath.Join(bin, a.exe(name))); err != nil {
			return err
		}
	}
	fmt.Fprintf(a.Out, "Shims for %s created in %s\n", strings.Join(ShimNames, ", "), bin)
	if !a.inPath(a.Getenv("PATH"), bin) {
		fmt.Fprintf(a.Out, "Add it to PATH before other Go installations:\n")
		if a.GOOS == "windows" {
			// Not `setx PATH "...;%PATH%"` (it copies the system PATH into the user
			// PATH and truncates it at 1024 characters), and not
			// [Environment]::SetEnvironmentVariable (it expands %VARIABLES% in the
			// PATH for good): the registry value, unexpanded, as install.ps1 does.
			fmt.Fprintf(a.Out, "  PowerShell: $k = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true); "+
				"$k.SetValue('Path', '%s;' + $k.GetValue('Path', '', 'DoNotExpandEnvironmentNames'), 'ExpandString'); $k.Close()\n", bin)
			fmt.Fprintf(a.Out, "  then sign out and in again (or restart Explorer) and open a new terminal\n")
		} else {
			fmt.Fprintf(a.Out, "  export PATH=\"%s:$PATH\"   (add to ~/.profile or your shell rc)\n", bin)
		}
	}
	return nil
}

// placeShim hard-links self to dst (falls back to a copy, e.g. across drives).
func placeShim(self, dst string) error {
	tmp := dst + ".tmp"
	os.Remove(tmp)
	if err := os.Link(self, tmp); err != nil {
		if err := copyFile(self, tmp); err != nil {
			return err
		}
	}
	os.Remove(dst + ".old")
	if _, err := os.Stat(dst); err == nil {
		// A running shim cannot be replaced on Windows, but it can be renamed.
		if err := os.Rename(dst, dst+".old"); err != nil {
			os.Remove(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	os.Remove(dst + ".old")
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o755)
}

func (a *App) inPath(pathEnv, dir string) bool {
	for _, p := range filepath.SplitList(pathEnv) {
		if p != "" && a.samePathString(p, dir) {
			return true
		}
	}
	return false
}

// update replaces gtr with a release and refreshes existing shims.
func (a *App) update(ctx context.Context, l home.Layout, req string) error {
	exe, err := a.Executable()
	if err != nil {
		return err
	}
	src := a.NewSource(l, "gtr", selfupdate.Repo)
	if _, err := selfupdate.Update(ctx, src, a.Version, req, exe, a.Out); err != nil {
		return err
	}
	// Refresh shims from the path captured before the update: after the
	// rename, os.Executable may report the deleted gtr.old.
	bin := filepath.Join(l.Root, "bin")
	if _, err := os.Stat(filepath.Join(bin, a.exe("go"))); err == nil {
		return a.placeShims(l, exe)
	}
	return nil
}

func toolName(tool string) string {
	if tool == "gtr" {
		return "gtr-manager"
	}
	return "Go"
}

// Help prints the usage.
func Help(w io.Writer, version string) {
	fmt.Fprintf(w, `gtr %s — Go version manager and launcher for gtr-manager

Usage:
    gtr <command> [arguments]

Project commands (run by gtr-manager with the project's Go; see gtr-manager help):
    new, init                      Create a project
    install, add, remove, update   Manage packages (gtr.json, gtr.lock, gtr_modules/, go.mod)
    ci, sync                       Strict install from gtr.lock; regenerate go.mod
    run, start                     Run a script
    list, why, outdated            Inspect installed packages
    build, dev, test               Build (cross-compile, archives), run with reload, test
    version <bump>, publish        Release: bump gtr.json and tag; publish to the registry or push the tag
    link, unlink                   Use a local copy of a package
    login, search, info, yank, owner
                                   The gtr package registry (GTR_REGISTRY)

Tools:
    env [--json] [NAME...]        Show the Go, gtr-manager and paths used here (installs nothing)
    doctor [--offline]            Check the installation, PATH, the project and the network
    mcp [--allow-publish]         MCP server on stdio for Claude Code / Claude Desktop
                                  (--print-config: client settings; mcp bundle: a .mcpb extension)

Versions:
    self install go [version]     Install Go (no version: what gtr.json requires)
    self install gtr [version]    Install gtr-manager from GitHub releases
    self uninstall go|gtr <ver>   Remove an installed version
    self use go|gtr <version>     Set the global default (installs it if needed)
    self list [go|gtr]            List installed versions (* = default)
    self list --remote [go|gtr]   List versions available to install (--all: every patch)
    self current                  Show the versions used in this directory and why
    self link gtr <path>          Use a local gtr-manager build instead of releases
    self unlink gtr               Stop using the linked build
    self shims                    Create go/gofmt shims in ~/.gtr/bin
    self update [version]         Update gtr itself
    self prune [--dry-run]        Remove downloaded archives and superseded patch versions

    version                       Show the gtr version (gtr version patch: the package's)
    help                          Show this help

Version syntax: 1.26.9 (exact), 1.26 (latest 1.26.x), ^1.22, ~1.26, ">=1.26 <1.28".
The project's versions come from gtr.json: "engines": {"go": ">=1.26", "gtr": "^0.2"}.
Overrides: gtr --go <version> <command>, gtr --gtr <version> <command>, or the
GTR_GO_VERSION and GTR_MANAGER_VERSION variables. Home: %s (GTR_HOME).
`, version, homeHint())
}

func homeHint() string {
	if d, err := home.Dir(); err == nil {
		return d
	}
	return "~/.gtr"
}

// suggest returns the command closest to a mistyped one ("" when none is
// one or two edits away).
func suggest(cmd string) string {
	names := []string{"help", "version", "self", "env", "doctor"}
	for n := range projectCommands {
		names = append(names, n)
	}
	sort.Strings(names)
	best, bestDist := "", 3
	for _, n := range names {
		if d := editDistance(cmd, n); d < bestDist {
			best, bestDist = n, d
		}
	}
	if bestDist > len(cmd)/2 {
		return ""
	}
	return best
}

// editDistance is the Damerau-Levenshtein distance ("tset" → "test" is one edit).
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	d := make([][]int, len(ra)+1)
	for i := range d {
		d[i] = make([]int, len(rb)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(ra)][len(rb)]
}
