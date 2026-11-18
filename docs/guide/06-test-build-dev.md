# 6. Test, build, dev

## Tests

```bash
gtr test                          # go test ./... with the project's Go
gtr test --run TestLogin -v       # one test, verbose; more go test flags after --
gtr test --integration            # also files with //go:build integration
gtr test --e2e                    # also //go:build e2e (--unit: no tags, the default)
gtr test --coverage               # coverage per package; build/coverage.out
gtr test --coverage --html        # and build/coverage.html
gtr test --watch                  # again on every change
gtr test -r                       # every workspace member, dependencies first
```

`-race` is on when cgo and a C compiler are available (`--race`/`--no-race` to choose).
Settings in `gtr.json`:

```json
"test": { "coverage": 70, "race": true, "timeout": "5m" }
```

With `test.coverage`, `gtr test --coverage` fails below that percentage (`--min 80` for
one run). Coverage counts the project's own packages, not `gtr_modules/`.

Plain `go test ./...` works too (with the [shims](01-install.md#the-shims), even with the
right Go version); `gtr test` adds the tags, the coverage table and the threshold.

## Build

`gtr build` builds the binary described by the `build` block:

```json
"build": {
    "entry": "./cmd/app",
    "output": "app",
    "ldflags": "-s -w -X main.Version={{version}} -X main.Commit={{commit}} -X main.Date={{date}}",
    "tags": ["netgo"],
    "targets": ["linux/amd64", "linux/arm64", "windows/amd64", "darwin/arm64"],
    "files": ["README.md", "LICENSE", "config.example.yml"]
}
```

```bash
gtr build                          # this platform: build/<version>/<os>-<arch>/app
gtr build -t linux/arm64           # another platform (repeat -t, or a comma-separated list)
gtr build --all-targets            # build.targets (or the six usual platforms)
gtr build --all-targets --archive  # + app_<version>_<os>_<arch>.zip|tar.gz and SHA256SUMS
gtr build --version 1.2.0-rc.1     # another version than gtr.json's
```

- `{{version}}`, `{{commit}}` (12 characters of the git commit), `{{date}}` (UTC; set
  `SOURCE_DATE_EPOCH` for reproducible builds) and `{{name}}` are filled in `ldflags`.
- Builds use `CGO_ENABLED=0` and `-trimpath` unless `"cgo": true` / `"trimpath": false`.
  Without cgo, cross-compiling needs nothing but Go.
- Targets build in parallel. Archives hold the binary and `files` (default `README*`,
  `LICENSE*`) — the layout to attach to a GitHub release.
- In a workspace root, `gtr build` builds every member with a `build` block.

```go
package main

var Version, Commit, Date = "dev", "", ""   // set by ldflags
```

## Dev: reload on change

```bash
gtr dev                    # build, run, rebuild and restart on every change
gtr dev -- --port 8080     # arguments for the program
```

`gtr dev` watches the Go files, `gtr.json` and `.env` of the package and of every
workspace member, and loads `.env` like scripts do. A failed build keeps the old process
running and prints the error. In a web application with fullstack-bundler, the browser
also reloads by itself and CSS changes apply without a reload ([Web applications](12-web-apps.md)).

Next: [Go versions](07-go-versions.md).
