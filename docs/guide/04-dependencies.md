# 4. Dependencies

## Adding

```bash
gtr add <source>...          # dependencies
gtr add -D <source>...       # devDependencies (tests, tools)
```

| Source | Example | Saved as |
|---|---|---|
| gtr registry | `gtr add orm`, `gtr add orm@^0.2` | `"orm": "^0.2"` |
| GitHub | `gtr add orm` | `"orm": "orm#^0.2"` |
| GitHub, a range | `gtr add orm#^0.1` | the range as given |
| a monorepo folder | `gtr add github:owner/repo/packages/http` | |
| GitLab | `gtr add gitlab:group/project`, `gitlab:group/project//sub/dir` | |
| any git server | `gtr add git+https://git.example.com/team/lib.git` (also `git+ssh://`, `git+file://`) | |
| a local folder | `gtr add file:../passport.go/core` | linked in place, not copied |
| a Go module | `gtr add github.com/google/uuid`, `gtr add github.com/redis/go-redis/v9@~9.7.0` | `"github.com/google/uuid": "^1.6.0"` |
| an npm package | `gtr add npm:preact` | `"preact": "npm:^10.19.3"` (into `node_modules/`) |
| a workspace member | `gtr add orm` inside a workspace | `"orm": "workspace:*"` |

The name of a gtr package comes from its own `gtr.json`, and it must match the key —
`gtr add orm=github:someone/fork` fails unless the fork is really called `orm`. Without
a range, gtr takes the newest version and saves `^<major.minor>` (for 0.x) so that patch
releases arrive with `gtr update`.

### Where versions come from

- **Registry:** the versions published to it ([Publishing](09-publishing.md)).
- **git sources:** tags. `v1.2.3` or `1.2.3` at the repository root; `<name>@1.2.3` or
  `<sub/dir>/v1.2.3` for a package in a subfolder. A repository without version tags can
  still be used with `*`: gtr takes the default branch and pins its commit in `gtr.lock`.
- **Go modules:** the module proxy (`GTR_GOPROXY`, default proxy.golang.org) and the
  checksum database, like `go get`; gtr computes the same build list the go command would.
- **npm:** the npm registry; versions and hoisting follow npm's rules. Install scripts
  never run.

### Ranges

| Range | Means |
|---|---|
| `1.2.5` | exactly 1.2.5 |
| `^1.2` | `>=1.2.0 <2.0.0`; for 0.x `^0.2` is `>=0.2.0 <0.3.0` |
| `~1.2.3` | `>=1.2.3 <1.3.0` |
| `1.2` | `>=1.2.0 <1.3.0` |
| `>=1.0 <2.0` | an explicit range |
| `*` | any stable version |

Pre-releases (`1.3.0-rc.1`) are chosen only when the range names one.

## One version per package

A project gets one version of each package, for all the packages that need it. When they
disagree, gtr says who asked for what:

```
no version of orm satisfies all requirements:
  the project requires orm ^0.2
  @orm/mysql@0.1.0 requires orm ^0.1
available versions: 0.2.0, 0.1.3, 0.1.0
```

Fix it by changing your range or updating the package that holds the old one.

**Peer dependencies** are what a package expects the application to provide: a database
driver lists `"peerDependencies": {"orm": "^0.2"}`. They are not installed for you; if the
project has no matching `orm`, gtr stops and tells you to add it.

## Installing

```bash
gtr install          # what gtr.json asks for, keeping the versions in gtr.lock
gtr ci               # exactly gtr.lock; fails if it does not match gtr.json (CI, deploys)
gtr install --offline   # only what ~/.gtr/store already has (or GTR_OFFLINE=1)
```

Downloads run in parallel and are verified: registry archives against their SHA-256, git
packages against the commit and hash in `gtr.lock`, Go modules against `gtr.lock` or the
checksum database, npm tarballs against their `integrity`. If a tag was moved to another
commit after you locked it, gtr refuses to install until you `gtr update` that package.

## Looking around

```bash
gtr list                 # direct dependencies
gtr list --tree          # everything (--depth 2; --json)
gtr why strutil          # who brings strutil in
gtr outdated             # newer versions (--all: also indirect ones)
```

```
Package        Current  Wanted  Latest  Source
orm            0.1.3    0.1.5   0.2.0   orm
faker (dev)    0.1.0    0.1.0   0.2.0   github:shibisty/faker.go
```

*Wanted* is the newest version your ranges allow — what `gtr update` installs. To get
*Latest*, change the range: `gtr add orm#^0.2`.

## Updating and removing

```bash
gtr update               # every package to the newest version its range allows
gtr update orm           # only orm
gtr remove @orm/mysql     # from gtr.json, gtr.lock, gtr_modules/ and go.mod
```

## Private packages and tokens

| | |
|---|---|
| GitHub | `GITHUB_TOKEN` (private repositories, and the API limit of 60 requests an hour without a token) |
| GitHub Enterprise, a mirror | `GTR_GITHUB_API` and `GTR_GITHUB_TOKEN` |
| GitLab | `GTR_GITLAB_URL`, `GTR_GITLAB_TOKEN` (or `GITLAB_TOKEN` for gitlab.com) |
| git servers | git's own credentials (credential helpers, SSH keys); gtr never waits for a password |
| private Go modules | `GOPRIVATE`, `GTR_GOPROXY_PRIVATE` |
| npm | `GTR_NPM_REGISTRY`, `GTR_NPM_TOKEN` |
| gtr registry | `gtr login`, or `GTR_REGISTRY_TOKEN` in CI |

```powershell
$env:GITHUB_TOKEN = "ghp_..."     # bash: export GITHUB_TOKEN=ghp_...
```

Next: [scripts and .env](05-scripts.md).
