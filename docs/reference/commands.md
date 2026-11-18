# Commands

`gtr help` and `gtr-manager help` print the same list. Project commands are run by
gtr-manager with the project's Go; `gtr` runs the others itself.

## Project

| Command | |
|---|---|
| `gtr new <name> [-y] [-t app\|cli\|lib\|fullstack\|orm-driver\|none]` | create a project in a new folder |
| `gtr init [name] [-y] [--ci]` | `gtr.json` in this folder (`--ci`: a GitHub Actions workflow) |

## Packages

| Command | |
|---|---|
| `gtr install` (`i`) | install what `gtr.json` asks for, keeping `gtr.lock` |
| `gtr add <source>... [-D]` (`install <source>`) | add packages (`-D`: devDependencies) |
| `gtr remove <name>...` (`uninstall`, `rm`) | remove packages |
| `gtr update [name]...` (`up`, `upgrade`) | newest versions the ranges allow |
| `gtr ci` | install exactly `gtr.lock`; fail if it does not match `gtr.json` |
| `gtr sync [--force]` | carry `go.mod` edits into `gtr.json` (`--force`: regenerate `go.mod`) |
| `gtr list [--tree] [--depth N] [--json]` (`ls`) | installed packages |
| `gtr why <name>` | which requirements bring a package in |
| `gtr outdated [--all] [--json]` | packages with newer versions |
| `--offline` (install, add, update, ci, sync, remove) | only what the store has |

## Scripts, tests, builds

| Command | |
|---|---|
| `gtr run [name] [-- args]` (`start`) | run a script (`start` by default) |
| `gtr run -r <name>` | in every workspace member that has it |
| `gtr run --no-env <name>` | without `.env` files |
| `gtr <script>` | a script whose name is not a command |
| `gtr test [--unit\|--integration\|--e2e] [--coverage [--html] [--min N]] [--watch] [-r] [--run RE] [-v] [--race\|--no-race]` | `go test ./...` with tags and coverage |
| `gtr build [-t os/arch]... [--all-targets] [--archive] [--version V]` | build by the `build` block |
| `gtr dev [-- args]` | build, run, rebuild and restart on changes |

## Releases and the registry

| Command | |
|---|---|
| `gtr version` | the package version |
| `gtr version patch\|minor\|major\|prerelease\|<x.y.z> [--preid rc] [--no-git] [--no-tag]` | bump, date the changelog, commit, tag |
| `gtr publish [--dry-run] [--skip-tests] [--allow-dirty] [--registry <url> \| --git]` | publish |
| `gtr publish -r [--dry-run] [--skip-tests] [--allow-dirty]` | publish every workspace member the registry does not have yet, dependencies first ([ADR-0012](../adr/0012-scopes-and-monorepos.md)) |
| `gtr login [--registry <url>] [--token <t>]`, `gtr logout`, `gtr whoami` | the registry account |
| `gtr search <query>`, `gtr info <name>` | find packages |
| `gtr yank <name>@<version> [--undo]` | hide a version from new installs |
| `gtr owner <name>`, `gtr owner add\|remove <name> <email>` | who may publish |

## Local development

| Command | |
|---|---|
| `gtr link` | in a package: register it |
| `gtr link <name\|path>`, `gtr link --list` | in a project: use a local copy |
| `gtr unlink [name] [--global]` | stop using local copies |

## gtr itself

| Command | |
|---|---|
| `gtr self install go\|gtr [version]` | install a Go or gtr-manager version |
| `gtr self uninstall go\|gtr <version>` | remove one |
| `gtr self use go\|gtr <version>` | the global default |
| `gtr self list [go\|gtr] [--remote [--all]]` | installed or available versions |
| `gtr self current` | the versions used here and why |
| `gtr self link gtr <path>`, `gtr self unlink gtr` | use a local gtr-manager build |
| `gtr self shims` | create the `go`/`gofmt` shims |
| `gtr self update [version]` | update gtr |
| `gtr self prune [--dry-run]` | remove archives and superseded patch versions |
| `gtr env [--json] [NAME...]` | Go, gtr-manager and paths used here |
| `gtr doctor [--offline]` | check the installation |
| `gtr mcp [--root dir] [--allow-publish] [--allow-self-install]` | the MCP server for Claude |
| `gtr mcp --print-config`, `gtr mcp bundle [-o file]` | client settings, a Claude Desktop extension |
| `gtr --go <v> <command>`, `gtr --gtr <v> <command>` | versions for one command |
| `gtr version`, `gtr help` | |
