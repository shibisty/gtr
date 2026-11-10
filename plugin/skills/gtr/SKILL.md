---
name: gtr
description: Use in Go projects that have a gtr.json (gtr, the Go package manager) - adding, removing or updating dependencies, installing, running scripts, tests, builds, releases. go.mod there is generated, so use the gtr tools instead of go get, go mod tidy or editing go.mod.
---

# Working in a gtr project

A directory with `gtr.json` is a gtr project. gtr is to Go what npm is to Node:

- Dependencies live in `gtr.json` (`dependencies`, `devDependencies`) and exact versions
  in `gtr.lock`. `go.mod` and `gtr_modules/` are **generated** by gtr: never edit them,
  and never run `go get`, `go mod tidy` or `go mod edit` - the next `gtr install` would
  overwrite the change.
- The project's Go version is `engines.go` in `gtr.json`; gtr runs that Go (installing it
  if needed), not the Go on the PATH.

Use the tools of the `gtr` MCP server:

| Task | Tool |
| --- | --- |
| Understand the project (manifest, members, scripts, Go version) | `project_info` first |
| Installed packages, why one is there, newer versions | `list`, `why`, `outdated` |
| Add a dependency | `add` (`faker`, `faker@^0.2`, `github:owner/repo`, `file:../path`, `npm:preact`; `dev: true` for tests and tools) |
| Remove or update | `remove`, `update` |
| After pulling changes or editing gtr.json | `install` (`ci: true` for exactly gtr.lock) |
| Run tests | `test` (`kind`, `coverage`, `run`, `recursive` for workspaces) |
| Run a script from gtr.json | `run` (`dev` and `start` servers run until the time limit) |
| Build binaries | `build` |
| Find packages | `search`, `info` |
| Something is wrong with the setup | `doctor`, `env` |

In a workspace (`workspaces` in the root `gtr.json`), pass `dir` (such as `core`) to act on
one member; members depend on each other with `workspace:*`.

Errors of `go build` and `go test` come with gtr hints (a missing package, a wrong Go
version): read them before changing code. `publish` is a dry run unless the user allowed
publishing; bump versions with `version_bump` only when asked.
