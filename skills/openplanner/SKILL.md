---
name: openplanner
description: Manage local calendar and task workflows through OpenPlanner's in-process Go SDK. Use this skill when an agent needs to create calendars, schedule recurring events or tasks, query agenda ranges, or complete task occurrences without starting a daemon or calling a hosted service.
license: MIT
compatibility: Requires Go 1.26.2+ and local filesystem access for SQLite storage. OpenPlanner runs in process and does not require a daemon, localhost service, auth flow, or runtime network access.
---

# OpenPlanner

Use this skill when you need local planning state in an agent or Go program and the repository or environment already has access to the OpenPlanner module.

## Activate when

- You need to create or list calendars through the Go SDK.
- You need recurring events or recurring tasks backed by local SQLite state.
- You need to query an agenda window or complete recurring task occurrences.
- You need an in-process planning API instead of a hosted service or background daemon.

## Workflow

1. Open a client with `sdk.OpenLocal(sdk.Options{})` or with an explicit `DatabasePath` for tests and throwaway runs.
2. Use `sdk/generated` request types with the generated client APIs.
3. Create or list calendars before writing events or tasks.
4. Create recurring events or recurring tasks as needed.
5. Query `AgendaAPI.ListAgenda(...)` for the target time range.
6. Complete task occurrences through `TasksAPI.CompleteTask(...)` when needed.

## Common queries

Use the default database path when answering questions about live local planning state:

```go
databasePath, err := sdk.DefaultDatabasePath()
if err != nil {
	panic(err)
}

client, err := sdk.OpenLocal(sdk.Options{DatabasePath: databasePath})
if err != nil {
	panic(err)
}
defer client.Close()
```

List calendars:

```go
page, _, err := client.CalendarsAPI.ListCalendars(ctx).Execute()
if err != nil {
	panic(err)
}
for _, calendar := range page.Items {
	fmt.Printf("%s\t%s\n", calendar.Id, calendar.Name)
}
```

List tasks and events. Add `.CalendarId(calendarID)` before `.Execute()` when the user asks for one calendar:

```go
tasks, _, err := client.TasksAPI.ListTasks(ctx).Limit(200).Execute()
if err != nil {
	panic(err)
}
events, _, err := client.EventsAPI.ListEvents(ctx).Limit(200).Execute()
if err != nil {
	panic(err)
}
fmt.Printf("tasks=%d events=%d\n", len(tasks.Items), len(events.Items))
```

Query an agenda window:

```go
from := time.Date(2026, 4, 16, 0, 0, 0, 0, time.Local)
to := from.AddDate(0, 0, 7)
agenda, _, err := client.AgendaAPI.ListAgenda(ctx).From(from).To(to).Limit(200).Execute()
if err != nil {
	panic(err)
}
for _, item := range agenda.Items {
	fmt.Printf("%s\t%s\t%s\n", item.Kind, item.OccurrenceKey, item.Title)
}
```

Complete a task occurrence:

```go
occurrenceDate := "2026-04-16"
_, _, err := client.TasksAPI.CompleteTask(ctx, taskID).
	CompleteTaskRequest(generated.CompleteTaskRequest{OccurrenceDate: &occurrenceDate}).
	Execute()
if err != nil {
	panic(err)
}
```

For a runnable read-only helper, use `go run ./examples/openplanner/query --from <RFC3339> --to <RFC3339>`.

## Install notes

- The repository does not have a release tag yet, so current consumers should use a local checkout with a `replace` directive or a pseudo-version from `main`.
- Future tagged installs will use `go get github.com/yazanabuashour/openplanner/sdk@v0.1.0` or later, but that is not the current primary path.

See [the reference guide](references/REFERENCE.md) for runtime details, entrypoints, and the example workflow.
