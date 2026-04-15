# openplanner

`openplanner` is a spec-first local planning SDK for agent-facing calendar and task workflows. The public release surface is an embeddable Go module built around a checked-in OpenAPI contract, generated Go client, in-process local transport, and SQLite storage.

## Install in your Go project

The first planned public tag is `v0.1.0`. Once that tag is published, the intended consumer install command is:

```bash
go get github.com/yazanabuashour/openplanner/sdk@v0.1.0
```

Go resolves that package version from the root module tag at `github.com/yazanabuashour/openplanner`.

Then open the local runtime in process:

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/yazanabuashour/openplanner/sdk"
	"github.com/yazanabuashour/openplanner/sdk/generated"
)

func main() {
	client, err := sdk.OpenLocal(sdk.Options{})
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
	_, _, err = client.EventsAPI.CreateEvent(ctx).CreateEventRequest(generated.CreateEventRequest{
		CalendarId: calendar.Id,
		Title:      "Standup",
		StartAt:    &startAt,
		EndAt:      &endAt,
	}).Execute()
	if err != nil {
		panic(err)
	}

	fmt.Println(calendar.Id)
}
```

By default, `sdk.OpenLocal(sdk.Options{})` stores SQLite data at `${XDG_DATA_HOME:-~/.local/share}/openplanner/openplanner.db`. Set `DatabasePath` to override that location in tests or embedded deployments.

`cmd/openplanner` is a lightweight bootstrap banner for maintainers and debugging. It is not the primary product surface.

## Runtime model

- `sdk.OpenLocal(...)` keeps all calls in process through a local round tripper. It does not bind a port, open a localhost listener, or start a daemon.
- The generated base URL is a placeholder used for request construction only. It is not a reachable host service.
- `sdk.OpenLocal(sdk.Options{})` stores SQLite data at `${XDG_DATA_HOME:-~/.local/share}/openplanner/openplanner.db`.
- Set `sdk.Options.DatabasePath` to override that location. OpenPlanner stores SQLite data exactly at the resulting path.

## Release contract

The tagged release surface is the Go module at `github.com/yazanabuashour/openplanner`. GitHub Releases remain the human-readable release-note surface for those tags and publish source-only release metadata: a deterministic source archive, `SHA256SUMS`, an SPDX SBOM, and GitHub attestations.

Until `v0.1.0` exists, local development should use a local `replace` directive or a pseudo-version from `main`. After the first tag lands, the SDK package path above is the canonical install story.

## Repository contents

- [CONTRIBUTING.md](CONTRIBUTING.md) explains how outside contributors should propose changes.
- [SECURITY.md](SECURITY.md) explains how to report vulnerabilities privately and what response timing to expect.
- [docs/maintainers.md](docs/maintainers.md) documents Beads-based maintainer workflow and repo administration notes.
- [docs/release-verification.md](docs/release-verification.md) explains the published release assets and how to verify them.
- [openapi/openapi.yaml](openapi/openapi.yaml) is the source-of-truth API contract.
- [sdk/generated](sdk/generated) contains the checked-in generated Go client types and operations.
- [sdk](sdk) opens the generated client against the in-process local transport.
- [interfaces/openclaw/SKILL.md](interfaces/openclaw/SKILL.md) documents the current OpenClaw-facing install and usage path.
- [examples/openclaw/agenda](examples/openclaw/agenda) demonstrates a local recurring event/task workflow.
- [cmd/openplanner](cmd/openplanner) contains the lightweight CLI entrypoint.
- [LICENSE](LICENSE) defines the project license.

## Development

Install the pinned toolchain with:

```bash
mise install
```

Inspect the bootstrap CLI with:

```bash
go run ./cmd/openplanner
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
