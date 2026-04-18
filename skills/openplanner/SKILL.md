---
name: openplanner
description: Manage local calendar and task workflows through OpenPlanner's AgentOps JSON runner. Use this skill when an agent needs to create calendars, schedule events or tasks, query agenda ranges, list events/tasks, or complete task occurrences without starting a daemon or calling a hosted service.
license: MIT
compatibility: Requires Go 1.26.2+ and local filesystem access for SQLite storage. OpenPlanner runs in process and does not require a daemon, localhost service, auth flow, or runtime network access.
---

# OpenPlanner AgentOps

Use this skill for local-first planning tasks. The production agent interface is
the machine-facing JSON runner:

```bash
go run ./cmd/openplanner-agentops planning
```

Send one JSON request on stdin and answer only from the JSON result on stdout.
Use the default database path unless the user names a specific database or you
are using an isolated test database; for tests/manual debugging, pass
`--db <path>`.

Supported routine actions are:

- `ensure_calendar`
- `create_event`
- `create_task`
- `list_agenda`
- `list_events`
- `list_tasks`
- `complete_task`
- `validate`

For event and task creation, prefer `calendar_name`. The runner ensures that
calendar internally so agents do not need to discover or shuttle calendar IDs.

For unsupported OpenPlanner workflows, say the production AgentOps skill does
not support that workflow yet. Do not switch to another interface unless the
user explicitly asks for one.

Do not write local OpenPlanner data through SQLite directly. Do not inspect
generated API bindings, generated request builders, the Go module cache, or
large dependency directories for routine planning tasks. The linked reference is
the routine task contract; do not inspect source files, tests, generated code,
or module-cache docs to rediscover request/result shapes before the first task
run. Only search the repository if the AgentOps runner fails in a way that
requires debugging the local checkout.

## Runner Pattern

Use this shape for supported tasks, changing only the JSON payload:

```bash
printf '%s\n' '{"action":"list_agenda","from":"2026-04-16T00:00:00Z","to":"2026-04-17T00:00:00Z"}' \
  | go run ./cmd/openplanner-agentops planning
```

Use strict `YYYY-MM-DD` date-only values for all-day events, date-based tasks,
and occurrence dates. Use RFC3339 values for timed fields such as `start_at`,
`end_at`, `due_at`, `from`, `to`, and `occurrence_at`.

If the user gives an ambiguous short date like `04/16` without enough year
context, ask for the year before writing. If the user gives a year-first slash
date such as `2026/04/16`, reject it instead of rewriting it. Explicit
month/day/year dates with a year, such as `04/16/2026`, may be converted to
`YYYY-MM-DD`.

When reporting results, answer from JSON `writes`, `calendars`, `events`,
`tasks`, `agenda`, or `rejection_reason`. Agenda results are already
chronologically ordered.

Copyable request examples live at [references/planning.md](references/planning.md).
