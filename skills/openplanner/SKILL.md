---
name: openplanner
description: Manage local calendar and task workflows through OpenPlanner's installed JSON runner; reject ambiguous dates, invalid times, missing titles, invalid ranges, unsupported recurrence, and non-positive limits directly before tools.
license: MIT
compatibility: Requires local filesystem access and an installed openplanner binary on PATH.
metadata: { openplanner.repo: "https://github.com/yazanabuashour/openplanner", openplanner.runner: "openplanner planning", openplanner.skillArchive: "openplanner_<version>_skill.tar.gz" }
---

# OpenPlanner

Use this skill for local-first calendar and task planning. The production agent
interface is the installed JSON runner:

```bash
openplanner planning
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
- `update_calendar`
- `update_event`
- `update_task`
- `list_agenda`
- `list_events`
- `list_tasks`
- `complete_task`
- `validate`

For event and task creation, prefer `calendar_name`. The runner ensures that
calendar internally so agents do not need to discover or shuttle calendar IDs.
For updates, use the object ID returned by a prior list/create result, except
`update_calendar`, which may identify the current calendar by exactly one of
`calendar_id` or `calendar_name`.

Update payloads use patch semantics. Omit a field to preserve it, send a
non-null value to set it, and send `null` to clear clearable optional fields.
Do not use empty strings as clear instructions. Required fields such as event
and task titles can be changed but cannot be cleared.

For unsupported OpenPlanner workflows, say the production OpenPlanner skill does
not support that workflow yet. Do not switch to another interface unless the
user explicitly asks for one. Import/export, delete actions, reminders, and
task priority/status/tags are not supported until the installed JSON runner
ships those actions or fields.

## Reject Before Tools

For the cases below, reject or clarify directly without running code, inspecting
files, searching the repo, checking the database, using the runner, or calling
any CLI when the request has:

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
source files, tests, Go module-cache docs, or large dependency directories to
rediscover request/result shapes before the first task run. Only search the
repository if the runner fails in a way that requires debugging the local
checkout.

## Runner Pattern

Pipe one JSON request to `openplanner planning` and answer only from JSON
`writes`, `calendars`, `events`, `tasks`, `agenda`, or `rejection_reason`.
Agenda results are already chronologically ordered.

Calendars:

```json
{"action":"ensure_calendar","calendar_name":"Personal"}
{"action":"update_calendar","calendar_name":"Work","description":null,"color":"#2563EB"}
```

Events:

```json
{"action":"create_event","calendar_name":"Work","title":"Standup","start_at":"2026-04-16T09:00:00Z","end_at":"2026-04-16T10:00:00Z"}
{"action":"create_event","calendar_name":"Personal","title":"Planning day","start_date":"2026-04-17"}
{"action":"create_event","calendar_name":"Work","title":"Daily standup","start_at":"2026-04-16T09:00:00Z","end_at":"2026-04-16T09:30:00Z","recurrence":{"frequency":"daily","count":3}}
{"action":"create_event","calendar_name":"Work","title":"Weekly sync","start_at":"2026-04-13T09:00:00Z","end_at":"2026-04-13T09:30:00Z","recurrence":{"frequency":"weekly","by_weekday":["MO","WE"],"count":4}}
{"action":"update_event","event_id":"<id-from-prior-runner-result>","location":null,"recurrence":null}
{"action":"update_event","event_id":"<id-from-prior-runner-result>","start_at":null,"end_at":null,"start_date":"2026-04-17"}
```

Tasks:

```json
{"action":"create_task","calendar_name":"Personal","title":"Review notes","due_date":"2026-04-16"}
{"action":"create_task","calendar_name":"Work","title":"Send summary","due_at":"2026-04-16T11:00:00Z"}
{"action":"create_task","calendar_name":"Personal","title":"Daily review","due_date":"2026-04-16","recurrence":{"frequency":"daily","count":3}}
{"action":"create_task","calendar_name":"Personal","title":"Pay rent","due_date":"2026-01-31","recurrence":{"frequency":"monthly","by_month_day":[31],"count":3}}
{"action":"update_task","task_id":"<id-from-prior-runner-result>","due_date":null,"due_at":"2026-04-16T11:00:00Z","recurrence":null}
{"action":"complete_task","task_id":"<id-from-prior-runner-result>"}
{"action":"complete_task","task_id":"<id-from-prior-runner-result>","occurrence_date":"2026-04-17"}
```

Lists:

```json
{"action":"list_agenda","from":"2026-04-16T00:00:00Z","to":"2026-04-17T00:00:00Z","limit":100}
{"action":"list_events","calendar_name":"Work","limit":1}
{"action":"list_tasks","calendar_name":"Personal","limit":1}
```

Use strict `YYYY-MM-DD` date-only values for all-day events, date-based tasks,
and occurrence dates. Use RFC3339 values for timed fields such as `start_at`,
`end_at`, `due_at`, `from`, `to`, and `occurrence_at`.

Recurrence supports `daily`, `weekly`, and `monthly` with optional positive
`interval`, positive `count`, one of `until_at` or `until_date`, weekly-only
`by_weekday` values (`MO`, `TU`, `WE`, `TH`, `FR`, `SA`, `SU`), and
monthly-only `by_month_day` values from 1 through 31.
