# Local Data Security Review

`op-2vv.18` reviews the security model for local OpenPlanner calendar and task
data. OpenPlanner is local-first and pre-`1.0`; this review documents the
current boundary, the data that should be treated as sensitive, the main threat
scenarios, and the follow-up hardening work that should block broader exposure.

## Security Boundary

OpenPlanner's supported product surface remains the installed
`openplanner planning` JSON runner plus the portable `skills/openplanner`
payload. Internal Go packages, the SQLite schema, and the experimental CalDAV
adapter support that surface but are not public compatibility contracts.

The JSON runner reads one structured JSON request from stdin, validates and
normalizes it, and reads or writes the selected SQLite database. By default the
database is stored at the XDG data location documented in
[`docs/local-data-backup.md`](local-data-backup.md). Callers may override that
path with `openplanner planning --db <database-path>` or
`OPENPLANNER_DATABASE_PATH=<database-path>`; `--db` wins when both are present.
Those path inputs are trusted caller-controlled filesystem inputs. OpenPlanner
does not sandbox database paths, prevent access to caller-selected locations, or
provide multi-user access control around local files.

The `import_icalendar` runner action accepts complete `.ics` text in the JSON
`content` field. It does not read `.ics` files directly. The `export_icalendar`
action returns complete `.ics` text in JSON and does not write export files.
Agents and users choose where to store exported content.

The experimental `openplanner caldav` adapter uses the same local SQLite-backed
service and the same database path precedence rules. It is unauthenticated,
does not provide TLS, and exists for local compatibility research. It is
loopback-only: bind addresses must be `localhost:<port>` or loopback IP
literals such as `127.0.0.1:<port>` or `[::1]:<port>`. Non-loopback, wildcard,
and empty-host bind addresses are rejected.

## Sensitive Data

Treat the following as sensitive local planning data:

- SQLite database files and any sidecar files created by SQLite.
- Backup copies of the database.
- iCalendar export files or copied export content.
- Imported `.ics` content and provider migration fixtures before sanitization.
- Calendar names, event and task titles, descriptions, locations, reminders,
  attendees, task metadata, recurrence exceptions, links, and completion state.
- Raw logs from manual import, export, CalDAV, or agent-eval runs when they
  include user planning content.

Committed docs, reports, and artifacts must use repo-relative paths or neutral
placeholders such as `<database-path>`, `<backup-dir>`, and `<run-root>`.
Do not commit personal calendar exports, private email addresses, real attendee
data, or machine-absolute filesystem paths.

## Threat Model

### Local Data Disclosure

The main confidentiality risk is accidental disclosure of local planning data
through database files, backups, exports, temp run directories, logs, or copied
`.ics` payloads. The runner and CalDAV adapter create the data directory with
private permissions where supported. OpenPlanner also pre-creates and corrects
the selected SQLite database file with owner-only permissions on POSIX-style
filesystems, and corrects SQLite sidecar files such as `-journal`, `-wal`, and
`-shm` when they exist.

Current mitigations:

- Keep the database in the user's local data directory by default.
- Use `0700` for OpenPlanner-created data directories and `0600` for local
  SQLite database and sidecar files on POSIX-style filesystems. Platforms and
  filesystems without meaningful owner-only mode support may treat those modes
  as best-effort.
- Document backup and restore through database-file copies in
  [`docs/local-data-backup.md`](local-data-backup.md).
- Use neutral artifact placeholders in committed docs and eval reports.
- Keep provider import fixtures synthetic and covered by tests that reject
  machine-absolute paths and non-example email markers.

### Integrity Loss

The main integrity risks are destructive or confusing local writes through:

- malformed or hostile `.ics` imports
- repeat imports that update rows by iCalendar UID within a calendar
- CalDAV `PUT` and `DELETE`
- broad delete actions from the JSON runner
- caller-selected database paths that point at the wrong local file

Current mitigations:

- Runner actions validate required IDs, calendar references, dates, recurrence,
  attendees, reminders, tags, task metadata, and pagination limits before
  opening the local database where practical.
- The JSON runner rejects stdin requests larger than 4 MiB before opening the
  database.
- Calendar deletion is empty-calendar-only and does not cascade to contained
  events or tasks.
- iCalendar import skips unsupported components instead of importing ambiguous
  partial data where validation fails.
- iCalendar imports reject `content` larger than 2 MiB or more than 2,000 total
  `VEVENT`/`VTODO` base and override components before writing imported data.
- CalDAV `PUT` only accepts one base `VEVENT` or `VTODO` per request.
- CalDAV `PUT` rejects request bodies larger than 2 MiB instead of truncating
  oversized input.
- CalDAV resource resolution is scoped to the requested calendar.

Remaining hardening:

- Add parser-focused fuzz and regression coverage in `op-5gj`.
- Keep CalDAV experimental and loopback-only.

### Parser And Server Denial Of Service

The highest current denial-of-service risk is local parsing of unusual JSON,
`.ics`, or XML payloads. OpenPlanner applies explicit local input ceilings to
the parser and server entrypoints:

- `openplanner planning` reads one JSON stdin request up to 4 MiB.
- `import_icalendar` accepts iCalendar `content` up to 2 MiB and 2,000 total
  `VEVENT`/`VTODO` base and override components.
- CalDAV `PROPFIND` and `REPORT` XML request bodies are limited to 2 MiB and an
  XML depth of 64.
- CalDAV `PUT` request bodies are limited to 2 MiB.

Parser hardening tests for malformed, oversized, nested, and unusual inputs are
tracked in `op-5gj`.

### Cross-Calendar Leakage

CalDAV listing and object reads expose local calendar data to any client that
can reach the adapter. The adapter rejects non-loopback bind addresses so this
surface remains local-only. Within the adapter, object resolution should remain
scoped to the target calendar, and `calendar-multiget` requests for missing or
cross-calendar hrefs should return not-found properties instead of content.
Existing tests cover those baseline behaviors. Any future remote calendar
service would need a separate product decision and design for access control,
authentication, and transport.

### Maintainer And Supply-Chain Security

Security reports are handled through [`SECURITY.md`](../SECURITY.md). Pull
requests run fork-safe checks, dependency review, tests, `govulncheck`, and
`golangci-lint` through the workflow described in
[`docs/maintainers.md`](maintainers.md). Broader recurring security operations
remain tracked by `op-4wm`, and maintainer isolation remains tracked by
`op-7tb`.

## Testing Plan

Routine changes that touch local data handling, iCalendar import/export,
CalDAV, runner validation, or SQLite storage should run:

```bash
mise exec -- make check
```

At minimum, security-sensitive changes in this area should run:

```bash
mise exec -- go test ./...
mise exec -- golangci-lint run ./...
```

Use targeted tests for:

- JSON runner rejection before database creation for invalid requests.
- iCalendar import malformed content, unsupported components, recurrence
  exceptions, reminders, attendees, task metadata, provider fixtures, and repeat
  imports.
- CalDAV discovery, `PROPFIND`, `REPORT`, `calendar-multiget`, `GET`, `HEAD`,
  `PUT`, `DELETE`, ETags, cross-calendar hrefs, invalid content types, and
  malformed calendar objects.
- SQLite migration and local data preservation across schema changes.
- Documentation policy checks that reject machine-absolute paths in committed
  docs and reports.

Security-specific follow-ups:

- `op-5gj`: add parser hardening fuzz and regression coverage.
- `op-d7k`: enforce loopback-only CalDAV local compatibility mode.

## Operational Guidance

- Prefer the JSON runner for normal local planning work.
- Keep CalDAV disabled unless actively testing local client compatibility.
- Bind CalDAV to loopback only; non-loopback bind addresses are rejected.
- Stop active runner and CalDAV usage before backing up or restoring the
  database.
- Store database backups and iCalendar exports only in locations covered by the
  user's normal encrypted backup process when the planning data is sensitive.
- Do not upload local databases, backups, raw CalDAV logs, or real `.ics`
  exports to public issues, pull requests, eval artifacts, or release assets.
