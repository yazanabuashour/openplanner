# openplanner

OpenPlanner is a local-first planning runtime for agent-facing calendar and task
workflows. It ships an installed `openplanner` JSON runner, an Agent
Skills-compatible skill, and SQLite-backed local storage.

## Agent Install

Tell your agent:

```text
Install OpenPlanner from https://github.com/yazanabuashour/openplanner.
Complete both required steps before reporting success:
1. Install and verify the openplanner runner binary.
2. Register the OpenPlanner skill from skills/openplanner/SKILL.md using your native skill system.
```

The agent should install the `openplanner` runner and place the
`skills/openplanner` skill where that agent normally loads skills. OpenPlanner
does not require one canonical skill path: Codex, Claude Code, OpenClaw, Hermes,
and other Agent Skills-compatible agents each manage skill locations in their
own way.

## Manual Install

Until the first release tag is published, build the runner from a checkout:

```bash
go build -o ./bin/openplanner ./cmd/openplanner
```

Put that binary on `PATH`, then install or copy `skills/openplanner` using your
agent's skill installation workflow. The portable skill payload is the folder
containing `SKILL.md`; agent-specific directories are intentionally not part of
OpenPlanner's release contract.

After `v0.1.0`, release archives will include platform builds of the
`openplanner` runner, `install.sh`, and an `openplanner_<version>_skill.tar.gz`
archive for manual skill installation. The release installer installs and
verifies only the runner; skill registration remains delegated to the target
agent's native skill system.

```bash
curl -fsSL https://github.com/yazanabuashour/openplanner/releases/latest/download/install.sh | sh
```

Optional per-agent examples are in [docs/agent-install.md](docs/agent-install.md).
Those examples are not the OpenPlanner install contract.

## Runner Interface

The skill calls the installed runner:

```bash
printf '%s\n' '{"action":"list_agenda","from":"2026-04-16T00:00:00Z","to":"2026-04-17T00:00:00Z"}' \
  | openplanner planning
```

The runner reads structured JSON from stdin, validates and normalizes the
request, performs the local planning operation, and writes structured JSON to
stdout. By default, it stores SQLite data at
`${XDG_DATA_HOME:-~/.local/share}/openplanner/openplanner.db`. Override the path
with `OPENPLANNER_DATABASE_PATH` or `openplanner planning --db <path>`; `--db`
wins when both are present.

Supported routine actions are:

- `ensure_calendar`
- `create_event`
- `create_task`
- `update_calendar`
- `update_event`
- `update_task`
- `list_agenda`
- `list_events`
- `list_tasks`
- `complete_task`
- `validate`

Update actions use patch semantics: omitted fields are preserved, non-null
fields are set, and `null` clears clearable optional fields. Use `event_id` for
`update_event`, `task_id` for `update_task`, and exactly one of `calendar_id` or
`calendar_name` for `update_calendar`.

## Development

Install the pinned toolchain with:

```bash
mise install
```

Exercise the JSON runner with:

```bash
go build -o ./bin/openplanner ./cmd/openplanner
printf '%s\n' '{"action":"validate"}' | ./bin/openplanner planning
```

Run the local quality gates with:

```bash
make check
```

`make check` runs formatting validation, Agent Skills validation, the Go test
suite, `govulncheck`, and `golangci-lint`.

## Release Contract

The `0.1.0` release deliverables are:

- platform archives for the `openplanner` runner binary
- the single-file `openplanner` skill archive
- a canonical source archive, SHA256 checksums, an SPDX SBOM, and GitHub
  attestations for release verification

Until `v0.1.0` exists, local development should build the runner from a checkout
or install from `main`.

## Repository Contents

- [CONTRIBUTING.md](CONTRIBUTING.md) explains how outside contributors should propose changes.
- [SECURITY.md](SECURITY.md) explains how to report vulnerabilities privately and what response timing to expect.
- [docs/maintainers.md](docs/maintainers.md) documents Beads-based maintainer workflow and repo administration notes.
- [docs/release-verification.md](docs/release-verification.md) explains the published release assets and how to verify them.
- [docs/agent-evals.md](docs/agent-evals.md) explains how to evaluate production agent workflows.
- [internal/runner](internal/runner) contains the JSON-friendly task facade for production agent workflows.
- [cmd/openplanner](cmd/openplanner) contains the installed JSON runner.
- [skills/openplanner/SKILL.md](skills/openplanner/SKILL.md) is the portable Agent Skills-compatible OpenPlanner skill.
- [LICENSE](LICENSE) defines the project license.

## Contributing

Outside contributors can work entirely through GitHub issues and pull requests.
Beads is maintainer-only workflow tooling and is not required for community
contributions.

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution expectations and
[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for community standards.
