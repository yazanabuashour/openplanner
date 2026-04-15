# OpenPlanner OpenClaw Notes

`openplanner` currently ships as a Go module for agent code that has access to the Go toolchain.

## Install

Pin a released tag:

```bash
go get github.com/yazanabuashour/openplanner@v0.y.z
```

## Runtime model

- Use `sdk.OpenLocal(...)` to open the embedded local transport against a SQLite database file.
- Use `sdk/generated` request and response types when constructing calls through the generated client.
- No daemon, auth flow, or user bootstrap is required in v1.

## Recommended entrypoints

- Contract: `openapi/openapi.yaml`
- Bootstrap: `sdk/client.go`
- Generated client: `sdk/generated/`
- Example: `examples/openclaw/agenda/main.go`

## Example workflow

1. Open a local client with `sdk.OpenLocal(sdk.Options{DatabasePath: "./openplanner.db"})`.
2. Create or list calendars.
3. Create recurring events and recurring tasks through the generated client.
4. Query `AgendaAPI.ListAgenda(...)` for a time range.
5. Complete recurring task occurrences through `TasksAPI.CompleteTask(...)`.
