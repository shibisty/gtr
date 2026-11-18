# gtr.json

The JSON Schema is [`schema/gtr.schema.json`](../../schema/gtr.schema.json); add
`"$schema": "https://raw.githubusercontent.com/shibisty/gtr/main/schema/gtr.schema.json"`
for completion in editors.

| Field | Type | Meaning |
|---|---|---|
| `name` | string | package name: lowercase letters, digits, single hyphens; no standard library names (ADR-0002); or `@scope/name`, imported as `scope/name` (ADR-0012). The import path is the name without a scope's `@` |
| `version` | string | SemVer 2.0 version, e.g. `1.0.0`, `0.2.0-rc.1` (ADR-0004) |
| `description` | string | shown by `gtr info` and the registry |
| `keywords` | string[] | for search |
| `license` | string | SPDX identifier, e.g. `MIT` |
| `author` | string or object | `"Ann <ann@example.com>"` or `{"name", "email", "url"}` |
| `homepage` | string | |
| `repository` | string | e.g. `github:shibisty/orm.go` |
| `private` | boolean | `gtr publish` refuses the package (applications, workspace roots) |
| `engines.go` | range | which Go runs the project; the `go` line of `go.mod` |
| `engines.gtr` | range | which gtr-manager runs the project |
| `dependencies` | object | import path → source ([sources](#sources)) |
| `devDependencies` | object | the same, for tests and tools; not installed for projects that depend on this one |
| `peerDependencies` | object | name → range the consuming project must install itself (a driver's core) |
| `workspaces` | string[] | member folders: paths, one-level globs (`drivers/*`), `!dir` to exclude (ADR-0009) |
| `scripts` | object | name → shell command for `gtr run`; `pre<name>`, `post<name>` |
| `build` | object | `gtr build`: see below |
| `test` | object | `gtr test`: see below |
| `gomod` | `"ignore"` or `"commit"` | whether the generated `go.mod` is ignored by git (default) or committed (ADR-0001) |

## build

| Field | Default | Meaning |
|---|---|---|
| `entry` | `.` | the main package, e.g. `./cmd/app` |
| `output` | `name` | binary name (`.exe` added on Windows) |
| `outDir` | `build` | output folder; binaries go to `<outDir>/<version>/<os>-<arch>/` |
| `targets` | this platform | `GOOS/GOARCH` list for `--all-targets` |
| `ldflags` | | `{{version}}`, `{{commit}}`, `{{date}}`, `{{name}}` are replaced |
| `tags` | | build tags (also used by `gtr test`) |
| `cgo` | `false` | `CGO_ENABLED=1` |
| `trimpath` | `true` | `-trimpath` |
| `files` | `README*`, `LICENSE*` | files and globs added to archives (`--archive`) |

## test

| Field | Meaning |
|---|---|
| `coverage` | minimum coverage in %; `gtr test --coverage` fails below it |
| `race` | force `-race` on or off (default: on when cgo and a C compiler are available) |
| `timeout` | `go test -timeout`, e.g. `5m` |

## Sources

| Value | Source |
|---|---|
| `^0.2`, `1.2.3`, `*` | the gtr registry |
| `github:owner/repo`, `#^0.2`, `github:owner/repo/sub/dir#^0.1` | GitHub tags |
| `gitlab:group/project`, `gitlab:group/project//sub/dir` | GitLab |
| `git+https://host/repo.git`, `git+ssh://…`, `git+file://…`, `//sub/dir`, `#range` | any git server |
| `file:../path` | a local folder, linked |
| `workspace:*`, `workspace:^0.2` | a workspace member |
| `npm:^10.19` | an npm package (into `node_modules/`) |
| a range, under a key with a dot (`github.com/google/uuid`) | a Go module |

Ranges: `1.2.5`, `^1.2`, `~1.2.3`, `1.2`, `>=1.0 <2.0`, `*`; pre-releases only when named.
