package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gtr/internal/config"
	"gtr/internal/goenv"
	"gtr/internal/goinstall"
	"gtr/internal/home"
	"gtr/internal/manager"
	"gtr/internal/project"
	"gtr/internal/release"
	"gtr/internal/resolve"
	"gtr/internal/selfupdate"
	"gtr/internal/semver"
)

// globalFlags strips the leading --go/--gtr flags:
//
//	gtr --go 1.25 run build      same as GTR_GO_VERSION=1.25 gtr run build
//	gtr --gtr=0.2 install        same as GTR_MANAGER_VERSION=0.2
func (a *App) globalFlags(args []string) ([]string, error) {
	overrides := map[string]string{}
	for len(args) > 0 && strings.HasPrefix(args[0], "--") {
		name, value, hasValue := strings.Cut(args[0][2:], "=")
		key := map[string]string{"go": "GTR_GO_VERSION", "gtr": "GTR_MANAGER_VERSION"}[name]
		if key == "" {
			break
		}
		args = args[1:]
		if !hasValue {
			if len(args) == 0 || strings.HasPrefix(args[0], "-") {
				return nil, fmt.Errorf("%w: --%s needs a version, e.g. --%s 1.26", ErrUsage, name, name)
			}
			value, args = args[0], args[1:]
		}
		if _, err := semver.ParseRange(value); err != nil || value == "" {
			return nil, fmt.Errorf("%w: --%s %q: not a version or range", ErrUsage, name, value)
		}
		overrides[key] = value
	}
	if len(overrides) > 0 {
		getenv := a.Getenv
		a.Getenv = func(k string) string {
			if v, ok := overrides[k]; ok {
				return v
			}
			return getenv(k)
		}
	}
	return args, nil
}

// goMode tells how the go command runs in dir: "workspace" (work is the
// generated go.work, ADR-0009), "project" (a go.mod gtr generated) or ""
// (plain go).
func goMode(dir string) (mode, work string) {
	if work := goenv.FindWork(dir); work != "" {
		return "workspace", work
	}
	if p, err := project.Find(dir); err == nil && p != nil && nearestGoMod(dir) == p.Dir && goenv.Generated(p.Dir) {
		return "project", ""
	}
	return "", ""
}

// --- gtr env ------------------------------------------------------------------

// envVars returns what `gtr env` prints, in order. Nothing is installed.
func (a *App) envVars(ctx context.Context, l home.Layout, dir string) ([][2]string, error) {
	r := a.resolver(l)
	r.NoInstall = true
	var out [][2]string
	add := func(k, v string) { out = append(out, [2]string{k, v}) }
	add("GTR_VERSION", a.Version)
	add("GTR_HOME", l.Root)
	p, err := project.Find(dir)
	if err != nil {
		return nil, err
	}
	pdir := ""
	if p != nil {
		pdir = p.Dir
	}
	add("GTR_PROJECT", pdir)
	mode, work := goMode(dir)
	add("GTR_WORKSPACE", work)

	g, gerr := r.Go(ctx, dir)
	var missing *resolve.NotInstalledError
	switch {
	case gerr == nil && g != nil:
		add("GTR_GO_VERSION", g.Version)
		add("GTR_GO_SOURCE", g.Source)
		add("GOROOT", g.Path)
	case errors.As(gerr, &missing):
		add("GTR_GO_VERSION", "")
		add("GTR_GO_SOURCE", missing.Source)
		add("GTR_GO_REQUIRED", missing.Request)
		add("GOROOT", "")
	case gerr != nil:
		return nil, gerr
	default:
		add("GTR_GO_VERSION", "")
		add("GTR_GO_SOURCE", "none (the go in PATH is used)")
		add("GOROOT", a.Getenv("GOROOT"))
	}
	gopath := a.Getenv("GOPATH")
	if gopath == "" {
		if h := a.Getenv("HOME"); h != "" {
			gopath = filepath.Join(h, "go")
		} else if h := a.Getenv("USERPROFILE"); h != "" {
			gopath = filepath.Join(h, "go")
		}
	}
	add("GOPATH", gopath)

	m, merr := r.Manager(ctx, dir)
	switch {
	case merr == nil:
		add("GTR_MANAGER_VERSION", m.Version)
		add("GTR_MANAGER", m.Path)
		add("GTR_MANAGER_SOURCE", m.Source)
	case errors.As(merr, &missing):
		add("GTR_MANAGER_VERSION", "")
		add("GTR_MANAGER", "")
		add("GTR_MANAGER_SOURCE", missing.Source)
		add("GTR_MANAGER_REQUIRED", missing.Request)
	default:
		return nil, merr
	}
	add("GTR_GO_MODE", mode)
	if mode != "" {
		for _, kv := range goenv.Env(nil, work) {
			k, v, _ := strings.Cut(kv, "=")
			add(k, v)
		}
	}
	return out, nil
}

func (a *App) env(ctx context.Context, args []string) error {
	asJSON := false
	var names []string
	for _, arg := range args {
		switch {
		case arg == "--json":
			asJSON = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("%w: env [--json] [NAME...]", ErrUsage)
		default:
			names = append(names, arg)
		}
	}
	l, err := a.Home()
	if err != nil {
		return err
	}
	dir, err := a.Getwd()
	if err != nil {
		return err
	}
	vars, err := a.envVars(ctx, l, dir)
	if err != nil {
		return err
	}
	if len(names) > 0 {
		byName := map[string]string{}
		for _, kv := range vars {
			byName[kv[0]] = kv[1]
		}
		vars = vars[:0]
		for _, n := range names {
			vars = append(vars, [2]string{n, byName[n]})
		}
		if !asJSON {
			for _, kv := range vars { // like `go env NAME`: values only
				fmt.Fprintln(a.Out, kv[1])
			}
			return nil
		}
	}
	if asJSON {
		obj := map[string]string{}
		for _, kv := range vars {
			obj[kv[0]] = kv[1]
		}
		data, _ := json.MarshalIndent(obj, "", "    ")
		fmt.Fprintln(a.Out, string(data))
		return nil
	}
	for _, kv := range vars {
		fmt.Fprintf(a.Out, "%s=%s\n", kv[0], a.quoteEnv(kv[1]))
	}
	return nil
}

// quoteEnv quotes a value for the shell (sh or cmd/PowerShell).
func (a *App) quoteEnv(v string) string {
	if a.GOOS == "windows" {
		return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
	}
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

// --- gtr doctor -------------------------------------------------------------

type report struct {
	w            io.Writer
	fails, warns int
}

func (r *report) ok(format string, args ...any) { fmt.Fprintf(r.w, "  ok    "+format+"\n", args...) }
func (r *report) info(format string, args ...any) {
	fmt.Fprintf(r.w, "  info  "+format+"\n", args...)
}
func (r *report) warn(format string, args ...any) {
	r.warns++
	fmt.Fprintf(r.w, "  warn  "+format+"\n", args...)
}
func (r *report) fail(format string, args ...any) {
	r.fails++
	fmt.Fprintf(r.w, "  FAIL  "+format+"\n", args...)
}

func (a *App) doctor(ctx context.Context, args []string) error {
	offline := false
	for _, arg := range args {
		if arg != "--offline" {
			return fmt.Errorf("%w: doctor [--offline]", ErrUsage)
		}
		offline = true
	}
	l, err := a.Home()
	if err != nil {
		return err
	}
	dir, err := a.Getwd()
	if err != nil {
		return err
	}
	rep := &report{w: a.Out}
	fmt.Fprintf(a.Out, "gtr %s, home %s\n", a.Version, l.Root)

	fmt.Fprintln(a.Out, "Home:")
	a.checkHome(l, rep)
	c, cerr := config.Load(l)
	if cerr != nil {
		rep.fail("%v", cerr)
		c = &config.Config{}
	}
	a.checkInstalled(l, c, rep)

	fmt.Fprintln(a.Out, "PATH and Go:")
	a.checkPath(l, rep)
	a.checkGoEnv(rep)

	fmt.Fprintln(a.Out, "Project:")
	a.checkProject(ctx, l, dir, rep)

	if !offline {
		fmt.Fprintln(a.Out, "Network:")
		a.checkNetwork(ctx, l, rep)
	}
	fmt.Fprintf(a.Out, "%d problem(s), %d warning(s)\n", rep.fails, rep.warns)
	if rep.fails > 0 {
		return ExitCode(1)
	}
	return nil
}

func (a *App) checkHome(l home.Layout, rep *report) {
	if err := os.MkdirAll(l.Root, 0o755); err != nil {
		rep.fail("cannot create %s: %v", l.Root, err)
		return
	}
	f, err := os.CreateTemp(l.Root, ".doctor-*")
	if err != nil {
		rep.fail("%s is not writable: %v", l.Root, err)
		return
	}
	f.Close()
	os.Remove(f.Name())
	rep.ok("%s is writable", l.Root)
}

func (a *App) checkInstalled(l home.Layout, c *config.Config, rep *report) {
	goVersions := dirNames(l.GoDir())
	var good []string
	for _, v := range goVersions {
		if _, err := os.Stat(filepath.Join(l.GoVersion(v), "bin", a.exe("go"))); err != nil {
			rep.warn("Go %s in %s is incomplete (an interrupted install?): run `gtr self uninstall go %s`", v, l.GoVersion(v), v)
			continue
		}
		good = append(good, v)
	}
	if len(good) == 0 {
		rep.info("no Go installed by gtr yet")
	} else {
		rep.ok("Go %s", strings.Join(good, ", "))
	}
	if c.Go != "" && !contains(good, c.Go) {
		rep.warn("the default Go %s is not installed: run `gtr self install go %s`", c.Go, c.Go)
	}
	var mgood []string
	for _, v := range dirNames(l.ManagerDir()) {
		if _, err := os.Stat(manager.Binary(l, v, a.exe("gtr-manager"))); err != nil {
			rep.warn("gtr-manager %s in %s has no executable: run `gtr self uninstall gtr %s`", v, filepath.Join(l.ManagerDir(), v), v)
			continue
		}
		mgood = append(mgood, v)
	}
	if len(mgood) == 0 {
		rep.info("no gtr-manager installed yet (installed on the first project command)")
	} else {
		rep.ok("gtr-manager %s", strings.Join(mgood, ", "))
	}
	if c.Gtr != "" && !contains(mgood, c.Gtr) {
		rep.warn("the default gtr-manager %s is not installed: run `gtr self install gtr %s`", c.Gtr, c.Gtr)
	}
	if link := c.Links["gtr"]; link != "" {
		if st, err := os.Stat(link); err != nil || st.IsDir() {
			rep.fail("the linked gtr-manager %s does not exist: rebuild it or run `gtr self unlink gtr`", link)
		} else {
			rep.ok("gtr-manager linked: %s", link)
		}
	}
}

func (a *App) checkPath(l home.Layout, rep *report) {
	bin := filepath.Join(l.Root, "bin")
	shim := filepath.Join(bin, a.exe("go"))
	self, _ := a.Executable()
	system := a.lookPathSkipping(a.exe("go"), bin, self)
	if _, err := os.Stat(shim); err != nil {
		rep.info("no shims (optional: `gtr self shims` makes plain go/gofmt follow gtr.json)")
	} else {
		first := a.lookPathSkipping(a.exe("go"), "", "")
		switch {
		case !a.inPath(a.Getenv("PATH"), bin):
			rep.warn("%s is not in PATH: the shims are not used", bin)
		case first != "" && !a.samePathString(filepath.Dir(first), bin):
			rep.warn("%s comes before %s in PATH: plain go ignores gtr.json; put %s first", first, bin, bin)
		default:
			rep.ok("shims in %s come first in PATH", bin)
		}
		if self != "" && !sameContent(self, shim) {
			rep.warn("the shims are from another gtr build: run `gtr self shims`")
		}
	}
	if system != "" {
		rep.info("system Go: %s (used where gtr selects no version)", system)
	} else {
		rep.info("no system Go in PATH")
	}
}

func (a *App) checkGoEnv(rep *report) {
	if v := a.Getenv("GOROOT"); v != "" {
		rep.warn("GOROOT=%s is set globally: gtr sets GOROOT per command, and a global one breaks other Go versions; unset it", v)
	}
	if v := a.Getenv("GOTOOLCHAIN"); v != "" && v != "local" {
		rep.warn("GOTOOLCHAIN=%s: go may download toolchains behind gtr's back; gtr sets GOTOOLCHAIN=local unless you set it", v)
	}
	if a.Getenv("GTR_GOPROXY") == "" {
		if gp := a.Getenv("GOPROXY"); gp != "" && !strings.Contains(gp, "http://") && !strings.Contains(gp, "https://") {
			rep.fail("GOPROXY=%s has no proxy URL, so gtr cannot download Go modules: set GTR_GOPROXY=https://proxy.golang.org", gp)
		}
	}
	if a.Getenv("GITHUB_TOKEN") == "" && a.Getenv("GTR_GITHUB_TOKEN") == "" {
		rep.info("GITHUB_TOKEN is not set: 60 GitHub API requests per hour, no private repositories")
	} else {
		rep.ok("a GitHub token is set")
	}
}

func (a *App) checkProject(ctx context.Context, l home.Layout, dir string, rep *report) {
	p, err := project.Find(dir)
	if err != nil {
		rep.fail("%v", err)
		return
	}
	if p == nil {
		rep.info("no gtr.json in %s or above", dir)
		return
	}
	rep.ok("%s", p.Path)
	r := a.resolver(l)
	r.NoInstall = true
	var missing *resolve.NotInstalledError
	switch g, err := r.Go(ctx, dir); {
	case errors.As(err, &missing):
		rep.warn("%v: the next gtr command installs it (or `gtr self install go`)", err)
	case err != nil:
		rep.fail("Go: %v", err)
	case g != nil:
		rep.ok("Go %s (%s)", g.Version, g.Source)
	}
	switch m, err := r.Manager(ctx, dir); {
	case errors.As(err, &missing):
		rep.warn("%v: the next gtr command installs it", err)
	case err != nil:
		rep.fail("gtr-manager: %v", err)
	default:
		rep.ok("gtr-manager %s (%s)", m.Version, m.Source)
	}
	mode, work := goMode(dir)
	switch {
	case mode == "workspace":
		rep.ok("workspace: %s", work)
	case mode == "project":
		rep.ok("go.mod is generated by gtr")
	default:
		if _, err := os.Stat(filepath.Join(p.Dir, "go.mod")); err != nil {
			rep.warn("no go.mod: run `gtr install`")
		} else {
			rep.warn("%s was not generated by gtr: `gtr sync` carries it over to gtr.json", filepath.Join(p.Dir, "go.mod"))
		}
	}
	root := p.Dir
	if work != "" {
		root = filepath.Dir(work)
	}
	if _, err := os.Stat(filepath.Join(root, "gtr.lock")); err == nil {
		if _, err := os.Stat(filepath.Join(root, "gtr_modules")); err != nil {
			rep.warn("gtr.lock exists but gtr_modules/ does not: run `gtr install`")
		}
	}
}

func (a *App) checkNetwork(ctx context.Context, l home.Layout, rep *report) {
	client := a.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	gi := a.NewGo(l)
	goURL := gi.BaseURL
	if goURL == "" {
		goURL = goinstall.DefaultBaseURL
	}
	src := a.NewSource(l, "gtr", selfupdate.Repo)
	api := src.API
	if api == "" {
		api = release.DefaultAPI
	}
	proxy := a.Getenv("GTR_GOPROXY")
	if proxy == "" {
		proxy = "https://proxy.golang.org"
		for _, e := range strings.FieldsFunc(a.Getenv("GOPROXY"), func(r rune) bool { return r == ',' || r == '|' }) {
			if strings.HasPrefix(e, "http://") || strings.HasPrefix(e, "https://") {
				proxy = e
				break
			}
		}
	}
	for _, t := range []struct{ what, url string }{
		{"Go downloads", strings.TrimRight(goURL, "/") + "/?mode=json"},
		{"GitHub API", strings.TrimRight(api, "/") + "/rate_limit"},
		{"Go module proxy", strings.TrimRight(proxy, "/") + "/golang.org/x/mod/@v/list"},
	} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.url, nil)
		if err != nil {
			rep.fail("%s: %v", t.what, err)
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			rep.fail("%s (%s) is not reachable: %v", t.what, t.url, err)
			continue
		}
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0":
			rep.warn("%s: rate limit exhausted; set GITHUB_TOKEN", t.what)
		case resp.StatusCode == http.StatusUnauthorized:
			rep.warn("%s: the token was rejected (expired or revoked?)", t.what)
		case resp.StatusCode >= 500:
			rep.warn("%s (%s) responded %s", t.what, t.url, resp.Status)
		default:
			rep.ok("%s reachable", t.what)
		}
	}
}

// --- gtr self prune -----------------------------------------------------------

// prune removes downloaded archives, leftovers of interrupted installs and
// versions superseded by a newer patch of the same minor version. The
// default versions and the versions the current directory uses are kept.
func (a *App) prune(ctx context.Context, l home.Layout, args []string) error {
	dry := false
	for _, arg := range args {
		if arg != "--dry-run" && arg != "-n" {
			return fmt.Errorf("%w: self prune [--dry-run]", ErrUsage)
		}
		dry = true
	}
	c, err := config.Load(l)
	if err != nil {
		return err
	}
	keep := map[string]bool{"go/" + c.Go: c.Go != "", "gtr/" + c.Gtr: c.Gtr != ""}
	if dir, err := a.Getwd(); err == nil {
		r := a.resolver(l)
		r.NoInstall = true
		if g, err := r.Go(ctx, dir); err == nil && g != nil {
			keep["go/"+g.Version] = true
		}
		if m, err := r.Manager(ctx, dir); err == nil && m.Version != "link" {
			keep["gtr/"+m.Version] = true
		}
	}
	var targets []string
	// Archives and leftovers; anything touched in the last hour may belong to
	// a gtr running right now (a download, an unpacking) and is kept.
	recent := time.Now().Add(-time.Hour)
	for _, d := range []string{l.Downloads(), l.Tmp()} {
		for _, n := range append(dirNames(d), fileNames(d)...) {
			p := filepath.Join(d, n)
			if st, err := os.Stat(p); err == nil && st.ModTime().After(recent) && (d == l.Tmp() || strings.HasSuffix(n, ".part")) {
				continue
			}
			targets = append(targets, p)
		}
	}
	for _, n := range fileNames(filepath.Join(l.Root, "bin")) {
		if strings.HasSuffix(n, ".old") || strings.HasSuffix(n, ".tmp") {
			targets = append(targets, filepath.Join(l.Root, "bin", n))
		}
	}
	for _, v := range superseded(dirNames(l.GoDir())) {
		if !keep["go/"+v] {
			targets = append(targets, l.GoVersion(v))
		}
	}
	for _, v := range superseded(dirNames(l.ManagerDir())) {
		if !keep["gtr/"+v] {
			targets = append(targets, filepath.Join(l.ManagerDir(), v))
		}
	}
	var total int64
	for _, t := range targets {
		size := diskUsage(t)
		total += size
		rel, _ := filepath.Rel(l.Root, t)
		if dry {
			fmt.Fprintf(a.Out, "would remove %s (%s)\n", filepath.ToSlash(rel), humanSize(size))
			continue
		}
		if err := os.RemoveAll(t); err != nil {
			// In use (a running Go on Windows, another gtr): keep going.
			fmt.Fprintf(a.Err, "warning: cannot remove %s: %v\n", filepath.ToSlash(rel), err)
			total -= size
			continue
		}
		fmt.Fprintf(a.Out, "removed %s (%s)\n", filepath.ToSlash(rel), humanSize(size))
	}
	switch {
	case len(targets) == 0:
		fmt.Fprintln(a.Out, "Nothing to prune")
	case dry:
		fmt.Fprintf(a.Out, "%s would be freed\n", humanSize(total))
	default:
		fmt.Fprintf(a.Out, "%s freed\n", humanSize(total))
	}
	return nil
}

// superseded returns the versions for which a newer release of the same
// major.minor is in the list: patches (1.26.5 when 1.26.9 is there) and
// pre-releases (1.27rc1 when 1.27.0 or 1.27rc2 is there).
func superseded(versions []string) []string {
	newest := map[string]semver.Version{}
	parsed := map[string]semver.Version{}
	for _, s := range versions {
		v, err := semver.Parse(s)
		if err != nil {
			continue
		}
		parsed[s] = v
		key := fmt.Sprintf("%d.%d", v.Major, v.Minor)
		if n, ok := newest[key]; !ok || semver.Compare(v, n) > 0 {
			newest[key] = v
		}
	}
	var out []string
	for _, s := range versions {
		v, ok := parsed[s]
		if !ok {
			continue
		}
		if semver.Compare(v, newest[fmt.Sprintf("%d.%d", v.Major, v.Minor)]) < 0 {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func diskUsage(path string) int64 {
	var n int64
	filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// --- gtr self list --remote --------------------------------------------------

func (a *App) listRemote(ctx context.Context, l home.Layout, tool string, all bool) error {
	c, err := config.Load(l)
	if err != nil {
		return err
	}
	if tool == "" || tool == "go" {
		releases, err := a.NewGo(l).Releases(ctx)
		if err != nil {
			return err
		}
		var versions []string
		for _, r := range releases {
			v := strings.TrimPrefix(r.Version, "go")
			if pv, err := semver.Parse(v); err == nil && (r.Stable || all || pv.Pre != "") {
				versions = append(versions, v)
			}
		}
		installed, err := goinstall.Installed(l)
		if err != nil {
			return err
		}
		a.printRemote("Go (go.dev)", remoteSelection(versions, all), installed, c.Go)
	}
	if tool == "" || tool == "gtr" {
		releases, err := a.NewSource(l, "gtr-manager", manager.Repo).Releases(ctx)
		if err != nil {
			return err
		}
		var versions []string
		for _, r := range releases {
			if _, err := semver.Parse(r.Version()); err == nil && (!r.Prerelease || all) {
				versions = append(versions, r.Version())
			}
		}
		installed, err := manager.Installed(l, a.exe("gtr-manager"))
		if err != nil {
			return err
		}
		a.printRemote("gtr-manager (GitHub releases of "+manager.Repo+")", remoteSelection(versions, true), installed, c.Gtr)
	}
	return nil
}

// remoteSelection sorts versions newest first. Without all, it keeps the
// newest patch of each minor version, and pre-releases only of a minor that
// has no stable release yet.
func remoteSelection(versions []string, all bool) []string {
	type pv struct {
		s string
		v semver.Version
	}
	var list []pv
	for _, s := range versions {
		if v, err := semver.Parse(s); err == nil {
			list = append(list, pv{s, v})
		}
	}
	sort.Slice(list, func(i, j int) bool { return semver.Compare(list[i].v, list[j].v) > 0 })
	var out []string
	seen, stable := map[string]bool{}, map[string]bool{}
	for _, p := range list {
		if p.v.Pre == "" {
			stable[fmt.Sprintf("%d.%d", p.v.Major, p.v.Minor)] = true
		}
	}
	for _, p := range list {
		key := fmt.Sprintf("%d.%d", p.v.Major, p.v.Minor)
		if !all {
			if p.v.Pre != "" && stable[key] || seen[key] {
				continue
			}
			seen[key] = true
		}
		out = append(out, p.s)
	}
	return out
}

func (a *App) printRemote(title string, versions, installed []string, def string) {
	fmt.Fprintf(a.Out, "%s:\n", title)
	if len(versions) == 0 {
		fmt.Fprintln(a.Out, "  none")
		return
	}
	for _, v := range versions {
		mark := "  "
		switch {
		case v == def:
			mark = "* "
		case contains(installed, v):
			mark = "i "
		}
		fmt.Fprintf(a.Out, "%s%s\n", mark, v)
	}
	fmt.Fprintln(a.Out, "(* default, i installed)")
}

// --- helpers ------------------------------------------------------------------

func dirNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if st, err := os.Stat(filepath.Join(dir, e.Name())); err == nil && st.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

func fileNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if st, err := os.Stat(filepath.Join(dir, e.Name())); err == nil && !st.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// sameContent reports whether two files are the same file or have the same
// bytes (a shim is a hard link to gtr or a copy of it).
func sameContent(a, b string) bool {
	if samePath(a, b) {
		return true
	}
	x, err1 := os.ReadFile(a)
	y, err2 := os.ReadFile(b)
	return err1 == nil && err2 == nil && string(x) == string(y)
}
