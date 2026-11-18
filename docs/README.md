# gtr documentation

gtr is to Go what npm is to Node: a project declares its dependencies and scripts in
`gtr.json`, `gtr install` puts them in place, and packages are imported by short names
(`import "orm"`). `go.mod` is generated, so the go command, gopls and IDEs keep working.
gtr also installs and selects the Go version each project asks for.

## Guide

Read in order the first time; each page stands on its own later.

1. [Install gtr](guide/01-install.md) — the installers, `gtr doctor`, the shims
2. [Your first project](guide/02-first-project.md) — `gtr new`, run, test, build
3. [gtr.json and the generated files](guide/03-project-files.md) — what is in a project and what to commit
4. [Dependencies](guide/04-dependencies.md) — adding, versions, the lock file, updating, removing
5. [Scripts and .env](guide/05-scripts.md) — `gtr run`, pre/post scripts, environment files
6. [Test, build, dev](guide/06-test-build-dev.md) — tests with coverage, cross-compiling, reload on change
7. [Go versions](guide/07-go-versions.md) — `engines`, `gtr self`, the `go` shim
8. [Workspaces and local copies](guide/08-workspaces.md) — several packages developed together
9. [Publishing a package](guide/09-publishing.md) — versions, changelog, the registry, git tags
10. [CI](guide/10-ci.md) — GitHub Actions
11. [An existing Go project](guide/11-existing-projects.md) — moving a go.mod project to gtr
12. [Web applications](guide/12-web-apps.md) — the `fullstack` template
13. [Claude](guide/13-claude.md) — `gtr mcp` for Claude Code and Claude Desktop
14. [Troubleshooting](guide/14-troubleshooting.md) — common errors and what to do

## Reference

- [Commands](reference/commands.md)
- [gtr.json](reference/gtr-json.md)
- [Environment variables and files](reference/environment.md)
- [Design decisions (ADR)](adr/README.md)
