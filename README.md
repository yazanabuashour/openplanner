# openplanner

`openplanner` is a spec-first local planning SDK for agent-facing calendar and task workflows. The first public artifact is a tagged Go module built around a checked-in OpenAPI contract, generated Go client, embedded local transport, and SQLite storage.

## Repository contents

- [CONTRIBUTING.md](CONTRIBUTING.md) explains how outside contributors should propose changes.
- [SECURITY.md](SECURITY.md) explains how to report vulnerabilities privately and what response timing to expect.
- [docs/maintainers.md](docs/maintainers.md) documents Beads-based maintainer workflow and repo administration notes.
- [openapi/openapi.yaml](openapi/openapi.yaml) is the source-of-truth API contract.
- [sdk/generated](sdk/generated) contains the checked-in generated Go client.
- [sdk](sdk) opens the generated client against the in-process local transport.
- [interfaces/openclaw/SKILL.md](interfaces/openclaw/SKILL.md) documents the current OpenClaw-facing install and usage path.
- [examples/openclaw/agenda](examples/openclaw/agenda) demonstrates a local recurring event/task workflow.
- [cmd/openplanner](cmd/openplanner) contains the lightweight CLI entrypoint.
- [LICENSE](LICENSE) defines the project license.

## Release contract

The initial release surface is a tagged Go module at `github.com/yazanabuashour/openplanner` in the `v0.y.z` range. GitHub Releases remain the human-readable release-note surface for those tags.

Consumers install a pinned version with:

```bash
go get github.com/yazanabuashour/openplanner@v0.y.z
```

Then open the local client in process:

```go
package main

import (
	"context"
	"time"

	"github.com/yazanabuashour/openplanner/sdk"
	"github.com/yazanabuashour/openplanner/sdk/generated"
)

func main() {
	client, err := sdk.OpenLocal(sdk.Options{DatabasePath: "./openplanner.db"})
	if err != nil {
		panic(err)
	}
	defer client.Close()

	ctx := context.Background()
	calendar, _, err := client.CalendarsAPI.CreateCalendar(ctx).
		CreateCalendarRequest(generated.CreateCalendarRequest{Name: "Personal"}).
		Execute()
	if err != nil {
		panic(err)
	}

	startAt := time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)
	endAt := startAt.Add(time.Hour)
	count := int32(3)
	_, _, err = client.EventsAPI.CreateEvent(ctx).CreateEventRequest(generated.CreateEventRequest{
		CalendarId: calendar.Id,
		Title:      "Standup",
		StartAt:    &startAt,
		EndAt:      &endAt,
		Recurrence: &generated.RecurrenceRule{
			Frequency: generated.RECURRENCEFREQUENCY_DAILY,
			Count:     &count,
		},
	}).Execute()
	if err != nil {
		panic(err)
	}
}
```

## Development

Install the pinned toolchain with:

```bash
mise install
```

Run the CLI with:

```bash
go run ./cmd/openplanner
```

Run the local quality gates with:

```bash
make check
```

Regenerate the checked-in client after changing `openapi/openapi.yaml`:

```bash
make openapi-generate
```

## Contributing

Outside contributors can work entirely through GitHub issues and pull requests. Beads is maintainer-only workflow tooling and is not required for community contributions.

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution expectations and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for community standards.
