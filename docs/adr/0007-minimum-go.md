# ADR-0007. Minimum Go version for libraries

Status: **Proposed** · 2026-10-09

## Context

Versions have drifted apart: the drivers use `go 1.22`, orm.go `1.25.0`, faker.go and
gtr `1.26.5`. faker.go requires 1.26.5 even though its README promises 1.18+, which is why
orm.go had to exclude the faker example from `go.work`.

## Decision

- **Libraries** (faker, orm, passport and their packages): `engines.go: ">=1.22"`.
  1.22 has generics, `for range` over integers and method-based routing
  in `net/http`, which is enough. Raise it only for a specific feature,
  with a CHANGELOG entry.
- **Tools** (gtr, gtr-manager): the latest stable Go release;
  it does not matter to users, who get a prebuilt binary.
- **fullstack**: decided in stage 8, 1.22 by default as well.
- Library CI tests two versions: the minimum (1.22) and the latest.

## Consequences

- `gtr` picks the Go version for a project from `engines.go` (stage 2), so a low
  minimum does not prevent an application from using a newer Go.
- faker.go and orm.go need to lower their `go` directive in stage 1 and verify the build on 1.22.
