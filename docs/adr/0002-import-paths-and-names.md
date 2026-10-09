# ADR-0002. Short import paths and package names

Status: **Accepted** · 2026-10-09

## Context

gtr aims for convenience. `import "orm"` reads better than
`import "github.com/shibisty/orm.go"`. Go allows paths without a dot in the first
element only via `replace`: such paths cannot be downloaded from the network,
and Go treats dotless paths as the standard library namespace.
The prototype confirmed that short paths work via `replace` (ADR-0001) and
revealed a conflict when a name matches a stdlib package (`ambiguous import`).

## Decision

1. **Import path = gtr package name.** `import "orm"`, subpackages as
   `import "orm/schema"`, drivers as `import "orm-mysql"`.

2. **Name format:** `^[a-z][a-z0-9]*(-[a-z0-9]+)*$`, 2–64 characters long.
   Lowercase letters, digits and single hyphens only. No dots: this is how gtr
   tells its own packages apart from external Go modules (which always have a dot
   in the first element, ADR-0003).

3. **Package families:** `<core>-<extension>`: `orm-mysql`, `orm-postgres`,
   `passport-google`, `passport-session-redis`. The family prefix is a convention,
   not a technical scope; claiming someone else's prefix in the registry requires
   permission from the owner of the core package (a registry rule, stage 6).

4. **The Go package name inside has no hyphen.** A hyphen is not allowed in `package`, so
   the last part of the name is declared: `package mysql` for `orm-mysql`,
   `package google` for `passport-google`. Usage:
   ```go
   import (
       "orm"
       mysql "orm-mysql" // the alias is optional, but gofmt/goimports will add it
   )
   conn, err := orm.New(ctx, mysql.Driver(dsn))
   ```
   If the last part clashes with another import (`mysql` from orm and from
   another library), the developer adds an alias, as in regular Go.

5. **Reserved names.** `gtr init`, `gtr add` and `gtr publish` reject:
   - first path elements of the standard library of the active Go version — the list
     comes from `go list std` at check time rather than being hard-coded (currently
     `errors`, `log`, `net`, `os`, `crypto`, `testing` and ~50 more);
   - go reserved words: `std`, `cmd`, `all`, `main`, `tool`, `work`,
     `internal`, `vendor`, `test`;
   - gtr reserved words: `gtr`, `go`, `self`.

   *Implementation (stage 3):* the list is cached per Go version in
   `~/.gtr/cache/std-<version>.txt`; `gtr init` checks the project name and
   `gtr install` checks the project and every installed package. Without a go command
   the check is skipped (the launcher always provides one). `gtr publish` arrives with
   the registry.

## Consequences

- Code looks like Node/Laravel code: short, memorable imports.
- A residual risk remains: if the stdlib later gains a package with the name of an already
  published gtr package, projects will get `ambiguous import` after a
  Go upgrade. The probability is low (Go adds 0–2 top-level packages
  per release, with names like `iter`, `unique`, `weak`); gtr catches it with an error
  with a clear message (ADR-0001, item 8), and on every Go release the registry checks
  all names and warns the owners.
- A package cannot be imported with a plain `go get` without gtr; this is a deliberate choice.
