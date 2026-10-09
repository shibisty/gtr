# ADR-0004. Versions, dependency resolution, gtr.lock

Status: **Proposed** · 2026-10-09

## Context

Developers know npm ranges (`^`, `~`). In Go, a single build can contain
only one version of a package with a given import path, so there will be no "two orms of different
versions" as in node_modules. GitHub archives (`/archive/refs/tags/…`)
are not guaranteed to be byte-for-byte stable, so hashing the archive is unreliable.

## Decision

**Versions are SemVer 2.0.** Ranges work as in npm:

| Notation | Meaning |
|---|---|
| `1.2.5` | exactly 1.2.5 |
| `^1.2` | `>=1.2.0 <2.0.0` (for `0.x`: `^0.1` = `>=0.1.0 <0.2.0`) |
| `~1.2.3` | `>=1.2.3 <1.3.0` |
| `1` | `>=1.0.0 <2.0.0` |
| `1.2` | `>=1.2.0 <1.3.0` |
| `>=1.0 <2.0` | explicit range |
| `*` | any stable version |

A pre-release (`1.3.0-rc.1`) is selected only if the range names it explicitly.

**Resolution.** One version per package name across the whole tree. The algorithm picks
the highest version satisfying all ranges and backtracks on conflicts
(PubGrub, as in Dart/uv, which produces explainable conflict messages).
*Stage 3 implements a plain backtracking search with the same rules; its conflict
report lists every range that applies to the failing package and who required it.
PubGrub can replace it later if real graphs need smarter backtracking.*
The version from `gtr.lock` is preferred as long as it satisfies the ranges.
*Stage 3: a module required from `gtr.json` gets the highest version its range
allows (or the locked one), like `go get`; versions then only go up through MVS. The
generated `go.mod` lists every selected module, so the go command reads each one's
`go.mod`; gtr does the same, to a fixed point, so both compute the same build list.
If MVS raises a module above a `gtr.json` range, installation fails with an
explanation. Lock entries of external modules also pin the `go.mod` hash (`goMod`).*
For Go modules, gtr honours their `go.mod` and takes the minimum version
that satisfies all requirements (like Go's MVS), so as not to break modules
that rely on that behaviour.

`peerDependencies` (drivers → core) are not installed automatically: they check that
the project has installed a compatible version of the core package, and fail with a hint otherwise.

**`gtr.lock`** is JSON with sorted keys, committed to the repository:

```json
{
  "lockfileVersion": 1,
  "packages": {
    "orm": {
      "version": "0.1.3",
      "source": "github:shibisty/orm.go",
      "commit": "04ccb78…",
      "integrity": "h1:base64…",
      "dependencies": {}
    },
    "orm-mysql": {
      "version": "0.1.0",
      "source": "github:shibisty/orm.go/drivers/mysql",
      "commit": "9a1f…",
      "integrity": "h1:…",
      "dependencies": { "orm": "^0.1", "github.com/go-sql-driver/mysql": "^1.10" }
    },
    "github.com/go-sql-driver/mysql": {
      "version": "v1.10.0",
      "source": "go",
      "integrity": "h1:…"
    }
  }
}
```

**Integrity** is computed over the extracted file tree, not the archive:
Go's `h1` algorithm (`golang.org/x/mod/sumdb/dirhash`, SHA-256 over a
sorted list of "file hash + path"). For Go modules the value
matches `go.sum` and is verified against sum.golang.org.

## Consequences

- `gtr ci` reproduces the tree byte for byte or fails.
- A major upgrade of a core package (`orm` 1 → 2) requires every driver in the project
  to support the new major version — the same constraint as in Go,
  and the conflict message has to explain it.
- The resolver is the hardest part of stage 3; it is covered by
  table-driven tests on synthetic graphs.
