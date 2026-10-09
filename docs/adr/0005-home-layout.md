# ADR-0005. The ~/.gtr home directory and the store

Status: **Accepted** · 2026-10-09

## Context

Currently `gtr self install go` writes to `./go` and `./cache` relative to the current
directory, and gtr-manager keeps its cache next to its executable. As a result, 32k Go
files ended up in `gtr/build/`, and `build.ps1` deletes them along with the build output.

## Decision

A single home directory, `~/.gtr` by default (`%USERPROFILE%\.gtr` on Windows),
overridden by the `GTR_HOME` variable:

```
~/.gtr/
  bin/                      shims: gtr, go, gofmt → the right version for the project
  go/<version>/             installed Go versions: go/1.26.5/bin/go
  gtr/<version>/            installed gtr-manager versions
  store/
    gtr/<name>@<version>-<hash8>/      gtr packages (with a generated go.mod)
    go/<module>@<version>/             external Go modules
  cache/
    downloads/              Go, gtr and package archives (safe to delete)
    index/                  cache of version and tag lists (10-minute TTL)
  tmp/                      extraction; moved into the store with an atomic rename
  config.json               global versions, registries, proxies, tokens by env reference
```

Rules:

1. **The store is immutable.** A package directory appears in full via `rename` from
   `tmp/` after the integrity check (ADR-0004) and never changes afterwards.
   If something on disk was edited, `gtr doctor` detects the hash mismatch.
2. **`gtr_modules/` in a project consists of links into the store**: symlinks on Linux/macOS,
   junctions on Windows (no administrator rights required). If links are
   unavailable (network drive, FAT), a copy is made with a warning.
3. **Nothing is written outside `~/.gtr` and the project directory.** An e2e test checks this.
4. Secrets (`GITHUB_TOKEN` and the like) are not stored in `config.json`, only the name
   of the environment variable.
5. `gtr self prune` removes `cache/` and store packages not referenced by
   any known project (gtr keeps a list of projects in `config.json`).

## Consequences

- gtr builds and installed Go versions no longer overlap.
- A given version of a package is stored on disk once for all projects (like pnpm).
- Junctions and path lengths on Windows must be checked at the start of stage 3.
