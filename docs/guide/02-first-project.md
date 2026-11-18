# 2. Your first project

## Create it

```bash
gtr new hello
cd hello
```

`gtr new` creates the directory and asks a few questions — the package name, the version,
your name and website — with defaults in brackets; `-y` accepts all of them. The name
becomes the import path of the project's packages, so it is lowercase letters, digits and
single hyphens (`hello`, `my-app`; ADR-0002).

The project comes from a template (`-t`, default `app`):

| Template | What you get |
|---|---|
| `app` | `main.go` with a test; scripts `start`, `build`, `test` |
| `cli` | a command-line tool: flags, a version set at build time, a testable `run()` |
| `lib` | a library: a package, a test, an example and a `CHANGELOG.md`; version 0.1.0 |
| `fullstack` | a web application: routes, views, SQLite, users with login, migrations ([Web applications](12-web-apps.md)) |
| `orm-driver` | an orm database driver with the shared driver test suite |
| `none` | `gtr.json` and `.gitignore` only |

`gtr init` does the same in the current directory instead of a new one.

## What is inside

```
hello/
  gtr.json        the manifest: name, version, Go version, dependencies, scripts
  main.go
  main_test.go
  README.md
  .gitignore      gtr_modules/, go.mod, go.work
```

```json
{
    "name": "hello",
    "version": "1.0.0",
    "engines": { "go": ">=1.22" },
    "dependencies": {},
    "scripts": {
        "build": "go build -o build/hello .",
        "start": "go run .",
        "test": "go test ./..."
    }
}
```

There is no `go.mod` yet: gtr writes it from `gtr.json`.

## Install, run, test

```bash
gtr install       # writes go.mod and gtr.lock (and installs dependencies, none yet)
gtr run           # the "start" script: go run .   →  Hello, world!
gtr test          # go test ./...
```

Run `gtr install` once after creating or cloning a project, and again whenever
`gtr.json` changes by hand. `gtr add` and `gtr remove` install by themselves.

`gtr run <name>` runs any script of `gtr.json`; `gtr start` is `gtr run start`, and a
script whose name is not a gtr command runs directly (`gtr build` is the built-in command,
`gtr migrate` runs the `migrate` script). [Scripts](05-scripts.md) has the rest.

## Add a package

```bash
gtr add github:shibisty/faker.go     # from GitHub, by the newest tag
gtr add faker                        # the same from a gtr registry, by name
```

```go
package main

import (
    "fmt"

    "faker"
)

func main() {
    f := faker.New()
    fmt.Println("Hello,", f.Person.FullName())
}
```

`gtr add` finds the package and its newest version, writes it to `gtr.json` under its name
(`"faker": "github:shibisty/faker.go#^0.1"`, or `"faker": "^0.1"` from a registry), the exact
version to `gtr.lock`, links it into `gtr_modules/` and regenerates `go.mod`. You import it
by its name. [Dependencies](04-dependencies.md) explains sources and versions.

## Build

```bash
gtr run build                     # the script: build/hello
gtr build                         # the built-in command: build/1.0.0/linux-amd64/hello
gtr build -t windows/amd64        # cross-compile
```

The built-in `gtr build` reads the `build` block of `gtr.json` (entry, ldflags with the
version and commit, targets, archives) — see [Test, build, dev](06-test-build-dev.md).

## Your editor

Editors read the generated `go.mod` (and `go.work` in a workspace), so completion and
go-to-definition work for packages in `gtr_modules/`. Open the project folder after
`gtr install`. If the editor runs its own `go`, install the [shims](01-install.md#the-shims)
so it uses the project's Go version.

## Commit

```bash
git init && git add . && git commit -m "hello"
```

Commit `gtr.json` and `gtr.lock`. `gtr_modules/` and `go.mod` are generated and already
in `.gitignore`. Someone who clones the project runs `gtr install` (or `gtr ci` for exactly
the locked versions) and gets the same packages.

Next: [gtr.json and the generated files](03-project-files.md).
