# Contributing

Outside contributors do not need Beads to contribute to this repository.

## Current project shape

This repository ships an installed JSON runner and Agent Skills-compatible
skill. It does not ship a standalone host service, background daemon, or remote
API.

## Local setup

Maintainers prefer:

```bash
mise install
```

Outside contributors may use their own local tooling if they can satisfy the repository checks.

Beads and Dolt are maintainer-only tools. They are optional for outside contributors and are not required to open, review, or merge pull requests.

## Pull request expectations

- Keep changes reviewable without access to Beads state.
- Update repository docs and release-contract tests when the JSON runner contract changes.
- Do not present a public SDK, REST API, hosted service, or web UI as supported
  behavior unless that product surface has been explicitly approved.
- Do not commit credentials, private infrastructure details, or sensitive sample data.
- Route security issues through the private process in [SECURITY.md](SECURITY.md), not through public issues or pull requests.

## Checks and review rules

Current pull request checks validate repository policy, the production runner,
the production skill payload, release packaging, dependency review, and a Go
vulnerability scan.

Before sending a pull request, run:

```bash
make check
```

`go test ./...` includes runner, service, recurrence, release-contract, and
agent-eval harness coverage without requiring a host service.

## Support and compatibility

Before `1.0`, compatibility is best effort and may change between releases. The
supported product surface is the installed `openplanner` JSON runner and the
Agent Skills-compatible `skills/openplanner` payload. SQLite storage defaults to
`${XDG_DATA_HOME:-~/.local/share}/openplanner`, plus explicit path override
support. No compatibility promise is made for a public SDK, REST API, hosted
service, background process, or web UI because none is shipped as a v1 product
surface.

Maintainer workflow notes live in [docs/maintainers.md](docs/maintainers.md).
