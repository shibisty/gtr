# GTR

`gtr` is the launcher of the GTR toolchain. It installs and selects Go and
[gtr-manager](gtr-manager) versions per project and forwards project commands
(`new`, `init`, `install`, `run`, …) to gtr-manager, which is included here as a git
submodule.

```bash
git clone --recurse-submodules https://github.com/shibisty/gtr.git
# already cloned without submodules:
git submodule update --init
```

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

Priority for Go: `GTR_GO_VERSION` → `engines.go` of the nearest `gtr.json` → the
global default (`gtr self use go`). For gtr-manager: a linked local build →
`GTR_MANAGER_VERSION` → `engines.gtr` → the global default → the latest release.

Version syntax: `1.26.9` (exact), `1.26` (latest 1.26.x), `^1.22`, `~1.26`,
`">=1.26 <1.28"`, `*`. Go pre-releases (`1.28rc1`) only when named explicitly.

## Commands

```bash
gtr self install go 1.26.9    # or 1.26, ^1.22; without a version: what gtr.json needs
gtr self install gtr 0.2
gtr self use go 1.27          # global default outside projects
gtr self list                 # installed versions (* = default)
gtr self uninstall go 1.25.4
gtr self link gtr ./gtr-manager/build/gtr-manager   # develop gtr-manager
gtr self shims                # go/gofmt in ~/.gtr/bin follow the project
gtr self update               # update gtr itself
```

Run `gtr help` for the full list.

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

### Environment

| Variable | Meaning |
|---|---|
| `GTR_HOME` | Home directory, default `~/.gtr` (`%USERPROFILE%\.gtr`) |
| `GTR_GO_VERSION` | Go version or range, overrides the project |
| `GTR_MANAGER_VERSION` | gtr-manager version or range, overrides the project |
| `GITHUB_TOKEN` | Raises the GitHub API rate limit, private repositories |
| `GTR_GITHUB_API` | GitHub Enterprise or a mirror of the GitHub API |
| `GTR_GITHUB_TOKEN` | Token for `GTR_GITHUB_API` (`GITHUB_TOKEN` is never sent to it) |

Child processes get `GOROOT`, `PATH`, `GTR_GO_VERSION` and `GOTOOLCHAIN=local`
(unless you set `GOTOOLCHAIN` yourself), so Go never downloads another toolchain
behind gtr's back.

## Layout

```
~/.gtr/
  go/<version>/          installed Go
  gtr/<version>/         installed gtr-manager
  bin/                   shims
  cache/downloads/       archives (safe to delete)
  tmp/                   unpacking
  config.json            global defaults and links
```

Go comes from go.dev, gtr and gtr-manager from GitHub releases; every archive is
verified by SHA-256 before it is unpacked.

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

Releases: pushing a tag `v1.2.3` runs `release.workflow.yml` (move it to
`.github/workflows/release.yml`), which builds archives with `./release.sh` and
publishes them with `SHA256SUMS`.

Design decisions: [docs/adr](docs/adr/README.md).
