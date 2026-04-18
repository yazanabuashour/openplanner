# Planning Task JSON Recipes

Pipe one request object into the production runner:

```bash
printf '%s\n' '<json>' | go run ./cmd/openplanner-agentops planning
```

For an isolated database used in tests or manual debugging:

```bash
printf '%s\n' '<json>' | go run ./cmd/openplanner-agentops planning --db openplanner-test.db
```

## Ensure A Calendar

```json
{"action":"ensure_calendar","calendar_name":"Personal"}
```

Optional calendar fields are `description` and `color` (`#RRGGBB`).

## Add Events

Timed event:

```json
{
  "action": "create_event",
  "calendar_name": "Work",
  "title": "Standup",
  "start_at": "2026-04-16T09:00:00Z",
  "end_at": "2026-04-16T10:00:00Z"
}
```

All-day event:

```json
{
  "action": "create_event",
  "calendar_name": "Personal",
  "title": "Planning day",
  "start_date": "2026-04-17"
}
```

Recurring event:

```json
{
  "action": "create_event",
  "calendar_name": "Work",
  "title": "Daily standup",
  "start_at": "2026-04-16T09:00:00Z",
  "end_at": "2026-04-16T09:30:00Z",
  "recurrence": {"frequency":"daily","count":5}
}
```

## Add Tasks

Dated task:

```json
{
  "action": "create_task",
  "calendar_name": "Personal",
  "title": "Review notes",
  "due_date": "2026-04-16"
}
```

Timed task:

```json
{
  "action": "create_task",
  "calendar_name": "Work",
  "title": "Send summary",
  "due_at": "2026-04-16T11:00:00Z"
}
```

Recurring task:

```json
{
  "action": "create_task",
  "calendar_name": "Personal",
  "title": "Review notes",
  "due_date": "2026-04-16",
  "recurrence": {"frequency":"daily","count":5}
}
```

Recurrence fields are `frequency`, `interval`, `count`, `until_at`,
`until_date`, `by_weekday`, and `by_month_day`. Frequencies are `daily`,
`weekly`, and `monthly`. Weekdays use `MO`, `TU`, `WE`, `TH`, `FR`, `SA`, and
`SU`.

## Query Planning Data

Agenda window:

```json
{
  "action": "list_agenda",
  "from": "2026-04-16T00:00:00Z",
  "to": "2026-04-23T00:00:00Z",
  "limit": 200
}
```

List events:

```json
{"action":"list_events","calendar_name":"Work","limit":25}
```

List tasks:

```json
{"action":"list_tasks","calendar_name":"Personal","limit":25}
```

Use `calendar_id` only when the user or a previous runner result already
provided an ID.

## Complete Tasks

Non-recurring task:

```json
{"action":"complete_task","task_id":"01ARZ3NDEKTSV4RRFFQ69G5FAV"}
```

Recurring date-based task:

```json
{
  "action": "complete_task",
  "task_id": "01ARZ3NDEKTSV4RRFFQ69G5FAV",
  "occurrence_date": "2026-04-16"
}
```

Recurring timed task:

```json
{
  "action": "complete_task",
  "task_id": "01ARZ3NDEKTSV4RRFFQ69G5FAV",
  "occurrence_at": "2026-04-16T09:00:00Z"
}
```

## Validation

Reject without running code when a request has an ambiguous short date, a
year-first slash date, a non-positive limit, a missing required title, or an
unsupported recurrence value. The runner also validates requests before opening
the database and returns JSON rejections with `rejected: true`.
