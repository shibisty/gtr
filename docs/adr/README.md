# Architecture Decision Records (ADR)

Each decision is a separate file: context, decision, consequences.
Statuses: **Accepted** — agreed; **Proposed** — awaiting the author's decision;
**Superseded** — see the link to the new decision.

| # | Decision | Status |
|---|---|---|
| [0001](0001-module-model.md) | Module model: gtr.json is the source of truth, go.mod is generated | Accepted |
| [0002](0002-import-paths-and-names.md) | Short import paths and package names | Accepted |
| [0003](0003-dependency-sources.md) | Package sources and dependency entry format | Accepted |
| [0004](0004-versions-and-lock.md) | Versions, dependency resolution, gtr.lock | Proposed |
| [0005](0005-home-layout.md) | The ~/.gtr home directory and the store | Accepted |
| [0006](0006-repositories.md) | Repository layout: one repository per package, gtr-manager as a gtr submodule | Accepted |
| [0007](0007-minimum-go.md) | Minimum Go version for libraries | Proposed |
| [0008](0008-version-selection.md) | Version selection, gtr-manager releases and command forwarding | Proposed |

The prototype that validated 0001 and 0002: [docs/prototype/bridge](../prototype/bridge/README.md).
