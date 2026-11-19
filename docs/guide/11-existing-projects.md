# 11. An existing Go project

A project with a `go.mod` moves to gtr in two commands:

```bash
cd myproject
gtr init          # gtr.json; the module path becomes the name when it is a valid gtr name
gtr sync          # go.mod → gtr.json: require → dependencies, go → engines.go
gtr install       # gtr.lock, gtr_modules/, and go.mod generated from gtr.json
gtr test
```

`gtr sync` keeps the module versions of `go.mod` (as `^` ranges from those versions), so
the build list does not move. `replace x => ../dir` becomes a `file:` source.

## The module path

A gtr name has no dots (ADR-0002). If the module was `github.com/me/app`, the project is
now `app`, and imports of its own packages change:

```go
import "github.com/me/app/internal/store"   // before
import "app/internal/store"                 // after
```

A search and replace over the `.go` files does it. External modules keep their full paths:
`"github.com/google/uuid": "^1.6"` stays `import "github.com/google/uuid"`.

## Replacing modules with gtr packages

Where a gtr package exists for something you use, swap it in at your own pace:

```bash
gtr remove github.com/someone/orm
gtr add orm
```

## Keeping go.mod for others

If people still build the project with plain `go` (`go install github.com/me/app@latest`),
set `"gomod": "commit"` and commit the generated `go.mod` — but note that gtr packages are
then only resolvable through gtr. For a library meant for both worlds, keep publishing it
as a Go module and add `gtr.json` alongside.

## After `go get`

If someone runs `go get` or edits `go.mod` in a gtr project, gtr notices (the hash in the
first line no longer matches) and stops. `gtr sync` takes the change into `gtr.json`;
`gtr sync --force` discards it.

Next: [web applications](12-web-apps.md).
