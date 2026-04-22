---
name: openplanner
description: Manage local calendar and task workflows through OpenPlanner's installed JSON runner; reject ambiguous dates, year-first slash dates, invalid times, missing titles, invalid ranges, unsupported recurrence, and non-positive limits directly without tools or file reads. For valid requests, pipe JSON to the installed runner; never inspect source, SQLite, or module-cache docs before the first runner call.
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
- `delete_calendar`
- `delete_event`
- `delete_task`
- `create_event_task_link`
- `delete_event_task_link`
- `list_event_task_links`
- `cancel_event_occurrence`
- `reschedule_event_occurrence`
- `cancel_task_occurrence`
- `reschedule_task_occurrence`
- `list_agenda`
- `list_events`
- `list_tasks`
- `complete_task`
- `list_pending_reminders`
- `dismiss_reminder`
- `export_icalendar`
- `validate`

For event and task creation, prefer `calendar_name`. The runner ensures that
calendar internally so agents do not need to discover or shuttle calendar IDs.
For updates, use the object ID returned by a prior list/create result, except
`update_calendar`, which may identify the current calendar by exactly one of
`calendar_id` or `calendar_name`.

For deletes, use the object ID returned by a prior list/create result for
events and tasks. If the user identifies an event or task by title, list the
matching items first, then delete by the returned `event_id` or `task_id`.
Calendars may be deleted by exactly one of `calendar_id` or `calendar_name`,
but only when they are empty; calendar deletion never cascades to contained
events or tasks.

Use event-task links for explicit prep or follow-up relationships. Create and
delete links with both `event_id` and `task_id`; list links with optional
`event_id` and/or `task_id`. Event results expose `linked_task_ids`, task
results expose `linked_event_ids`, and agenda items expose whichever linked IDs
apply to that item.

Update payloads use patch semantics. Omit a field to preserve it, send a
non-null value to set it, and send `null` to clear clearable optional fields.
Do not use empty strings as clear instructions. Required fields such as event
and task titles can be changed but cannot be cleared.

For unsupported OpenPlanner workflows, say the production OpenPlanner skill does
not support that workflow yet. Do not switch to another interface unless the
user explicitly asks for one. iCalendar export is supported through
`export_icalendar`, which returns `.ics` text in JSON; iCalendar import is not
supported yet. Event `time_zone` maps to iCalendar `TZID` during export and
future import work.

Tasks support metadata. Use `priority` values `low`, `medium`, or `high`;
`status` values `todo`, `in_progress`, or `done`; and `tags` as lowercase labels
containing only letters, digits, `_`, or `-`. Invalid task metadata values should
be rejected directly before tools.

Events and tasks support relative reminders with `reminders`, an array of
objects such as `{"before_minutes":60}`. Reminder offsets must be positive and
cannot be duplicated on one event or task. For pending reminder requests, call
`list_pending_reminders` with RFC3339 `from` and `to` bounds. To dismiss a
pending reminder occurrence, call `dismiss_reminder` with the returned
`reminder_occurrence_id`; repeated dismissals are idempotent.

Timed events may include `time_zone` with an IANA timezone name such as
`America/New_York`. Keep all timed fields as strict RFC3339 values, and make
sure their numeric offsets match the named zone. Use `time_zone` only with
timed event fields, never with all-day event dates.

## Reject Before Tools

For the cases below, reject or clarify directly without running code, inspecting
files, searching the repo, checking the database, using the runner, or calling
any CLI when the request has:

| Issue | Response |
| --- | --- |
| ambiguous short date without year context, like `04/16` | ask for the year |
| year-first slash date, like `2026/04/16` | require `YYYY-MM-DD` |
| invalid RFC3339 time, like `2026-04-16 09:00` | require RFC3339 |
| invalid event timezone, like `Not/AZone` or `Local` | require an IANA timezone |
| event timezone with all-day fields | require timed event fields |
| RFC3339 offset that does not match `time_zone` | require a matching offset |
| missing required event/task title | ask for the title |
| invalid agenda range where `from` is after `to` | reject the range |
| unsupported recurrence, like hourly | support only daily, weekly, monthly |
| non-positive limit | require a positive limit |
| invalid task priority | require `low`, `medium`, or `high` |
| invalid task status | require `todo`, `in_progress`, or `done` |
| invalid task tags with spaces or punctuation | require lowercase letters, digits, `_`, or `-` |
| non-positive reminder offset | require a positive `before_minutes` value |
| duplicate reminder offsets on one item | require one reminder per offset |
| invalid event attendee email | require a plain email address |
| duplicate event attendee emails | require one attendee per email |
| invalid event attendee role | require `required`, `optional`, `chair`, or `non_participant` |
| invalid event attendee participation status | require `needs_action`, `accepted`, `declined`, `tentative`, or `delegated` |

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
`writes`, `calendars`, `events`, `tasks`, `agenda`, `reminders`,
`event_task_links`, `icalendar`, or
`rejection_reason`. Agenda results are already chronologically ordered. Pending
reminder results are already chronologically ordered.

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
{"action":"create_event","calendar_name":"Work","title":"Weekly New York sync","start_at":"2026-03-03T09:00:00-05:00","time_zone":"America/New_York","recurrence":{"frequency":"weekly","count":2}}
{"action":"create_event","calendar_name":"Work","title":"Standup","start_at":"2026-04-16T09:00:00Z","reminders":[{"before_minutes":30}]}
{"action":"create_event","calendar_name":"Work","title":"Planning","start_at":"2026-04-16T09:00:00Z","attendees":[{"email":"alex@example.com","display_name":"Alex Rivera","role":"required","participation_status":"accepted","rsvp":true}]}
{"action":"update_event","event_id":"<id-from-prior-runner-result>","location":null,"recurrence":null}
{"action":"update_event","event_id":"<id-from-prior-runner-result>","start_at":null,"end_at":null,"start_date":"2026-04-17"}
{"action":"update_event","event_id":"<id-from-prior-runner-result>","reminders":null}
{"action":"update_event","event_id":"<id-from-prior-runner-result>","attendees":null}
{"action":"cancel_event_occurrence","event_id":"<id-from-prior-runner-result>","occurrence_at":"2026-04-17T09:00:00Z"}
{"action":"reschedule_event_occurrence","event_id":"<id-from-prior-runner-result>","occurrence_at":"2026-04-18T09:00:00Z","start_at":"2026-04-19T11:00:00Z"}
{"action":"delete_event","event_id":"<id-from-prior-runner-result>"}
```

Tasks:

```json
{"action":"create_task","calendar_name":"Personal","title":"Review notes","due_date":"2026-04-16"}
{"action":"create_task","calendar_name":"Work","title":"Send summary","due_at":"2026-04-16T11:00:00Z"}
{"action":"create_task","calendar_name":"Personal","title":"Review notes","due_date":"2026-04-16","priority":"high","status":"in_progress","tags":["planning","review"]}
{"action":"create_task","calendar_name":"Personal","title":"Daily review","due_date":"2026-04-16","recurrence":{"frequency":"daily","count":3}}
{"action":"create_task","calendar_name":"Personal","title":"Pay rent","due_date":"2026-01-31","recurrence":{"frequency":"monthly","by_month_day":[31],"count":3}}
{"action":"create_task","calendar_name":"Personal","title":"Take medicine","due_at":"2026-04-16T10:00:00Z","reminders":[{"before_minutes":60}]}
{"action":"update_task","task_id":"<id-from-prior-runner-result>","due_date":null,"due_at":"2026-04-16T11:00:00Z","recurrence":null}
{"action":"update_task","task_id":"<id-from-prior-runner-result>","priority":"medium","tags":null}
{"action":"update_task","task_id":"<id-from-prior-runner-result>","reminders":null}
{"action":"complete_task","task_id":"<id-from-prior-runner-result>"}
{"action":"complete_task","task_id":"<id-from-prior-runner-result>","occurrence_date":"2026-04-17"}
{"action":"complete_task","task_id":"<id-from-prior-runner-result>","occurrence_key":"<occurrence-key-from-agenda>"}
{"action":"cancel_task_occurrence","task_id":"<id-from-prior-runner-result>","occurrence_date":"2026-04-17"}
{"action":"reschedule_task_occurrence","task_id":"<id-from-prior-runner-result>","occurrence_date":"2026-04-18","due_date":"2026-04-19"}
{"action":"delete_task","task_id":"<id-from-prior-runner-result>"}
```

Event-task links:

```json
{"action":"create_event_task_link","event_id":"<id-from-prior-runner-result>","task_id":"<id-from-prior-runner-result>"}
{"action":"list_event_task_links","event_id":"<id-from-prior-runner-result>"}
{"action":"list_event_task_links","task_id":"<id-from-prior-runner-result>"}
{"action":"delete_event_task_link","event_id":"<id-from-prior-runner-result>","task_id":"<id-from-prior-runner-result>"}
```

Lists:

```json
{"action":"list_agenda","from":"2026-04-16T00:00:00Z","to":"2026-04-17T00:00:00Z","limit":100}
{"action":"list_events","calendar_name":"Work","limit":1}
{"action":"list_tasks","calendar_name":"Personal","limit":1}
{"action":"list_tasks","calendar_name":"Work","priority":"high","status":"in_progress","tags":["planning","review"],"limit":10}
{"action":"list_pending_reminders","from":"2026-04-16T08:00:00Z","to":"2026-04-16T10:00:00Z","limit":10}
{"action":"dismiss_reminder","reminder_occurrence_id":"<id-from-list-pending-reminders-result>"}
{"action":"export_icalendar"}
{"action":"export_icalendar","calendar_name":"Work"}
```

For export results, use `icalendar.content_type`, `icalendar.filename`,
`icalendar.event_count`, `icalendar.task_count`, and `icalendar.content`.
The runner returns complete `.ics` text in JSON and does not write files.

Deletes:

```json
{"action":"delete_calendar","calendar_name":"Archive"}
{"action":"delete_calendar","calendar_id":"<id-from-prior-runner-result>"}
```

Use strict `YYYY-MM-DD` date-only values for all-day events, date-based tasks,
and occurrence dates. Use RFC3339 values for timed fields such as `start_at`,
`end_at`, `due_at`, `from`, `to`, and `occurrence_at`.

Recurrence supports `daily`, `weekly`, and `monthly` with optional positive
`interval`, positive `count`, one of `until_at` or `until_date`, weekly-only
`by_weekday` values (`MO`, `TU`, `WE`, `TH`, `FR`, `SA`, `SU`), and
monthly-only `by_month_day` values from 1 through 31.
