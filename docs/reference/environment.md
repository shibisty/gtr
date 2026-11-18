# Environment variables and files

## gtr

| Variable | Meaning |
|---|---|
| `GTR_HOME` | the home folder (default `~/.gtr`, `%USERPROFILE%\.gtr`) |
| `GTR_GO_VERSION` | Go version or range for this command, over `engines.go` (also `gtr --go`) |
| `GTR_MANAGER_VERSION` | gtr-manager version or range (also `gtr --gtr`) |
| `GTR_GO_MIRROR` | a mirror of https://go.dev/dl/ |
| `GTR_OFFLINE=1` | only what the store and caches have |
| `GTR_MCP=1` | set by `gtr mcp` for the commands it runs |

## Sources

| Variable | Meaning |
|---|---|
| `GITHUB_TOKEN` | GitHub: private repositories, higher API limits (only sent to api.github.com) |
| `GTR_GITHUB_API`, `GTR_GITHUB_TOKEN` | GitHub Enterprise or a mirror of the API, and its token |
| `GTR_GITLAB_URL`, `GTR_GITLAB_TOKEN` (`GITLAB_TOKEN`) | GitLab server and token |
| `GTR_GOPROXY` | Go module proxy (default: the first URL of `GOPROXY`, else proxy.golang.org) |
| `GTR_GOPROXY_PRIVATE` | proxy for `GOPRIVATE` / `GONOPROXY` modules |
| `GTR_GOSUMDB` | checksum database URL or `off` (default from `GOSUMDB`) |
| `GOPRIVATE`, `GONOSUMDB` | modules not looked up publicly |
| `GTR_NPM_REGISTRY`, `GTR_NPM_TOKEN` | npm registry and its token |
| `GTR_REGISTRY` | the gtr registry (default: the last `gtr login`, else https://gtr.shibisty.com; a local one is http://localhost:4873) |
| `GTR_REGISTRY_TOKEN` | registry token (CI) |

## Set by gtr for the commands it runs

`GOROOT`, `PATH` (the selected Go first), `GTR_GO_VERSION`, `GOTOOLCHAIN=local` (unless you
set it). In a project with a generated `go.mod`: `GOPROXY=off`, `GOWORK=off` or the
workspace's `go.work`, `GOFLAGS=-mod=mod`. Scripts and `gtr dev` also get `.env` and
`.env.local`.

## Files

| Path | What it is |
|---|---|
| `~/.gtr/bin/` | `gtr` and the `go`/`gofmt` shims |
| `~/.gtr/go/<version>/` | installed Go versions |
| `~/.gtr/gtr/<version>/` | installed gtr-manager versions |
| `~/.gtr/store/` | packages, one immutable folder per version (`gtr/`, `go/`, `npm/`) |
| `~/.gtr/cache/` | downloads (safe to delete) |
| `~/.gtr/config.json` | default versions, the linked gtr-manager |
| `~/.gtr/links.json` | packages registered with `gtr link` |
| `~/.gtr/registry.json` | the registry and its tokens (readable only by you) |
| project `gtr.json`, `gtr.lock` | the manifest and the lock (commit both) |
| project `go.mod`, `go.work`, `gtr_modules/`, `node_modules/`, `.gtr/` | generated, ignored by git |
| project `.env`, `.env.local` | variables for scripts and `gtr dev` |
