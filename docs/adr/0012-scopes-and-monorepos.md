# ADR-0012. Scoped names and publishing monorepos

Status: **Accepted** · 2026-11-18 · amends [0002](0002-import-paths-and-names.md) and [0006](0006-repositories.md)

## Context

The package families (passport, orm, fullstack, migration) are developed as workspaces
([ADR-0009](0009-workspaces.md)), but every member lives in a repository of its own and
is published from it (`passport.go-facebook-strategy`, `orm.go-postgres-driver`). The
names repeat the family by convention only (`passport-facebook`), nothing stops anybody
from publishing `passport-something` to the registry, and releasing a family means a
commit, a tag and a `gtr publish` in every repository.

npm solves this with scopes (`@babel/core`) and workspaces that publish together.

## Decision

1. **Scoped names.** A package name is `name` or `@scope/name`. Each part follows the
   rules of [ADR-0002](0002-import-paths-and-names.md) (lowercase letters, digits, single
   hyphens, starting with a letter, at least 2 characters, not reserved); the whole name
   is at most 64 characters.
2. **Import path.** Go import paths cannot contain `@`, so a scoped package is imported
   without it: `@orm/postgres` is the module and import path `orm/postgres`. The family's
   core keeps its bare name (`orm`), so the code reads `import "orm"` and
   `import "orm/postgres"`. In `gtr.json` the key is the package name
   (`"@orm/postgres": "^0.1"`); `go.mod` and `go.work` have the import path.
3. **On disk.** Links are `gtr_modules/@orm/postgres` (a plain directory per scope), so a
   scoped package never lands inside the link of the bare package. Store directories
   are `@orm+postgres@0.1.0-<hash8>`. `gtr build` names a scoped program after its last
   part (`@acme/tool` builds `tool`).
4. **Command line.** `gtr add @orm/postgres`, `gtr add @orm/postgres@^0.1`,
   `gtr yank @orm/postgres@0.1.0`: the `@` at the start belongs to the name.
5. **Ownership in the registry.** A scope and the bare name of the same word belong to the
   same people. The first package of a scope (or the bare name) claims it; afterwards a
   new `@orm/…` package or the bare `orm` can only be created by an owner of `orm` or of
   any `@orm/…` package. Existing packages keep their own owners (`gtr owner add`).
   Registry URLs carry the name as one segment (`/api/v1/packages/@orm%2Fpostgres`); web
   pages are at `/packages/@orm/postgres`; luna-pages entries get the slug `orm/postgres`.
6. **Monorepos.** One repository holds a family as a workspace. Members depend on each
   other with `workspace:` ranges. `gtr publish -r` (anywhere in the workspace) publishes
   every member that is not private and whose version the registry does not have yet, in
   dependency order, and stops at the first failure (running it again continues). The
   repository must be clean (`--allow-dirty` to override); each member's tests run
   unless `--skip-tests`.
7. **What goes into the archive.** Like pnpm, the archive's `gtr.json` gets registry
   ranges instead of member references; the member's own `gtr.json` is not changed:

   | In the member | In the archive |
   |---|---|
   | `workspace:*` | `1.2.3` (the member's version) |
   | `workspace:^` | `^1.2.3` |
   | `workspace:~` | `~1.2.3` |
   | `workspace:^1` | `^1` (must match the member's version) |
   | `file:../core` (a path to a member) | `^1.2.3` |

   This applies to `dependencies` and `peerDependencies`; `devDependencies` stay as they
   are. A `file:` path outside the workspace or a reference to a private member is
   refused. A single `gtr publish` in a member does the same. `gtr publish --git` (tags)
   cannot rewrite anything, so it still refuses `workspace:` and `file:` sources.

## Consequences

- The families move to one repository each and to scoped names: `@passport/facebook`,
  `@orm/postgres`, `@fullstack/orm`, `@migration/orm`. Consumers change their keys
  (`"orm-postgres"` → `"@orm/postgres"`) and imports (`"orm-postgres"` →
  `"orm/postgres"`). `github:` sources keep working for the old names.
- A new family claims its scope with its first publish, so it should publish the bare
  core name or a scoped package before anybody else does.
- `gtr version` in a member still tags `<name>@<version>`; for a scoped name the tag is
  `@orm/postgres@0.1.0`, which git accepts.
