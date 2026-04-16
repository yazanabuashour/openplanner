---
name: openplanner
description: Manage local calendar and task workflows through OpenPlanner's ergonomic in-process Go SDK. Use this skill when an agent needs to create calendars, schedule events or tasks, query agenda ranges, or complete task occurrences without starting a daemon or calling a hosted service.
license: MIT
compatibility: Requires Go 1.26.2+ and local filesystem access for SQLite storage. OpenPlanner runs in process and does not require a daemon, localhost service, auth flow, or runtime network access.
---

# OpenPlanner Agent SDK

Use this skill for local-first planning tasks. The production agent path is the
hand-written SDK facade on top of `sdk.OpenLocal(...)`, not raw generated
OpenAPI request builders.

## Default Path

- Install from the current development line until a release tag exists:
  `go get github.com/yazanabuashour/openplanner@main`.
- Import `github.com/yazanabuashour/openplanner/sdk`.
- Open local data with `sdk.OpenLocal(sdk.Options{})`.
- Use `EnsureCalendar`, `CreateEvent`, `CreateTask`, `ListAgenda`,
  `ListEvents`, `ListTasks`, and `CompleteTask` for routine planning work.
- Use `sdk.Options{DatabasePath: "..."}` only when the user names a specific
  database or you are using an isolated test database.

Do not inspect `sdk/generated`, generated request builders, the Go module cache,
or large dependency directories for routine add/list/agenda/complete tasks. Use
targeted repo searches only when the SDK facade does not cover the user's ask.

## Common Workflow

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/yazanabuashour/openplanner/sdk"
)

func main() {
	api, err := sdk.OpenLocal(sdk.Options{})
	if err != nil {
		log.Fatal(err)
	}
	defer api.Close()

	ctx := context.Background()
	calendar, err := api.EnsureCalendar(ctx, sdk.CalendarInput{Name: "Personal"})
	if err != nil {
		log.Fatal(err)
	}

	startAt := time.Date(2026, 4, 16, 9, 0, 0, 0, time.Local)
	endAt := startAt.Add(time.Hour)
	if _, err := api.CreateEvent(ctx, sdk.EventInput{
		CalendarID: calendar.Calendar.ID,
		Title:      "Standup",
		StartAt:    &startAt,
		EndAt:      &endAt,
	}); err != nil {
		log.Fatal(err)
	}

	agenda, err := api.ListAgenda(ctx, sdk.AgendaOptions{
		From: time.Date(2026, 4, 16, 0, 0, 0, 0, time.Local),
		To:   time.Date(2026, 4, 17, 0, 0, 0, 0, time.Local),
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("agenda items=%d", len(agenda.Items))
}
```

Use `EnsureCalendar` before writing events or tasks. It returns `created`,
`already_exists`, or `updated` and avoids duplicate calendar setup.

## Task And Agenda Recipes

Copyable task snippets live at [references/planning.md](references/planning.md).
Use those examples for all-day events, timed tasks, recurring tasks, agenda
windows, and task completion.

## Generated Client Fallback

The generated OpenAPI client remains embedded on `sdk.Client` for advanced
API-contract work, HTTP compatibility checks, or endpoints not yet covered by
the SDK facade. Do not start there for common agent tasks.

See [the reference guide](references/REFERENCE.md) for runtime details and
entrypoints.
