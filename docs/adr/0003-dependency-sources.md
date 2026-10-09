# ADR-0003. Package sources and dependency entry format

Status: **Accepted** · 2026-10-09

## Context

gtr packages live on GitHub for now; a registry will come later (stage 6). Drivers
need regular Go modules: `go-sql-driver/mysql`, `pgx`, `go-redis`,
`mongo-driver`. Today three places use three formats: `"repo/router"`,
`"github:golang/example"` (gtr-manager README) and
`"github.com/shibisty/faker.go"` (fullstack.go/package.json). We need one.

## Decision

**The key in `dependencies` is always the import path; the value says where it comes from
and which version.** The dependency type is determined by the key:

- a key without a dot → a gtr package (`orm`, `orm-mysql`), ADR-0002;
- a key with a dot in the first element → an external Go module
  (`github.com/go-sql-driver/mysql`).

```json
"dependencies": {
  "orm": "^0.1",
  "orm-mysql": "github:shibisty/orm.go-mysql-driver#^0.1",
  "faker": "github:shibisty/faker.go#^0.1",
  "passport": "file:../passport.go",
  "github.com/go-sql-driver/mysql": "^1.10"
}
```

Value forms:

| Value | Source | When |
|---|---|---|
| `^0.1` | The gtr registry (for gtr packages) or proxy.golang.org (for Go modules) | The main form; for gtr packages, after stage 6 |
| `github:<owner>/<repo>[/<subdir>]#<range>` | GitHub, versions from git tags | Now, until the registry exists |
| `gitlab:…#<range>`, `git+https://host/repo.git[/<subdir>]#<range>` | Other git hosts | Later |
| `file:<path>` | A local directory, not copied | Developing several packages at once |
| `workspace:*`, `workspace:<range>` | A member of the same workspace (ADR-0009) | Now |

Rules:

1. The name in the downloaded package's `gtr.json` must match the key. Otherwise
   it is an error: this prevents substituting `orm` with someone else's repository.
2. For a monorepo subdirectory, the git tag is looked up with the package name as a prefix:
   `orm-mysql@0.1.0`, then `drivers/mysql/v0.1.0` (Go style). For the repository
   root: `v0.1.0` or `0.1.0`.
3. Go modules are downloaded from proxy.golang.org (configurable: a `GOPROXY`-compatible
   URL in `~/.gtr/config.json`), verified against sum.golang.org and
   extracted into the store. gtr resolves their own dependencies from `go.mod`
   the same way as its own (ADR-0004).

   *Implementation (stage 3):* the proxy comes from `GTR_GOPROXY` or `GOPROXY`;
   `go.mod` and zip hashes are compared with the checksum database's lookup records
   (`GTR_GOSUMDB`, `GONOSUMDB`, `GOPRIVATE`, `GOSUMDB=off`) and with `gtr.lock`. The
   transparency-log inclusion proof is not verified yet, so a compromised database
   server could go unnoticed on the first download; later downloads are pinned by
   `gtr.lock`. Every module of the build list is downloaded (not only those whose
   packages are imported), which keeps `go build`/`go test` working with
   `GOPROXY=off`. `go mod tidy` is not needed in a gtr project — `gtr install` plays its
   role — and may rewrite the requirements of external modules.
4. `gtr add <name>` without a source: until the registry exists, an error suggesting
   the `github:` form; after stage 6, the registry.

## Consequences

- One entry, one rule: the key always equals what goes into `import`.
- Moving from GitHub to the registry means replacing the value `github:…#^0.1` with `^0.1`;
  the code does not change.
- The old gtr-manager format (`github:user/repo` as the key) is migrated by
  `gtr install` with a warning (done in stage 3).
