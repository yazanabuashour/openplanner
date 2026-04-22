# openplanner

OpenPlanner is a local-first planning runtime for agent-facing calendar and task
workflows. It ships an installed `openplanner` JSON runner, an Agent
Skills-compatible skill, and SQLite-backed local storage.

## Agent Install

Tell your agent:

```text
Install OpenPlanner from https://github.com/yazanabuashour/openplanner.
Complete both required steps before reporting success:
1. Install and verify the openplanner runner binary.
2. Register the OpenPlanner skill from skills/openplanner/SKILL.md using your native skill system.
```

The agent should install the `openplanner` runner and place the
`skills/openplanner` skill where that agent normally loads skills. OpenPlanner
does not require one canonical skill path: Codex, Claude Code, OpenClaw, Hermes,
and other Agent Skills-compatible agents each manage skill locations in their
own way.

## Manual Install

Until the first release tag is published, build the runner from a checkout:

```bash
go build -o ./bin/openplanner ./cmd/openplanner
```

Put that binary on `PATH`, then install or copy `skills/openplanner` using your
agent's skill installation workflow. The portable skill payload is the folder
containing `SKILL.md`; agent-specific directories are intentionally not part of
OpenPlanner's release contract.

After `v0.1.0`, release archives will include platform builds of the
`openplanner` runner, `install.sh`, and an `openplanner_<version>_skill.tar.gz`
archive for manual skill installation. The release installer installs and
verifies only the runner; skill registration remains delegated to the target
agent's native skill system.

```bash
curl -fsSL https://github.com/yazanabuashour/openplanner/releases/latest/download/install.sh | sh
```

Optional per-agent examples are in [docs/agent-install.md](docs/agent-install.md).
Those examples are not the OpenPlanner install contract.

## Product Surface

OpenPlanner's v1 product surface is the installed `openplanner planning` JSON runner
plus the portable `skills/openplanner` payload. Agents should integrate
through that runner and skill rather than importing repository packages.

The `internal/runner`, `internal/service`, `internal/store`, and domain packages are
implementation boundaries that support the runner. They are not public extension points
and do not carry SDK compatibility promises.

A public SDK, REST API, OpenAPI contract, hosted service, package registry, and
web UI are not v1 product deliverables unless a future roadmap issue explicitly
reapproves them.

## Runner Interface

The skill calls the installed runner:

```bash
printf '%s\n' '{"action":"list_agenda","from":"2026-04-16T00:00:00Z","to":"2026-04-17T00:00:00Z"}' \
  | openplanner planning
```

The runner reads structured JSON from stdin, validates and normalizes the
request, performs the local planning operation, and writes structured JSON to
stdout. By default, it stores SQLite data at
`${XDG_DATA_HOME:-~/.local/share}/openplanner/openplanner.db`. Override the path
with `OPENPLANNER_DATABASE_PATH` or `openplanner planning --db <path>`; `--db`
wins when both are present.

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
- `import_icalendar`
- `validate`

Update actions use patch semantics: omitted fields are preserved, non-null
fields are set, and `null` clears clearable optional fields. Use `event_id` for
`update_event`, `task_id` for `update_task`, and exactly one of `calendar_id` or
`calendar_name` for `update_calendar`.

Recurring events and tasks support occurrence-level cancellation and timing-only
reschedules without changing the base recurrence rule. Use `occurrence_at` for
timed occurrences and `occurrence_date` for all-day events or date-based tasks.
Agenda entries keep a stable `occurrence_key` based on the original occurrence;
use that key with `complete_task` when completing a moved recurring task.
Timed events may include `time_zone` with an IANA timezone name such as
`America/New_York`. The `start_at`, `end_at`, `occurrence_at`, and replacement
timed fields still must be strict RFC3339 values, and their numeric offsets must
match the named zone. Timezone-aware recurring events keep their local wall-clock
time across DST transitions. All-day events remain date-only and do not accept
`time_zone`.

```json
{"action":"cancel_event_occurrence","event_id":"<event-id>","occurrence_at":"2026-04-17T09:00:00Z"}
{"action":"reschedule_event_occurrence","event_id":"<event-id>","occurrence_at":"2026-04-18T09:00:00Z","start_at":"2026-04-19T11:00:00Z"}
{"action":"create_event","calendar_name":"Work","title":"Weekly sync","start_at":"2026-03-03T09:00:00-05:00","time_zone":"America/New_York","recurrence":{"frequency":"weekly","count":2}}
{"action":"cancel_task_occurrence","task_id":"<task-id>","occurrence_date":"2026-04-17"}
{"action":"reschedule_task_occurrence","task_id":"<task-id>","occurrence_date":"2026-04-18","due_date":"2026-04-19"}
{"action":"complete_task","task_id":"<task-id>","occurrence_key":"<key-from-list-agenda-result>"}
```

Events support optional attendee metadata through `attendees`. Each attendee
requires `email`; optional fields are `display_name`, `role`, `participation_status`,
and `rsvp`. `role` defaults to `required` and accepts `required`, `optional`,
`chair`, or `non_participant`. `participation_status` defaults to
`needs_action` and accepts `needs_action`, `accepted`, `declined`, `tentative`,
or `delegated`. Duplicate attendee emails on one event are rejected
case-insensitively.

The event `time_zone` field maps to iCalendar `TZID` during export and import.
The runner supports iCalendar export with `export_icalendar` and import with
`import_icalendar`.

```json
{"action":"export_icalendar"}
{"action":"export_icalendar","calendar_name":"Work"}
{"action":"import_icalendar","content":"BEGIN:VCALENDAR\r\nVERSION:2.0\r\n..."}
{"action":"import_icalendar","calendar_name":"Work","content":"BEGIN:VCALENDAR\r\nVERSION:2.0\r\n..."}
```

The export action returns an `icalendar` object with `content_type`, `filename`,
optional calendar metadata, `event_count`, `task_count`, and `content`. The
`content` value is complete `.ics` text; the runner does not write files
directly.

The import action accepts complete `.ics` text in `content`; it does not read
files directly. It imports supported `VEVENT` and `VTODO` fields, recurrence,
occurrence exceptions, reminders, attendees, task status, priority, and tags.
Repeat imports update existing rows by iCalendar UID within the target
calendar. The result includes `icalendar_import` counts for calendars, events,
tasks, created rows, updated rows, skipped rows, and component skip reasons.

```json
{"action":"create_event","calendar_name":"Work","title":"Planning","start_at":"2026-04-16T09:00:00Z","attendees":[{"email":"alex@example.com","display_name":"Alex Rivera","role":"required","participation_status":"accepted","rsvp":true}]}
{"action":"update_event","event_id":"<id-from-prior-runner-result>","attendees":null}
```

Tasks support optional metadata. `priority` is one of `low`, `medium`, or
`high` and defaults to `medium`. `status` is one of `todo`, `in_progress`, or
`done` and defaults to `todo`. `tags` is an array of lowercase labels using
letters, digits, `_`, or `-`; `list_tasks` matches all supplied tags when
filtering.

Events and tasks can be linked explicitly for prep, follow-up, and agenda-based
completion workflows. Use `create_event_task_link` with `event_id` and
`task_id`, `list_event_task_links` with optional `event_id` and/or `task_id`,
and `delete_event_task_link` with both IDs. Event responses include
`linked_task_ids`; task responses include `linked_event_ids`; agenda event and
task items include the corresponding linked IDs.

```json
{"action":"create_event_task_link","event_id":"<event-id>","task_id":"<task-id>"}
{"action":"list_event_task_links","event_id":"<event-id>"}
{"action":"delete_event_task_link","event_id":"<event-id>","task_id":"<task-id>"}
```

Events and tasks support optional reminder rules through `reminders`, an array
of objects with positive `before_minutes` values. Reminder offsets are measured
before the event start or task due time. Use `list_pending_reminders` with
`from` and `to` RFC3339 bounds to query pending reminder occurrences, including
expanded recurring occurrences, and use `dismiss_reminder` with the returned
`reminder_occurrence_id` to dismiss an occurrence idempotently.

```json
{"action":"create_task","calendar_name":"Personal","title":"Take medicine","due_at":"2026-04-16T10:00:00Z","reminders":[{"before_minutes":60}]}
{"action":"list_pending_reminders","from":"2026-04-16T08:00:00Z","to":"2026-04-16T10:00:00Z","limit":10}
{"action":"dismiss_reminder","reminder_occurrence_id":"<id-from-list-pending-reminders-result>"}
```

Delete actions use `event_id` for `delete_event`, `task_id` for `delete_task`,
and exactly one of `calendar_id` or `calendar_name` for `delete_calendar`.
Calendar deletion is empty-calendar-only and never cascades to contained events
or tasks.

## Development

Install the pinned toolchain with:

```bash
mise install
```

Exercise the JSON runner with:

```bash
go build -o ./bin/openplanner ./cmd/openplanner
printf '%s\n' '{"action":"validate"}' | ./bin/openplanner planning
```

Run the local quality gates with:

```bash
make check
```

`make check` runs formatting validation, Agent Skills validation, the Go test
suite, `govulncheck`, and `golangci-lint`.

## Release Contract

The `0.1.0` release deliverables are:

- platform archives for the `openplanner` runner binary
- the single-file `openplanner` skill archive
- a canonical source archive, SHA256 checksums, an SPDX SBOM, and GitHub
  attestations for release verification

Until `v0.1.0` exists, local development should build the runner from a checkout
or install from `main`.

## Repository Contents

- [CONTRIBUTING.md](CONTRIBUTING.md) explains how outside contributors should propose changes.
- [SECURITY.md](SECURITY.md) explains how to report vulnerabilities privately and what response timing to expect.
- [docs/maintainers.md](docs/maintainers.md) documents Beads-based maintainer workflow and repo administration notes.
- [docs/release-verification.md](docs/release-verification.md) explains the published release assets and how to verify them.
- [docs/agent-evals.md](docs/agent-evals.md) explains how to evaluate production agent workflows.
- [internal/runner](internal/runner) contains the JSON-friendly task facade for production agent workflows.
- [cmd/openplanner](cmd/openplanner) contains the installed JSON runner.
- [skills/openplanner/SKILL.md](skills/openplanner/SKILL.md) is the portable Agent Skills-compatible OpenPlanner skill.
- [LICENSE](LICENSE) defines the project license.

## Contributing

Outside contributors can work entirely through GitHub issues and pull requests.
Beads is maintainer-only workflow tooling and is not required for community
contributions.

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution expectations and
[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for community standards.
