# 8. Workspaces and local copies

## Workspaces

Packages developed together — a core and its drivers, a framework and its plugins — live
in one workspace (ADR-0009): a root folder whose `gtr.json` lists the members.

```
orm.go/
  gtr.json             {"name": "orm-dev", "private": true, "workspaces": ["core", "drivers/*"]}
  core/gtr.json        {"name": "orm", "version": "0.2.0"}
  drivers/mysql/gtr.json
                       {"name": "orm-mysql", "dependencies": {"orm": "github:shibisty/orm.go#^0.2"}}
```

- **Members win.** A dependency on a member's name — from the root, a member or any
  downloaded package, whatever its source — uses the member's folder. Its version must
  still fit the range. So `orm-mysql` keeps its real source (it has to install on its own,
  in its own CI), and inside the workspace it gets the local `orm` without edits.
  `"orm": "workspace:*"` (what `gtr add orm` writes in a member) names a member explicitly.
- **One install.** `gtr install` anywhere in the workspace installs everything from the
  root: one `gtr.lock`, one `gtr_modules/`, one version of each package, members'
  `devDependencies` included.
- **Generated files.** A `go.mod` in every member and a `go.work` at the root, which
  editors and the go command find by themselves.
- **Commands in a member** act on that member: `gtr add` changes its `gtr.json`, `gtr run
  test` runs its script. `gtr run -r test`, `gtr test -r` and `gtr build` at the root go
  through all members, dependencies first.

Patterns: paths or one-level globs (`drivers/*`), `"!drivers/old"` to exclude.

A family that is published from one repository uses scoped names and `workspace:` ranges,
and publishes with `gtr publish -r` ([publishing](09-publishing.md#a-whole-monorepo)):

```
orm.go/
  gtr.json                {"name": "orm-dev", "private": true, "workspaces": ["core", "drivers/*"]}
  core/gtr.json           {"name": "orm", "version": "0.2.0"}
  drivers/postgres/gtr.json
                          {"name": "@orm/postgres", "version": "0.1.0", "dependencies": {"orm": "workspace:^"}}
```

### Using a neighbour's packages

A dependency of the workspace root overrides that package's source for the whole
workspace — handy for two families side by side:

```json
{
    "name": "passport-dev",
    "private": true,
    "workspaces": ["core", "strategies/*", "sessions/*", "example"],
    "dependencies": {
        "orm": "file:../orm.go/core"
    }
}
```

## Local copies

To try a change of a package in one project without a workspace:

```bash
cd ~/src/orm.go/core && gtr link     # register: orm → this folder
cd ~/src/app && gtr link orm         # use it here
gtr link ../orm.go/core              # or by path, without registering
gtr link --list
gtr unlink orm                       # back to gtr.lock (gtr unlink: all of them)
```

A link replaces every source of that name until `gtr unlink`. It is personal: kept in
`.gtr/links.json` (ignored by git), and `gtr.json` and `gtr.lock` do not change. `gtr ci`
refuses to run while links are active, so a CI build never depends on your folders.

`file:../path` in `gtr.json` is the committed alternative: everyone gets the folder, which
suits a monorepo but not a package you publish (`gtr publish` refuses `file:` sources).

Next: [publishing a package](09-publishing.md).
