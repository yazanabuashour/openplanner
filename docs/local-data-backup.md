# Local Data Backup And Recovery

OpenPlanner stores local-first planning data in one SQLite database file. Back
up and restore that file when you need full local recovery of calendars, events,
tasks, reminders, links, and imported iCalendar data.

See [`docs/local-data-security.md`](local-data-security.md) for the local data
threat model, sensitive artifact handling, and parser/server hardening
follow-ups.

## Database Path

By default, the installed runner stores data at:

```text
${XDG_DATA_HOME:-~/.local/share}/openplanner/openplanner.db
```

You can override the database path in two ways:

- `openplanner planning --db <database-path>` uses the explicit path for that
  runner call.
- `OPENPLANNER_DATABASE_PATH=<database-path>` sets the path for runner calls in
  that environment.

When both are present, `openplanner planning --db <database-path>` wins over
`OPENPLANNER_DATABASE_PATH`. When neither is present, the default path is used.
`OPENPLANNER_DATABASE_PATH` is the only supported environment-variable runtime
override. Other runtime configuration is stored in the local SQLite database.

The experimental `openplanner caldav` adapter uses the same database path rules:
`--db <database-path>` wins over `OPENPLANNER_DATABASE_PATH`, and the default
path is used when neither override is present.

## File Permissions

On POSIX-style filesystems, OpenPlanner creates or corrects its local data
directory to `0700` and its SQLite database file to `0600`. SQLite sidecar files
created next to the database, including `-journal`, `-wal`, and `-shm`, are also
corrected to `0600` when OpenPlanner sees them.

Platforms and filesystems without meaningful owner-only mode support may treat
these modes as best-effort. If your planning data is sensitive, keep the
database and backups in a user-private location covered by your normal encrypted
backup process.

## Back Up Data

1. Stop active OpenPlanner runner usage and stop any local CalDAV adapter using
   the same database.
2. Pick the database path to back up. Use the default path above unless you run
   OpenPlanner with `OPENPLANNER_DATABASE_PATH` or `--db <database-path>`.
3. Copy the database file to a timestamped backup path:

```bash
mkdir -p <backup-dir>
backup="<backup-dir>/openplanner-$(date -u +%Y%m%dT%H%M%SZ).db"
cp -p <database-path> "$backup"
```

`cp -p` preserves the source file mode and timestamps on common POSIX systems.
Keep `<backup-dir>` private to the local user, and store the backup in a
location managed by your normal encrypted backup process when the planning data
is sensitive.

## Restore Data

1. Stop active OpenPlanner runner usage and stop any local CalDAV adapter using
   the same database.
2. Move the current database aside before replacing it:

```bash
mv <database-path> <database-path>.before-restore
cp -p <backup-dir>/openplanner-<timestamp>.db <database-path>
```

3. Verify the restored file through the installed runner before resuming normal
   use.

If the restore does not look correct, stop active use again, move the restored
file aside, and move `<database-path>.before-restore` back to
`<database-path>`.

## Verify Recovery

First confirm that the runner can read the selected database path:

```bash
printf '%s\n' '{"action":"validate"}' | openplanner planning --db <restored-db>
```

The result should be JSON with `rejected` set to `false` and `summary` set to
`valid`.

Then confirm expected planning data with read-only runner actions. For example:

```bash
printf '%s\n' '{"action":"list_events","limit":5}' | openplanner planning --db <restored-db>
printf '%s\n' '{"action":"list_tasks","limit":5}' | openplanner planning --db <restored-db>
printf '%s\n' '{"action":"list_agenda","from":"2026-04-22T00:00:00Z","to":"2026-04-23T00:00:00Z","limit":10}' | openplanner planning --db <restored-db>
```

Use a date range that should contain known local data when checking
`list_agenda`.

## Safety Notes

Do not edit local OpenPlanner data through SQLite directly. Use the installed
`openplanner planning` JSON runner for normal writes and for recovery
verification.

Use `export_icalendar` and `import_icalendar` when you need calendar
interchange. Use database-file backup and restore when you need complete local
OpenPlanner recovery.

Do not upload local databases, backups, real iCalendar exports, or raw import
and CalDAV logs to public issues, pull requests, eval artifacts, or release
assets.
