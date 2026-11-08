# ADR-0010. Build, test, release and links

Status: **Accepted** · 2026-11-08

## Context

Stages 0–3 made gtr install packages; a project still built, tested and released itself
with scripts of its own (gtr had `build.sh`, `build.ps1`, `win.ps1` and `release.sh`, and
every package repository copied some of them). npm has `npm test`, `npm version`,
`npm publish` and `npm link` in the tool; gtr should too, and gtr should be released by
gtr. Until the registry (stage 6) a release is a git tag that `github:`, `gitlab:` and
`git+` sources install from (ADR-0003, ADR-0004).

## Decision

1. **`gtr build`** reads the `build` block of `gtr.json` (schema: entry, output, outDir,
   targets, ldflags, tags, cgo, trimpath, files). Output goes to
   `<outDir>/<version>/<os>-<arch>/<output>[.exe]` (outDir `build`, version from
   `gtr.json` or `--version`). `ldflags` take `{{version}}`, `{{commit}}` (12 characters
   of `git rev-parse`), `{{date}}` (UTC RFC 3339, `SOURCE_DATE_EPOCH` respected) and
   `{{name}}`; an unknown placeholder is an error. Defaults: `CGO_ENABLED=0`,
   `-trimpath`. `-t os/arch` picks targets, `--all-targets` builds `build.targets` or
   darwin, linux and windows × amd64, arm64, up to three at a time.
   `--archive` writes `<output>_<version>_<os>_<arch>.zip` (Windows) or `.tar.gz` with the
   binary at the root plus `files` (default `README*`, `LICENSE*`) with a fixed time, and
   `SHA256SUMS` — the release format of ADR-0008. go runs with the environment gtr gives
   it in the project or workspace (ADR-0001, ADR-0009).
2. **`gtr dev`** builds the entry into `build/dev/`, runs it with the `.env` variables,
   polls the package and the workspace members for changes (Go files, `gtr.json`,
   `go.mod`, `.env*`, templates; no file-watching dependency) and rebuilds and restarts.
   A failed build keeps the running process.
3. **`gtr test`** is `go test ./...` in the package (in a workspace root, or with `-r`, every
   member, dependencies first). `--integration`, `--e2e` add those build tags (`--unit`
   none), `build.tags` always apply. The race detector is on by default (`test.race`)
   but skipped with a note where it cannot work (no cgo or no C compiler; an explicit
   `--race` is then an error). `--coverage` merges the profiles, prints coverage per
   package and fails below `test.coverage` (or `--min`); `--html` adds
   `build/coverage.html`. `--watch` reruns on changes.
4. **`gtr version <bump>`** (`patch`, `minor`, `major`, `pre*`, `prerelease`, a version)
   writes `gtr.json`, renames `## Unreleased` in `CHANGELOG.md` to `## <version> — <date>`
   under a new empty `## Unreleased`, commits both and makes an annotated tag. The tag is
   `v<version>` when the package is the root of its repository and `<name>@<version>` when
   it is in a subdirectory, the names `source.TagVersion` reads. The repository must be
   clean. A pre-release bumped with `patch`/`minor`/`major` becomes its release
   (`1.3.0-rc.1` → `1.3.0`).
5. **`gtr publish`** checks, in order: name and SemVer version, not `private`, no `file:` or
   `workspace:` in `dependencies`/`peerDependencies` (a tag cannot be rewritten the way npm
   rewrites `workspace:` in a tarball, so members keep real sources, ADR-0009), a clean
   repository, the version tag on `HEAD`, the tests (with the coverage threshold), the
   remote without that tag. Then `git push --atomic <remote> <branch> <tag>`.
6. **`gtr link`** in a package registers it in `~/.gtr/links.json`; `gtr link <name|path>` in
   a project records name → directory in `.gtr/links.json` (added to `.gitignore`) and
   reinstalls. The resolver treats a link like a workspace member: it replaces every
   source of that name (ranges still apply), but only if something requires the name.
   `gtr.lock` keeps the entry of the real source, so `gtr unlink` returns to it;
   `gtr ci` refuses to run with links. A workspace member cannot be linked.
7. **`gtr init --ci`** writes `.github/workflows/ci.yml`: Linux, Windows and macOS × the
   lowest and the newest Go of `engines.go` (through `GTR_GO_VERSION`), gtr from
   `install.sh`/`install.ps1`, then `gtr ci`, `go vet ./...`, `gtr test --coverage`.
8. **gtr and gtr-manager build themselves.** Each has a `gtr.json` with a `build` block; the
   release workflows build gtr-manager from source (for gtr: from the submodule) and run
   `gtr-manager build --all-targets --archive --version <tag>`. `make build|all|release`
   do the same locally without gtr installed. The shell and PowerShell build scripts are
   removed. The launcher's `gtr.json` is named `gtr-launcher`: `gtr` is a reserved name
   (ADR-0002).

## Consequences

- Package repositories need no build or release scripts; `gtr version patch && gtr publish`
  is the release.
- `gtr publish` does not create the GitHub release; a tag is enough for gtr sources, and
  repositories that ship binaries run `gtr build --all-targets --archive` in a tag
  workflow.
- In a monorepo every package has its own tags (`orm-mysql@0.1.1`); `v*` tags belong to
  the package at the root.
- Not covered: rewriting `workspace:` on publish, signing archives, registry publishing
  (stage 6), `gtr dev` for several packages at once.
