---
name: openplanner
description: Manage local calendar and task workflows through OpenPlanner's AgentOps JSON runner; reject ambiguous dates, invalid times, missing titles, invalid ranges, unsupported recurrence, and non-positive limits directly before tools.
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
are using an isolated test database. For isolated runs, set
`OPENPLANNER_DATABASE_PATH=<path>` or pass `--db <path>`; `--db` wins when both
are present.

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

## Reject Before Tools

For the cases below, reject or clarify directly without running code, opening
references, inspecting files, searching the repo, checking the database, using
the AgentOps runner, or calling any CLI when the request has:

| Issue | Response |
| --- | --- |
| ambiguous short date without year context, like `04/16` | ask for the year |
| year-first slash date, like `2026/04/16` | require `YYYY-MM-DD` |
| invalid RFC3339 time, like `2026-04-16 09:00` | require RFC3339 |
| missing required event/task title | ask for the title |
| invalid agenda range where `from` is after `to` | reject the range |
| unsupported recurrence, like hourly | support only daily, weekly, monthly |
| non-positive limit | require a positive limit |

Never convert a year-first slash date to dashed ISO form; reject it. Never
convert an invalid RFC3339 time like `2026-04-16 09:00` to
`2026-04-16T09:00:00Z`; reject it. Explicit month/day/year dates with a year,
such as `04/16/2026`, may be normalized to `2026-04-16`.

Do not write local OpenPlanner data through SQLite directly. Do not inspect
generated API bindings, generated request builders, the Go module cache, or
large dependency directories for routine planning tasks. This skill and its
linked reference are the routine task contract; do not inspect source files,
tests, generated code, or module-cache docs to rediscover request/result shapes
before the first task run. Only search the repository if the AgentOps runner
fails in a way that requires debugging the local checkout.

## Runner Pattern

Use this shape for supported tasks, changing only the JSON payload:

```bash
printf '%s\n' '{"action":"list_agenda","from":"2026-04-16T00:00:00Z","to":"2026-04-17T00:00:00Z"}' \
  | go run ./cmd/openplanner-agentops planning
```

Common one-line payloads:
`{"action":"ensure_calendar","calendar_name":"Personal"}`;
`{"action":"create_event","calendar_name":"Work","title":"Standup","start_at":"2026-04-16T09:00:00Z","end_at":"2026-04-16T10:00:00Z"}`;
`{"action":"create_task","calendar_name":"Personal","title":"Review notes","due_date":"2026-04-16"}`;
`{"action":"create_event","calendar_name":"Work","title":"Daily standup","start_at":"2026-04-16T09:00:00Z","end_at":"2026-04-16T09:30:00Z","recurrence":{"frequency":"daily","count":3}}`;
`{"action":"create_task","calendar_name":"Personal","title":"Daily review","due_date":"2026-04-16","recurrence":{"frequency":"daily","count":3}}`;
`{"action":"list_agenda","from":"2026-04-16T00:00:00Z","to":"2026-04-17T00:00:00Z","limit":100}`;
`{"action":"list_events","calendar_name":"Work","limit":1}`;
`{"action":"list_tasks","calendar_name":"Personal","limit":1}`;
`{"action":"complete_task","task_id":"<id-from-prior-runner-result>"}`;
`{"action":"complete_task","task_id":"<id-from-prior-runner-result>","occurrence_date":"2026-04-17"}`.

Use strict `YYYY-MM-DD` date-only values for all-day events, date-based tasks,
and occurrence dates. Use RFC3339 values for timed fields such as `start_at`,
`end_at`, `due_at`, `from`, `to`, and `occurrence_at`.

When reporting results, answer from JSON `writes`, `calendars`, `events`,
`tasks`, `agenda`, or `rejection_reason`. Agenda results are already
chronologically ordered.

Copyable request examples live at [references/planning.md](references/planning.md).
