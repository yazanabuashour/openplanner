# Contributing

Outside contributors do not need Beads to contribute to this repository.

## Current project shape

This repository ships an embeddable Go module, direct local SDK, installed JSON
runner, and Agent Skills-compatible skill. It does not ship a standalone host
service, background daemon, or remote API.

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

Current pull request checks validate repository policy, the embeddable Go SDK,
the production skill payload, packaging smoke coverage, dependency review, and a
Go vulnerability scan.

Before sending a pull request, run:

```bash
make check
```

`go test ./...` includes the temp-module packaging smoke test that opens `sdk.OpenLocal(...)`, writes SQLite data to an explicit path, and reads it back without any host service.

## Support and compatibility

Before `1.0`, compatibility is best effort and may change between releases. The
supported product surface is the direct local Go SDK on Go `1.26.2`, the
installed `openplanner` JSON runner, and the Agent Skills-compatible
`skills/openplanner` payload. SQLite storage defaults to
`${XDG_DATA_HOME:-~/.local/share}/openplanner`, plus explicit path override
support. No compatibility promise is made for a hosted service or background
process because none is shipped.

Maintainer workflow notes live in [docs/maintainers.md](docs/maintainers.md).
