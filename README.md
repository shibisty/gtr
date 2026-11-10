# GTR

`gtr` is the launcher of the GTR toolchain. It installs and selects Go and
[gtr-manager](gtr-manager) versions per project and forwards project commands
(`new`, `init`, `install`, `run`, …) to gtr-manager, which is included here as a git
submodule.

GTR is to Go what npm is to Node: dependencies are declared in `gtr.json`, installed with
one command and imported by short names (`import "orm"`), while `go.mod` is generated so
the Go toolchain and IDEs keep working.

[![Patreon](https://c5.patreon.com/external/logo/become_a_patron_button.png)](https://www.patreon.com/cw/shibisty)

## Install

```bash
# Linux, macOS
curl -fsSL https://raw.githubusercontent.com/shibisty/gtr/main/install.sh | sh
```

```powershell
# Windows (PowerShell)
irm https://raw.githubusercontent.com/shibisty/gtr/main/install.ps1 | iex
```

The installers download the latest release for your system, check it against
`SHA256SUMS`, put `gtr` into `~/.gtr/bin` with the `go`/`gofmt` shims and add that
directory to `PATH`. Settings: `GTR_VERSION=0.2.0` (a version instead of the latest),
`GTR_HOME`, `GTR_NO_SHIMS=1`, `GTR_NO_MODIFY_PATH=1`. Then check the setup:

```bash
gtr doctor
```

From source:

```bash
git clone --recurse-submodules https://github.com/shibisty/gtr.git
# already cloned without submodules:
git submodule update --init
```

## Quick start

```bash
gtr new blog -y && cd blog            # gtr.json, main.go with a test (templates: app, cli, lib, orm-driver)
gtr add github:shibisty/orm.go github:shibisty/orm.go-postgres-driver
gtr run                               # the "start" script: go run .
gtr run test
gtr list --tree                       # what is installed; gtr outdated: what is newer
```

```go
import (
    "orm"
    postgres "orm-postgres"
)
```

Packages, `gtr.json`, workspaces and the other project commands are described in
[gtr-manager/README.md](gtr-manager/README.md).

## Packages

| Package | Repository | What it is |
|---|---|---|
| `orm` | [shibisty/orm.go](https://github.com/shibisty/orm.go) | ORM with a schema builder; drivers `orm-mysql`, `orm-postgres`, `orm-sqlite`, `orm-mongodb`, `orm-redis` (`shibisty/orm.go-<name>-driver`) |
| `passport` | [shibisty/passport.go](https://github.com/shibisty/passport.go) | Authentication in the style of passport.js; strategies `passport-local`, `passport-oauth2`, `passport-google`, `passport-facebook`; sessions `passport-session-redis`, `passport-session-file` |
| `migration` | [shibisty/migration.go](https://github.com/shibisty/migration.go) | Laravel-style migrations; `migration-orm` runs them through orm |
| `faker` | [shibisty/faker.go](https://github.com/shibisty/faker.go) | Fake data with locales and deterministic seeds |

## How versions are chosen

A project declares what it needs in `gtr.json`:

```json
{
    "name": "blog",
    "engines": { "go": ">=1.26", "gtr": "^0.2" }
}
```

Any `gtr` command run inside the project (or a subdirectory) uses the highest
installed versions that match and installs missing ones automatically:

```bash
cd blog
gtr run test        # installs Go 1.27.x and gtr-manager 0.2.x if needed, then runs the script
gtr self current    # shows which versions apply here and why
```

Priority for Go: `gtr --go <version> <command>` or `GTR_GO_VERSION` → `engines.go` of the nearest `gtr.json` (in a
workspace member also at least the `go` line of the workspace's `go.work`) → the global
default (`gtr self use go`). For gtr-manager: a linked local build →
`gtr --gtr <version>` or `GTR_MANAGER_VERSION` → `engines.gtr` → the global default → the latest release.

Version syntax: `1.26.9` (exact), `1.26` (latest 1.26.x), `^1.22`, `~1.26`,
`">=1.26 <1.28"`, `*`. Go pre-releases (`1.28rc1`) only when named explicitly.

## Commands

```bash
gtr self install go 1.26.9    # or 1.26, ^1.22; without a version: what gtr.json needs
gtr self install gtr 0.2
gtr self use go 1.27          # global default outside projects
gtr self list                 # installed versions (* = default)
gtr self list --remote        # versions available to install (--all: every patch)
gtr self uninstall go 1.25.4
gtr self link gtr ./gtr-manager/build/gtr-manager   # develop gtr-manager
gtr self shims                # go/gofmt in ~/.gtr/bin follow the project
gtr self update               # update gtr itself
gtr self prune                # remove downloaded archives and superseded patch versions (--dry-run)

gtr env                       # Go, gtr-manager and paths used here; installs nothing (--json, or NAME...)
gtr doctor                    # check the installation, PATH and shims, the project, the network
```

Run `gtr help` for the full list. `gtr self prune` keeps the default versions and the ones
the current directory uses; a project that pins a removed patch gets it back on its next
command.

### Shims

After `gtr self shims`, put `~/.gtr/bin` first in `PATH`. Plain `go build`,
`go test` and `gofmt` then use the project's Go version; outside a project they use
the global default, and with no default the next `go` in `PATH` (your system Go). After rebuilding gtr from source, run `gtr self shims` again
(`gtr self update` does it for you).

In a project whose `go.mod` gtr generated, the `go` shim also sets `GOPROXY=off`,
`GOWORK=off` and `-mod=mod` (dependencies are installed by gtr, never by go) and follows
go errors with a hint, for example
``gtr: package "orm" is not installed: add it with `gtr add github:owner/repo` …``. When stderr is a terminal, `go run`, `go generate` and
`go tool` are left without hints, so your program still sees the terminal. In a
workspace (a `go.work` gtr generated, ADR-0009) the shim sets `GOWORK` to it instead.

### Claude (MCP)

`gtr mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) server on stdio,
so Claude Code and Claude Desktop work with a project through gtr instead of the bare go
command: they install packages, run scripts and tests and read `gtr.json` and `gtr.lock`,
and see the same hints as you do in a terminal ([ADR-0011](docs/adr/0011-mcp.md)).

```bash
claude mcp add gtr -- gtr mcp       # Claude Code, in the project
gtr mcp --print-config              # the JSON for .mcp.json or claude_desktop_config.json
gtr mcp bundle -o gtr.mcpb          # a Claude Desktop extension: open it in Claude Desktop
```

Or install the plugin (the server and a skill that tells Claude to use it):

```
/plugin marketplace add shibisty/gtr
/plugin install gtr@gtr
```

Tools: `project_info`, `list`, `why`, `outdated`, `search`, `info`, `env`, `doctor` (read
only); `add`, `remove`, `install`, `update`, `sync`, `link`, `unlink`, `version_bump`
(change the project: Claude asks first); `run`, `test`, `build` (time limit, exit code and
results per package); `publish` (a dry run unless the server runs with `--allow-publish`);
`self_install` (only with `--allow-self-install`). Resources: `gtr://gtr.json`,
`gtr://gtr.lock`, `gtr://schema`. The server works in the workspace root or the project
above the directory it starts in (`--root` to choose) and refuses directories outside it.

### Environment

| Variable | Meaning |
|---|---|
| `GTR_HOME` | Home directory, default `~/.gtr` (`%USERPROFILE%\.gtr`) |
| `GTR_GO_VERSION` | Go version or range, overrides the project |
| `GTR_MANAGER_VERSION` | gtr-manager version or range, overrides the project |
| `GITHUB_TOKEN` | Raises the GitHub API rate limit, private repositories |
| `GTR_GITHUB_API` | GitHub Enterprise or a mirror of the GitHub API |
| `GTR_GITHUB_TOKEN` | Token for `GTR_GITHUB_API` (`GITHUB_TOKEN` is never sent to it) |
| `GTR_GO_MIRROR` | A mirror of https://go.dev/dl/, e.g. https://golang.google.cn/dl/ |

Child processes get `GOROOT`, `PATH`, `GTR_GO_VERSION` and `GOTOOLCHAIN=local`
(unless you set `GOTOOLCHAIN` yourself), so Go never downloads another toolchain
behind gtr's back.

## Layout

```
~/.gtr/
  go/<version>/          installed Go
  gtr/<version>/         installed gtr-manager
  bin/                   shims
  store/gtr/             gtr packages, one immutable directory per version (<name>@<version>-<hash>)
  store/go/              external Go modules (<module>@<version>)
  cache/downloads/       archives (safe to delete)
  cache/download/go/     verified go.mod files of external modules (safe to delete)
  tmp/                   unpacking
  config.json            global defaults and links
```

Projects link packages from the store into their `gtr_modules/` (junctions on Windows),
so each version is downloaded once for all projects (ADR-0005).

Go comes from go.dev, gtr and gtr-manager from GitHub releases; every archive is
verified by SHA-256 before it is unpacked. A dropped download resumes where it stopped
(up to four attempts); on a terminal the download shows its progress.

## Development

```bash
go test ./...                       # gtr
cd gtr-manager && go test ./...     # gtr-manager
```

Updating the pinned gtr-manager version:

```bash
git submodule update --remote gtr-manager
git commit -am "Update gtr-manager"
```

CI: `.github/workflows/ci.workflow.yml`. gtr is built by gtr-manager, from the `build`
block of `gtr.json`: `gtr build` (this platform), `gtr build --all-targets` (every
platform into `build/<version>/<os>-<arch>/`), `gtr build --all-targets --archive` (the
release archives and `SHA256SUMS`). Without gtr installed, `make build`, `make all` and
`make release` build gtr-manager from the submodule first and do the same.

Releases: `gtr version patch` (bumps `gtr.json`, dates `CHANGELOG.md`, commits and tags
`v0.1.1`), then `gtr publish` (checks and pushes the tag). The tag runs
`.github/workflows/release.workflow.yml`, which builds the archives with gtr-manager from
the submodule and publishes them as the GitHub release. To try a gtr-manager change,
`go build -o build/gtr-manager ./cmd/gtr-manager` in `gtr-manager/` and
`gtr self link gtr ./gtr-manager/build/gtr-manager`.

`go test -short ./...` skips the end-to-end tests, which build gtr and run it against
fake servers (`cmd/gtr/e2e_test.go`, `cmd/gtr/install_script_test.go`).

Design decisions: [docs/adr](docs/adr/README.md).

[![Patreon](https://c5.patreon.com/external/logo/become_a_patron_button.png)](https://www.patreon.com/cw/shibisty)

If this project helps you, consider supporting its development on Patreon ❤️
