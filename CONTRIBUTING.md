# Contributing

Outside contributors do not need Beads to contribute to this repository.

## Current project shape

This repository ships an embeddable Go module and generated client for in-process local use. It does not ship a standalone host service, background daemon, or native installer.

Changes to the public SDK surface should also update the install/runtime docs, compatibility notes, and validation gates that keep the in-process release story honest.

## Local setup

Maintainers prefer:

```bash
mise install
```

Outside contributors may use their own local tooling if they can satisfy the repository checks.

Beads and Dolt are maintainer-only tools. They are optional for outside contributors and are not required to open, review, or merge pull requests.

## Pull request expectations

- Keep changes reviewable without access to Beads state.
- Update repository docs when the public contract changes.
- Do not commit credentials, private infrastructure details, or sensitive sample data.
- Route security issues through the private process in [SECURITY.md](SECURITY.md), not through public issues or pull requests.

## Checks and review rules

Current pull request checks validate repository policy, the embeddable Go SDK, its OpenAPI contract, packaging smoke coverage, dependency review, and a Go vulnerability scan.

Before sending a pull request, run:

```bash
make check
```

`go test ./...` includes the temp-module packaging smoke test that opens `sdk.OpenLocal(...)`, writes SQLite data to an explicit path, and reads it back without any host service.

## Support and compatibility

Before `1.0`, compatibility is best effort and may change between releases. The supported product surface is the in-process Go SDK on Go `1.26.2` with SQLite storage under `${XDG_DATA_HOME:-~/.local/share}/openplanner` by default, plus explicit path override support. No compatibility promise is made for a hosted service, background process, or native installer because none is shipped.

Maintainer workflow notes live in [docs/maintainers.md](docs/maintainers.md).
