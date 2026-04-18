# openplanner

`openplanner` is a spec-first local planning runtime for agent-facing calendar and task workflows. The public release surface is an embeddable Go SDK plus a machine-facing AgentOps JSON runner, both built on a checked-in OpenAPI contract, generated Go client, in-process local transport, and SQLite storage.

## Install in your Go project

Until the first release tag is published, install the current development line
and import the SDK package from it:

```bash
go get github.com/yazanabuashour/openplanner@main
```

Then open the local runtime in process:

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
	_, err = client.CreateEvent(ctx, sdk.EventInput{
		CalendarID: calendar.Calendar.ID,
		Title:      "Standup",
		StartAt:    &startAt,
		EndAt:      &endAt,
	})
	if err != nil {
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

By default, `sdk.OpenLocal(sdk.Options{})` stores SQLite data at `${XDG_DATA_HOME:-~/.local/share}/openplanner/openplanner.db`. Set `DatabasePath` to override that location in tests or embedded deployments.

For common local planning tasks, prefer the ergonomic helper methods on the SDK
client over generated OpenAPI method names: `EnsureCalendar`, `CreateEvent`,
`CreateTask`, `ListAgenda`, `ListCalendars`, `ListEvents`, `ListTasks`, and
`CompleteTask`. The embedded generated client remains available as the
API-contract substrate for advanced compatibility work.

For skill-driven agent workflows, use the JSON runner instead of generated
request builders or ad hoc command discovery:

```bash
printf '%s\n' '{"action":"list_agenda","from":"2026-04-16T00:00:00Z","to":"2026-04-17T00:00:00Z"}' \
  | go run ./cmd/openplanner-agentops planning
```

## Runtime model

- `sdk.OpenLocal(...)` keeps all calls in process through a local round tripper. It does not bind a port, open a localhost listener, or start a daemon.
- The generated base URL is a placeholder used for request construction only. It is not a reachable host service.
- `sdk.OpenLocal(sdk.Options{})` stores SQLite data at `${XDG_DATA_HOME:-~/.local/share}/openplanner/openplanner.db`.
- Set `sdk.Options.DatabasePath` to override that location. OpenPlanner stores SQLite data exactly at the resulting path.

## Release contract

The tagged release surface is the Go module at `github.com/yazanabuashour/openplanner`. GitHub Releases remain the human-readable release-note surface for those tags and publish source-only release metadata: a deterministic source archive, `SHA256SUMS`, an SPDX SBOM, and GitHub attestations.

Until `v0.1.0` exists, local development should use a local `replace` directive, a pseudo-version from `main`, or `@main`. After the first tag lands, consumers should use the tagged root module version.

## Repository contents

- [CONTRIBUTING.md](CONTRIBUTING.md) explains how outside contributors should propose changes.
- [SECURITY.md](SECURITY.md) explains how to report vulnerabilities privately and what response timing to expect.
- [docs/maintainers.md](docs/maintainers.md) documents Beads-based maintainer workflow and repo administration notes.
- [docs/release-verification.md](docs/release-verification.md) explains the published release assets and how to verify them.
- [docs/agent-evals.md](docs/agent-evals.md) explains how to evaluate production agent workflows without mixing comparison variants into the production skill.
- [agentops](agentops) contains the JSON-friendly task facade for production agent workflows.
- [cmd/openplanner-agentops](cmd/openplanner-agentops) contains the machine-facing JSON runner for agents.
- [openapi/openapi.yaml](openapi/openapi.yaml) is the source-of-truth API contract.
- [sdk/generated](sdk/generated) contains the checked-in generated Go client types and operations.
- [sdk](sdk) opens the generated client against the in-process local transport.
- [skills/openplanner/SKILL.md](skills/openplanner/SKILL.md) documents the Agent Skills-compatible OpenPlanner install and usage path.
- [examples/openplanner/agenda](examples/openplanner/agenda) demonstrates a local recurring event/task workflow.
- [LICENSE](LICENSE) defines the project license.

## Development

Install the pinned toolchain with:

```bash
mise install
```

Exercise the AgentOps runner with:

```bash
printf '%s\n' '{"action":"validate"}' | go run ./cmd/openplanner-agentops planning
```

Run the local quality gates with:

```bash
make check
```

`make check` runs formatting validation, OpenAPI lint/checks, the Go test suite, a temp-module packaging smoke test, `govulncheck`, and `golangci-lint`.

Regenerate the checked-in client after changing `openapi/openapi.yaml`:

```bash
make openapi-generate
```

## Contributing

Outside contributors can work entirely through GitHub issues and pull requests. Beads is maintainer-only workflow tooling and is not required for community contributions.

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution expectations and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for community standards.
