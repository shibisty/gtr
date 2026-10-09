# ADR-0006. Repository layout

Status: **Accepted** · 2026-10-09

## Context

- gtr and gtr-manager are two separate projects with identical `archive`, download
  and build scripts.
- orm.go: `.git` lives inside `core/` (remote `shibisty/orm.go`) and inside
  `drivers/mysql/` (remote `shibisty/orm.go-mysql-driver`), while the
  `orm.go` directory itself is not a repository.
- passport.go contained a full copy of orm under the same module path (removed 2026-10-09).

## Decision

1. **gtr and gtr-manager are two repositories; gtr-manager is included in gtr
   as a submodule.** `git clone --recurse-submodules` fetches both projects at once:
   ```
   gtr/                  shibisty/gtr — the launcher (Go and gtr-manager versions, command proxy)
     cmd/gtr/, internal/, docs/adr/, schema/
     gtr-manager/        submodule shibisty/gtr-manager — the package manager
   ```
   - Each has its own `go.mod`, history, tags and releases; the nested `go.mod`
     excludes `gtr-manager/` from the `gtr` module.
   - The submodule pins the gtr-manager version the launcher was tested with. To
     update it, commit to gtr-manager, then commit the new pointer in gtr.
   - gtr CI checks out with `submodules: true` and tests both projects together.
   - Shared code (`archive`, downloading, build scripts) is duplicated for now. If
     the copies start to diverge, extract them into a third module rather than merging the repositories.

2. **Every package is a separate repository.** The core and every driver, strategy
   and session store live in their own repositories with their own history, `v<version>`
   tags and CI. The repository naming convention follows the existing
   `orm.go-mysql-driver`:

   | gtr package | Repository |
   |---|---|
   | `orm` | `shibisty/orm.go` |
   | `orm-mysql`, `orm-postgres`, `orm-sqlite`, `orm-mongodb`, `orm-redis` | `shibisty/orm.go-<name>-driver` |
   | `passport` | `shibisty/passport.go` |
   | `passport-oauth2`, `passport-local`, `passport-google`, `passport-facebook` | `shibisty/passport.go-<name>-strategy` |
   | `passport-session-redis` | `shibisty/passport.go-redis-session` |
   | `migration` | `shibisty/migration.go` |
   | `migration-orm` | `shibisty/migration.go-orm-driver` |
   | `faker` | `shibisty/faker.go` |

   The package name is set in its `gtr.json`, not by the repository name (ADR-0003, item 1),
   so the repository convention can change without affecting code.

3. **Local development of a family uses an uncommitted workspace.**
   The `orm.go/` directory on the developer's machine is not a repository but the root
   of a local workspace:
   ```
   C:\OSPanel6\home\orm.go\
     gtr.json              {"name":"orm-dev","private":true,"workspaces":["core","drivers/*"]}
     core\                 clone of shibisty/orm.go
     drivers\mysql\        clone of shibisty/orm.go-mysql-driver
     drivers\postgres\     …
   ```
   Inside the workspace, drivers see the local core, so core changes are immediately
   tested by all drivers. The layout already works this way today; the only change is
   that each subdirectory becomes its own repository.

   *Implementation (stage 4, ADR-0009):* a member's committed `gtr.json` names the core
   by its real source (`"orm": "github:shibisty/orm.go"` as a devDependency, `"orm":
   "^0.1"` as a peer), and the workspace uses the member in place. Families that depend
   on another family (passport-session-redis and migration-orm on orm) take its packages
   from the sibling folder through `file:` dependencies of their workspace root
   (`"orm": "file:../orm.go/core"`), which override the source for the whole workspace.
   Apps and tests that never leave the workspace (`example`, `drivers/sqlite/integration`)
   are members too.

4. **Compatibility with the core** is maintained through `peerDependencies` and a shared
   test suite: the core publishes the `orm/drivertest` package, and every driver runs it
   in its own CI against its own database version. When a new minor version of the core is released,
   the core's CI runs the driver tests from the workspace.

5. **passport.go** follows the same structure: `core/` (`passport`), `strategies/<name>/`,
   `sessions/redis/`, `example/`. The copy of orm was removed, and `passwordhash` moved from orm
   to `passport/passwordhash`. `passport-session-redis` depends on the `orm` core
   (the `orm.KeyValueStore` interface) rather than on `orm-redis`: any key-value
   store will do; `orm-redis` is only needed for its integration test. Session stores
   are tested by the shared `passport/sessionstoretest` suite, OAuth2 strategies by
   the fake provider `passport-oauth2/oauth2test`. **faker.go** is a single package. **fullstack.go** will be decided
   in stage 8.

## Consequences

- There must be a way to develop a change to the core and drivers together: the local
  workspace (item 3) is a required part of stage 4, and until then `file:` sources are used.
- Shipping a change to the core interface takes several releases in different repositories,
  in order: core → drivers.
- gtr needs a git repository `shibisty/gtr` (the directory is not under git yet);
  gtr-manager stays `shibisty/gtr-manager`.
- The existing `.git` directories in `orm.go/core` and `orm.go/drivers/mysql` remain;
  repositories for the other drivers, strategies and stores are created in stage 1.
