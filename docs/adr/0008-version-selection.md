# ADR-0008. Version selection, gtr-manager releases and command forwarding

Status: **Proposed** · 2026-10-09

## Context

`gtr` is the launcher; `gtr-manager` implements project commands (ADR-0006). A
project must build with the Go version it declares, and a team must run the same
gtr-manager, without anyone switching versions by hand. gtr-manager had no
releases, so there was nothing for the launcher to install.

## Decision

1. **Ranges** use the ADR-0004 syntax for both tools (`1.26.9`, `1.26`, `^1.22`,
   `~1.26`, `>=1.26 <1.28`, `*`). Comparators with partial versions follow npm:
   `<=1.26` includes all of 1.26.x, `>1.26` starts at 1.27.0, `=1.26` is 1.26.x.
   Go release names such as `1.27rc1` are understood; pre-releases are picked only
   when the range names one.

2. **Go is selected** for a directory, highest priority first:
   1. `GTR_GO_VERSION`;
   2. the nearest `gtr.json` upwards: `engines.go` (the legacy `engine` field
      written by older `gtr-manager init` is read too);
   3. the global default (`gtr self use go <version>`, stored in `~/.gtr/config.json`).

   With no selection, the system Go from `PATH` is left alone.

3. **gtr-manager is selected**:
   1. a local build linked with `gtr self link gtr <path>` (development, e.g. the
      submodule);
   2. `GTR_MANAGER_VERSION`;
   3. `engines.gtr` in the nearest `gtr.json`;
   4. the global default (`gtr self use gtr`);
   5. the latest release.

4. **A range resolves to the highest installed version** that satisfies it. If none
   is installed, the newest matching release is installed automatically, with a
   message saying which file required it. `gtr self current` shows the result and
   the reason.

5. **Project commands are forwarded**: `gtr new|init|install|uninstall|run|start …`
   runs the selected gtr-manager with the same arguments. When a Go version is
   selected, the child gets `PATH=<goroot>/bin:…`, `GOROOT`, `GTR_GO_VERSION=<exact>`
   (nested calls keep the same Go) and `GOTOOLCHAIN=local` unless already set (Go
   must not download a different toolchain behind gtr's back). The exit code of the
   child is the exit code of gtr.

6. **Shims**: `gtr self shims` places `go` and `gofmt` in `~/.gtr/bin` (hard links
   to, or copies of, gtr). Started under those names, gtr runs that tool of the
   selected Go version. Putting `~/.gtr/bin` first in `PATH` makes plain `go` follow
   the project. With no version selected, the shim runs the next `go` in `PATH`
   (skipping itself), so installing shims never breaks Go outside projects. The
   shim does not ignore Ctrl+C (an ignored signal would be inherited by `go test`);
   a child killed by a signal makes gtr exit with 128+signal. `gtr self update`
   refreshes existing shims.

7. **Releases** of gtr and gtr-manager are GitHub releases built by `release.sh`
   on a `v*` tag:
   ```
   <app>_<version>_<os>_<arch>.zip       Windows
   <app>_<version>_<os>_<arch>.tar.gz    other systems
   SHA256SUMS                            "<sha256>  <file>" per line
   ```
   gtr refuses to install an archive that is not listed in `SHA256SUMS` or whose
   hash differs. A GitHub release flagged "pre-release" is used only when requested
   by its exact version. Releases come from `shibisty/gtr-manager` and
   `shibisty/gtr`; `GITHUB_TOKEN` raises the API rate limit and is sent only to the
   API host itself. `GTR_GITHUB_API` points at GitHub Enterprise or a mirror, with
   its own `GTR_GITHUB_TOKEN`.

   Parallel gtr processes may install the same version at once (shims make that
   likely): a complete installation is never deleted or replaced; the process that
   loses the race uses the winner's copy.

8. **Self-update** replaces the running executable by renaming it to `gtr.old`
   (allowed on Windows even while running) and moving the new binary into place;
   `gtr.old` is removed on the next start.

## Consequences

- A fresh machine needs only `gtr`: the first project command installs the Go and
  gtr-manager versions the project asks for.
- Until gtr-manager has published releases, developers use `gtr self link gtr`.
- After rebuilding gtr locally, run `gtr self shims` again: hard-linked shims keep
  pointing at the old file.
- A GitHub release is the trust root for binaries; signing (e.g. Sigstore) can be
  added on top of `SHA256SUMS` later without changing the layout.
