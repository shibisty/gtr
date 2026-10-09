# ADR-0009. Workspaces

Status: **Accepted** · 2026-10-09

## Context

A family of packages (orm and its drivers, passport and its strategies) is developed
together: a change to the core interface must be tested by every driver before it is
released (ADR-0006, item 3). Each package lives in its own repository, so the
`gtr.json` committed in a driver must work on its own — in the driver's CI it
names the core by its real source (`"orm": "github:shibisty/orm.go#^0.1"`). On the
developer's machine the same driver must see the local core instead.

Go has workspaces (`go.work`): several modules are built together and each one sees
the others in place. gtr generates `go.mod` (ADR-0001); it can generate `go.work` too.

## Decision

1. **A workspace is a directory whose `gtr.json` lists member directories:**
   ```json
   {"name": "orm-dev", "private": true, "workspaces": ["core", "drivers/*"]}
   ```
   Entries are paths or globs relative to the root (`*`, `?`, `[...]` within one path
   element; `**` is not supported). A leading `!` excludes matches wherever it appears
   in the list. Links, hidden directories and anything under `gtr_modules/` are never
   members. A matched directory
   without a `gtr.json` is skipped, except a literal path, which is an error. Members
   cannot be workspaces themselves, member names are unique and differ from the root's
   name. The root may have dependencies and code of its own.

2. **Members win over every other source.** Inside the workspace, any dependency on a
   member's name — from the root, another member or a downloaded package, whatever its
   source (`github:…`, `file:…`, a range) — resolves to the member in place. Its range
   is still checked against the member's `version`; a mismatch is an error, not a
   silent download (bump the range or the member's version). `workspace:<range>`
   (`workspace:*`, `workspace:^0.1`; `workspace:^` and `workspace:~` mean any) names a
   member explicitly and is an error outside a workspace — suitable for monorepos whose
   members are never installed alone, and replaced by a real range on publishing
   (`gtr publish`).

3. **One resolution for the whole workspace.** `gtr install` anywhere in a workspace
   installs it from the root: members are roots of the graph together with their
   `devDependencies` (members are developed, so their tests must build); one version of
   each package and module is selected for all of them (as Go itself does in a
   workspace). The root has the only `gtr.lock` (members are not recorded in it) and the
   only `gtr_modules/`; members get neither. `gtr.lock` lists the member directories
   (`"workspace"`), so the `go.mod` generated for a former member can be removed.

4. **Generated files:**
   - every member gets a generated `go.mod` (`module <name>`, its own dependencies as
     direct requirements, what those need in turn as indirect, no `replace`). Other
     members are not required: they come from `use`, and a versioned requirement on a
     workspace module makes go look that version up in the module proxy;
   - the root gets a generated `go.mod` the same way;
   - a member directory that another project uses as a `file:` package keeps its
     workspace `go.mod` (it serves both; a package `go.mod` would require the other
     members at a placeholder version, which go cannot resolve in workspace mode);
   - the root gets a generated `go.work` with the same hash header as `go.mod`:
     `use` of the root and every member, and the `replace` directives pointing into
     `gtr_modules/` (in workspace mode replacements belong to `go.work`).
   Members' and the root's `.gitignore` get `go.mod`; the root's also gets `go.work` and
   `go.work.sum`.

5. **go runs in workspace mode:** `GOWORK=<root>/go.work`, `GOPROXY=off`, and no
   `-mod=mod` (go rejects it in workspace mode; the generated files are complete, so
   the default `-mod=readonly` is enough). `gtr run`, the `go` shim and IDEs (which find
   `go.work` by walking up) all see the same build. This applies only when the
   directory's module is one of the `use` entries; a project nested in the workspace
   without being a member keeps `GOWORK=off`. In a member the launcher picks a Go that
   satisfies both the member's `engines.go` and the `go` line of `go.work`.

6. **Commands in a member** act on the workspace: `gtr install`, `update`, `ci` at the
   root; `gtr add`/`remove` change the member's `gtr.json` and then install the
   workspace. `gtr run <script>` runs the member's script; `gtr run -r <script>` (later
   step) runs it in every member that defines it.

## Consequences

- A driver's committed `gtr.json` keeps its real source; the workspace overrides it
  locally with no edits, so the same repository works alone and in the workspace.
- A member's `go.mod` is regenerated when it joins a workspace; a hand-written one is
  reported like a foreign project `go.mod` (`gtr sync --force` replaces it).
- Publishing (`gtr publish`) must refuse `workspace:` values that cannot be rewritten.
- Not covered: nested workspaces, `**` globs, carrying edits of `go.work` back to
  `gtr.json` (`gtr sync --force` regenerates it).
