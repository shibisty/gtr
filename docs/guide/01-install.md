# 1. Install gtr

gtr is one binary, `gtr`. It downloads everything else it needs — Go versions and
gtr-manager, the package manager it runs project commands with — into its home directory
`~/.gtr` (`%USERPROFILE%\.gtr` on Windows).

## The installers

```bash
# Linux, macOS
curl -fsSL https://raw.githubusercontent.com/shibisty/gtr/main/install.sh | sh
```

```powershell
# Windows (PowerShell)
irm https://raw.githubusercontent.com/shibisty/gtr/main/install.ps1 | iex
```

The installer downloads the latest release for your system, checks it against the
release's `SHA256SUMS`, puts `gtr` into `~/.gtr/bin` together with `go` and `gofmt` shims,
and adds that directory to the front of your `PATH`. Open a new terminal afterwards.

| Setting | Effect |
|---|---|
| `GTR_VERSION=0.2.0` | install this version instead of the latest |
| `GTR_HOME=D:\gtr` | another home directory |
| `GTR_NO_SHIMS=1` | no `go`/`gofmt` shims |
| `GTR_NO_MODIFY_PATH=1` | leave `PATH` alone |

For example `GTR_VERSION=0.2.0 sh -c "$(curl -fsSL …/install.sh)"`, or in PowerShell
`$env:GTR_VERSION = "0.2.0"` before running the installer.

## Check the setup

```bash
gtr version     # gtr 0.2.0
gtr doctor      # the installation, PATH and shims, the project, the network
```

`gtr doctor` says what is wrong and how to fix it: `~/.gtr/bin` missing from `PATH` or
another `go` before it, shims from an older gtr build, an interrupted Go install, a global
`GOROOT` or `GOTOOLCHAIN` that fights gtr's choice, no access to GitHub or go.dev.
`--offline` skips the network checks.

> **Several `gtr` binaries.** If `gtr version` shows an old version after an update, another
> `gtr` comes first in `PATH`. `where gtr` (Windows) or `which -a gtr` shows all of them;
> keep `~/.gtr/bin` and remove the others.

## Go

You do not need Go installed. The first command that needs Go installs the version the
project asks for (`"engines": {"go": ">=1.22"}` in `gtr.json`) from go.dev into
`~/.gtr/go/<version>`. Outside a project gtr uses the global default, which you set with:

```bash
gtr self install go 1.26      # the newest 1.26.x
gtr self use go 1.26          # the default outside projects
gtr self list                 # installed versions (* marks the default)
```

A Go you already have keeps working: without a default, the shims fall back to the next
`go` in `PATH`. [Go versions](07-go-versions.md) has the details.

## The shims

`~/.gtr/bin/go` and `gofmt` are small copies of gtr that run the right Go for the directory
you are in. With them, `go build`, `go test` and your editor's Go tools use the project's Go
version and its generated `go.mod` without any setup. Recreate them after building gtr
yourself:

```bash
gtr self shims
```

## Updating and removing

```bash
gtr self update               # the latest gtr (gtr self update 0.3.0: a given version)
gtr self prune                # remove downloaded archives and superseded patch versions
gtr self uninstall go 1.25.4
```

To remove gtr, delete `~/.gtr` and take `~/.gtr/bin` out of `PATH`.

## From source

```bash
git clone --recurse-submodules https://github.com/shibisty/gtr.git
cd gtr
go build -o ~/.gtr/bin/gtr ./cmd/gtr          # Windows: -o %USERPROFILE%\.gtr\bin\gtr.exe
gtr self shims
```

gtr-manager is the `gtr-manager` submodule. To use your own build of it instead of the
releases:

```bash
cd gtr-manager && go build -o build/gtr-manager ./cmd/gtr-manager
gtr self link gtr ./build/gtr-manager        # gtr self unlink gtr: back to releases
```

Next: [your first project](02-first-project.md).
