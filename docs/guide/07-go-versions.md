# 7. Go versions

## Per project

```json
"engines": { "go": ">=1.22", "gtr": "^0.2" }
```

Every gtr command in the project (or a subfolder) runs with the newest **installed** Go
that `engines.go` allows, and installs one when none fits. The same goes for gtr-manager
and `engines.gtr`. `engines.go` also becomes the `go` line of the generated `go.mod`.

```bash
gtr self current          # which Go and gtr-manager apply here, and why
gtr env                   # the same plus paths: GOROOT, the store, gtr_modules (--json)
```

The order gtr decides in, first match wins:

| | Go | gtr-manager |
|---|---|---|
| 1 | `gtr --go 1.25 <command>` or `GTR_GO_VERSION` | a linked build (`gtr self link gtr <path>`) |
| 2 | `engines.go` of the nearest `gtr.json` (in a workspace member: at least the workspace's `go` line) | `gtr --gtr 0.2 <command>` or `GTR_MANAGER_VERSION` |
| 3 | the global default (`gtr self use go`) | `engines.gtr` |
| 4 | the next `go` in `PATH` | the global default, then the latest release |

Version syntax: `1.26.9` (exact), `1.26` (latest 1.26.x), `^1.22`, `~1.26`,
`">=1.26 <1.28"`, `*`. Pre-releases (`1.28rc1`) only when named.

A library supports the oldest Go its users have: write `>=1.22` and test 1.22 in CI
(ADR-0007). An application can ask for exactly what it is tested with: `"go": "1.26"`.

## Managing versions

```bash
gtr self install go 1.26          # or 1.26.9, ^1.22; no version: what gtr.json needs
gtr self install gtr 0.2
gtr self use go 1.27              # the default outside projects
gtr self list                     # installed (* = default)
gtr self list --remote            # available (--all: every patch)
gtr self uninstall go 1.25.4
gtr self prune --dry-run          # what prune would remove
```

Go comes from go.dev (`GTR_GO_MIRROR` for a mirror such as https://golang.google.cn/dl/),
gtr-manager from GitHub releases; every archive is checked against its SHA-256, and a
broken download resumes.

## What gtr sets for the commands it runs

`GOROOT` and `PATH` of the selected Go, `GTR_GO_VERSION`, and `GOTOOLCHAIN=local`, so the
go command never downloads another toolchain behind gtr's back. In a project with a
generated `go.mod` also `GOPROXY=off`, `GOWORK=off` (or `GOWORK=<root>/go.work` in a
workspace) and `GOFLAGS=-mod=mod`: dependencies come only from `gtr_modules/`.

Do not set `GOROOT` globally; `gtr doctor` warns about it.

## The go shim

With `~/.gtr/bin` first in `PATH`, plain `go` and `gofmt` are gtr: in a project they run the
project's Go with the settings above, outside a project the global default, and without
one your system Go. Editors that run `go` (gopls, the Go extension of VS Code, GoLand with
"go" from `PATH`) then follow each project automatically.

Next: [workspaces and local copies](08-workspaces.md).
