# OpenPlanner Agent Reference

The production agent path is the ergonomic Go SDK facade in
`github.com/yazanabuashour/openplanner/sdk`.

## Agent Quick Start

- Use `sdk.OpenLocal(sdk.Options{})` for live local data. It opens the default
  SQLite database and serves the OpenAPI handler in process.
- Use `sdk.DefaultDatabasePath()` only when you need to report or verify the
  default database path.
- Use explicit `sdk.Options{DatabasePath: "..."}` for tests, fixtures, and
  throwaway examples.
- Use `EnsureCalendar`, `CreateEvent`, `CreateTask`, `ListAgenda`,
  `ListEvents`, `ListTasks`, and `CompleteTask` for routine planning work.
- Use generated OpenAPI methods only for endpoints not covered by the SDK
  facade or when the user explicitly needs raw API-contract behavior.

## Runtime Model

- OpenPlanner runs entirely in process through `sdk.OpenLocal(...)`.
- The generated base URL is a request-construction placeholder, not a reachable
  service.
- No daemon, auth flow, host port, user bootstrap, or background process is
  required.
- `sdk.OpenLocal(sdk.Options{})` stores SQLite data under
  `${XDG_DATA_HOME:-~/.local/share}/openplanner/openplanner.db`.

## Planning Tasks

- Use `EnsureCalendar` before creating tasks or events. It matches calendars by
  trimmed name and updates description or color only when those fields are
  provided and changed.
- Use `CreateEvent` for timed or all-day event series.
- Use `CreateTask` for dated, timed, one-off, or recurring task series.
- Use `ListAgenda` for "what is on my schedule?" questions. Agenda results are
  ordered by the service's chronological sort key.
- Use `CompleteTask` only when the user asks to mark a specific task or
  recurrence occurrence complete.

Copyable snippets live at [planning.md](planning.md).

## Recommended Repo Entrypoints

- `sdk/client.go`
- `sdk/helpers.go`
- `skills/openplanner/references/planning.md`
- `examples/openplanner/query/main.go`
