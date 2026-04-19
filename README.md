# openplanner

OpenPlanner is a local-first planning runtime for agent-facing calendar and task
workflows. It ships an installed `openplanner` JSON runner, an Agent
Skills-compatible skill, and a direct local Go SDK backed by SQLite storage.

## Agent Install

Tell your agent:

```text
Install https://github.com/yazanabuashour/openplanner
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
`openplanner` runner and an `openplanner_<version>_skill.tar.gz` archive for
manual skill installation.

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
- `list_agenda`
- `list_events`
- `list_tasks`
- `complete_task`
- `validate`

## Local Go SDK

Go developers can embed the same local runtime directly:

```bash
go get github.com/yazanabuashour/openplanner@main
```

Minimal usage from Go:

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/yazanabuashour/openplanner/sdk"
)

func main() {
	client, err := sdk.OpenLocal(sdk.Options{})
	if err != nil {
		panic(err)
	}
	defer client.Close()

	ctx := context.Background()
	calendar, err := client.EnsureCalendar(ctx, sdk.CalendarInput{Name: "Personal"})
	if err != nil {
		panic(err)
	}

	startAt := time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)
	endAt := startAt.Add(time.Hour)
	if _, err := client.CreateEvent(ctx, sdk.EventInput{
		CalendarID: calendar.Calendar.ID,
		Title:      "Standup",
		StartAt:    &startAt,
		EndAt:      &endAt,
	}); err != nil {
		panic(err)
	}

	agenda, err := client.ListAgenda(ctx, sdk.AgendaOptions{
		From: time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(len(agenda.Items))
}
```

`sdk.OpenLocal(...)` opens SQLite locally and calls the same local planning
service used by the runner. There is no hosted service, remote API contract,
daemon, or localhost listener in the release surface.

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
suite, a temp-module packaging smoke test, `govulncheck`, and `golangci-lint`.

## Release Contract

The `0.1.0` release deliverables are:

- platform archives for the `openplanner` runner binary
- the single-file `openplanner` skill archive
- the Go module import path rooted at `github.com/yazanabuashour/openplanner`
- the direct-local Go package at `github.com/yazanabuashour/openplanner/sdk`
- a canonical source archive, SHA256 checksums, an SPDX SBOM, and GitHub
  attestations for release verification

Until `v0.1.0` exists, local development should use a local `replace` directive,
a pseudo-version from `main`, or `@main`. After the first tag lands, consumers
should use the tagged root module version.

## Repository Contents

- [CONTRIBUTING.md](CONTRIBUTING.md) explains how outside contributors should propose changes.
- [SECURITY.md](SECURITY.md) explains how to report vulnerabilities privately and what response timing to expect.
- [docs/maintainers.md](docs/maintainers.md) documents Beads-based maintainer workflow and repo administration notes.
- [docs/release-verification.md](docs/release-verification.md) explains the published release assets and how to verify them.
- [docs/agent-evals.md](docs/agent-evals.md) explains how to evaluate production agent workflows.
- [internal/runner](internal/runner) contains the JSON-friendly task facade for production agent workflows.
- [cmd/openplanner](cmd/openplanner) contains the installed JSON runner.
- [sdk](sdk) contains the direct local Go SDK.
- [skills/openplanner/SKILL.md](skills/openplanner/SKILL.md) is the portable Agent Skills-compatible OpenPlanner skill.
- [examples/openplanner/agenda](examples/openplanner/agenda) demonstrates a local recurring event/task workflow.
- [LICENSE](LICENSE) defines the project license.

## Contributing

Outside contributors can work entirely through GitHub issues and pull requests.
Beads is maintainer-only workflow tooling and is not required for community
contributions.

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution expectations and
[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for community standards.
