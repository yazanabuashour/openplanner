# OpenPlanner Reference

`openplanner` ships as an embeddable Go module for agent code that has access to the Go toolchain.

## Current install path

There are currently no repository tags. Until the first release tag exists, use one of these approaches:

- Work from a local checkout and add a `replace` directive for `github.com/yazanabuashour/openplanner`.
- Depend on a pseudo-version from `main` if your environment resolves Go modules that way.

The first planned tagged install path is:

```bash
go get github.com/yazanabuashour/openplanner/sdk@v0.1.0
```

Treat that command as forward-looking release documentation until the tag exists.

## Runtime model

- OpenPlanner runs entirely in process through `sdk.OpenLocal(...)`.
- The generated base URL is a request-construction placeholder, not a reachable service.
- No daemon, auth flow, host port, user bootstrap, or background process is required.
- `sdk.OpenLocal(sdk.Options{})` stores SQLite data under `${XDG_DATA_HOME:-~/.local/share}/openplanner/openplanner.db`.
- Set `sdk.Options.DatabasePath` to override the SQLite path explicitly.

## Agent quick start

- For live local state, call `sdk.DefaultDatabasePath()` and pass the result as `sdk.Options.DatabasePath`.
- For tests, demos, and examples, pass an explicit throwaway `DatabasePath` so the run is isolated and rerunnable.
- Use `CalendarsAPI.ListCalendars`, `EventsAPI.ListEvents`, `TasksAPI.ListTasks`, and `AgendaAPI.ListAgenda` for read-only answers before writing new data.
- Use `TasksAPI.CompleteTask` only when the user asks to mark a specific task occurrence complete.
- Use `go run ./examples/openplanner/query --from <RFC3339> --to <RFC3339>` for a compact read-only calendar and agenda dump.
- Do not call or probe `http://openplanner.invalid`; it is only the generated client's placeholder base URL.

## Recommended repo entrypoints

- `openapi/openapi.yaml`
- `sdk/client.go`
- `sdk/generated/`
- `examples/openplanner/agenda/main.go`
- `examples/openplanner/query/main.go`

## Example workflow

1. Open a local client with `sdk.OpenLocal(sdk.Options{})`.
2. Create or list calendars.
3. Create recurring events and recurring tasks through the generated client.
4. Query `AgendaAPI.ListAgenda(...)` for a time range.
5. Complete recurring task occurrences through `TasksAPI.CompleteTask(...)`.
