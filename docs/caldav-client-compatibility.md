# CalDAV Client Compatibility

`op-2vv.15` tested the experimental CalDAV adapter from `op-2vv.14`
against practical local clients. The production OpenPlanner surface remains the
JSON runner and portable skill payload; these results only evaluate whether
CalDAV is useful as an interoperability or migration aid.

## Run Metadata

- Initial run date: 2026-04-22
- Initial OpenPlanner commit: `6dee25f`
- Sync recheck date: 2026-04-22 after `op-jty`; use the commit containing
  this document as the code reference for the fixed behavior.
- Host OS: macOS 26.4.1, build 25E253
- Go: `go1.26.2 darwin/arm64`
- Server command:

```bash
mise exec -- go build -o <run-root>/bin/openplanner ./cmd/openplanner
<run-root>/bin/openplanner caldav --db <tmp-db> --addr 127.0.0.1:18080
```

The database was seeded through the production JSON runner with one `Compat`
calendar containing:

- one timed `VEVENT`: `Seed timed event`
- one all-day `VEVENT`: `Seed all-day event`
- one `VTODO`: `Seed task`

Raw client logs and temp databases were kept under `<run-root>` during the runs
and were not committed.

## Results

| Client | Version | Discovery | List/read | Create | Update | Delete | Tasks | Recurrence/time-range | Result |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `curl` | 8.7.1 | Passed: `OPTIONS` 204, `/.well-known/caldav` 301, `PROPFIND` root, principal, calendar-home, and calendar collection 207 | Passed: `GET` 200 and `HEAD` 200 for seeded object | Passed: `PUT` `VEVENT` 201, `PUT` `VTODO` 201 | Passed: repeat `PUT` returned 204 for event and task | Passed: `DELETE` 204, deleted object `GET` 404 | Passed | Passed: broad `REPORT` 207 included events/tasks; time-ranged `REPORT` 207 included the timed event and omitted the all-day event outside the range | Compatible with the prototype surface |
| `vdirsyncer` | 0.20.0 on Python 3.14.4 | Passed: discovered `/caldav/`, principal, calendar home, and `Compat` collection | Passed: initial sync downloaded events and tasks to a local vdir | Passed: synced a khal-created `VEVENT` and todoman-created `VTODO` to the adapter with `PUT` 201 | Passed after `op-jty`: client-side event and task updates sync back without conflict workarounds | Passed: todoman delete synced through vdirsyncer with `DELETE` 204 and the object disappeared from server reports | Passed for list/create/update/delete | Not covered beyond server-side `curl` time-range | Compatible with the prototype sync surface |
| `khal` | 0.14.0 on Python 3.14.4 | Via vdirsyncer | Passed: listed `Seed timed event` and `Seed all-day event` from the synced vdir | Passed: created `Khal client event`; vdirsyncer uploaded it and server `REPORT` included it | Not covered non-interactively | Not covered non-interactively | Not applicable | Not covered | Usable for event read/create through vdirsyncer |
| `todoman` | 4.7.0 on Python 3.14.4 | Via vdirsyncer | Passed: listed `Seed task` and `Client task updated` from the synced vdir | Passed with explicit `--priority medium`; vdirsyncer uploaded `Todoman client task` and server `REPORT` included it | Passed after `op-jty`: marking the task done synced to the server and exported `STATUS:COMPLETED` | Passed: deleting the task synced to the server with `DELETE` 204 | Passed for list/create/update/delete | Not applicable | Compatible with the prototype sync surface through vdirsyncer |
| Apple Calendar | 16.0 | Not run | Not run | Not run | Not run | Not run | Not applicable | Not run | Version captured, but the test did not create a machine-level Internet Accounts entry in the user's macOS profile |
| Thunderbird | Not installed | Not run | Not run | Not run | Not run | Not run | Not run | Not run | Not in local-practical matrix for this run |
| DAVx5 | Not available on this host | Not run | Not run | Not run | Not run | Not run | Not run | Not run | Not in local-practical matrix for this run |

## Failure Notes

- `op-jty` fixed the vdirsyncer sync gaps found in the first compatibility run:
  the adapter now handles `calendar-multiget` by requested `href`, and CalDAV
  object ETags remain stable across unchanged reads.
- After `op-jty`, vdirsyncer-backed todoman task completion syncs back to the
  server and subsequent exports include `STATUS:COMPLETED`.
- todoman's non-interactive creation required an explicit priority value in
  this environment. `todo new "Todoman client task"` failed with an invalid
  empty priority value, while `todo new --priority medium "Todoman client task"`
  succeeded. This appears to be client/config behavior, not a server failure.
- Apple Calendar was not connected during this automated run. Adding a CalDAV
  account requires mutating the user's macOS Internet Accounts state, and the
  prototype currently has no authentication or TLS story to validate there.

The remaining failure notes are client setup or product-scope limits rather than
known vdirsyncer sync blockers.

## Viability

CalDAV is not viable as the v1 migration path yet. The adapter is useful for
low-level interoperability research and for smoke-testing the service-backed
iCalendar import/export path, and `op-jty` makes vdirsyncer-backed sync clients
viable for the prototype surface. Practical GUI client support still needs a
deliberate auth/TLS/account setup story.

For v1 migration, keep the JSON runner and `import_icalendar` /
`export_icalendar` as the supported path. Treat CalDAV as experimental until the
sync-client follow-up work is complete and GUI clients such as Apple Calendar,
Thunderbird, and DAVx5 are verified in a controlled matrix.
