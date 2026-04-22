# CalDAV Prototype

`op-2vv.14` adds an experimental local CalDAV adapter for compatibility
research. The supported OpenPlanner product surface remains the installed
`openplanner planning` JSON runner and the portable `skills/openplanner`
payload; this adapter is not an agent-facing API contract.

## Running Locally

```bash
openplanner caldav --db <tmp-db> --addr 127.0.0.1:8080
```

The adapter uses the same SQLite-backed service as the JSON runner. `--db`
overrides `OPENPLANNER_DATABASE_PATH`; when both are absent, the default
OpenPlanner data path is used.

## Prototype Surface

- `/.well-known/caldav` redirects to `/caldav/`.
- `/caldav/` exposes basic DAV discovery.
- `/caldav/principals/local/` exposes the local principal.
- `/caldav/calendars/local/` lists local calendars.
- `/caldav/calendars/local/<calendar-id>/` exposes one calendar collection.
- `/caldav/calendars/local/<calendar-id>/<resource>.ics` exposes one calendar object.

Supported methods are minimal:

- `PROPFIND` with `Depth: 0` and `Depth: 1`.
- `REPORT` for CalDAV `calendar-query` and `calendar-multiget`.
- `GET` and `HEAD` for calendar object resources.
- `PUT` for one base `VEVENT` or `VTODO` per request.
- `DELETE` for existing calendar object resources.

## Smoke Results

Local scripted smoke coverage uses `curl` against the local adapter. Practical
client compatibility results are recorded in
[`docs/caldav-client-compatibility.md`](caldav-client-compatibility.md).
`op-2vv.15` found the prototype compatible with the baseline curl surface.
Follow-up `op-jty` added the `calendar-multiget` and stable object ETag behavior
needed by vdirsyncer-backed khal/todoman sync workflows. CalDAV remains
experimental and is not the v1 migration path until broader GUI client setup is
validated.

| Client | Scope | Result |
| --- | --- | --- |
| Go 1.26.2 | `httptest` coverage for discovery, `PROPFIND`, `calendar-query`, `calendar-multiget`, stable ETags, `GET`, `PUT`, and `DELETE` | Passed with `go test ./...` |
| curl 8.7.1 | Manual local HTTP smoke using the commands below | Passed: `PROPFIND` root `207`, `PROPFIND` home `207`, `PUT` `201`, `GET` `200`, `REPORT` `207`, `DELETE` `204`, deleted object `GET` `404` |

Example smoke sequence:

```bash
openplanner caldav --db <tmp-db> --addr 127.0.0.1:8080
```

```bash
curl -i -X PROPFIND http://127.0.0.1:8080/caldav/ \
  -H 'Depth: 0' \
  -H 'Content-Type: application/xml' \
  --data '<propfind xmlns="DAV:"><allprop/></propfind>'

curl -i -X PROPFIND http://127.0.0.1:8080/caldav/calendars/local/ \
  -H 'Depth: 1' \
  -H 'Content-Type: application/xml' \
  --data '<propfind xmlns="DAV:"><allprop/></propfind>'

curl -i -X REPORT http://127.0.0.1:8080/caldav/calendars/local/<calendar-id>/ \
  -H 'Content-Type: application/xml' \
  --data '<calendar-query xmlns="urn:ietf:params:xml:ns:caldav"/>'

curl -i -X PUT http://127.0.0.1:8080/caldav/calendars/local/<calendar-id>/client-event@example.com.ics \
  -H 'Content-Type: text/calendar' \
  --data-binary @<single-object-ics>

curl -i http://127.0.0.1:8080/caldav/calendars/local/<calendar-id>/client-event@example.com.ics

curl -i -X DELETE http://127.0.0.1:8080/caldav/calendars/local/<calendar-id>/client-event@example.com.ics
```

## Known Gaps

- No authentication, TLS, scheduling inbox/outbox, sharing, sync-token,
  `If-Match` enforcement, collection mutation, principal management, free-busy,
  or full client compatibility promise.
- No background daemon, hosted service, or public REST-style compatibility
  contract is introduced.
- URL/resource identity is best effort. Server-originated resources use
  OpenPlanner IDs. Client-originated resources can usually be resolved by their
  iCalendar UID if the UID is URL-safe.
- `PROPFIND` property support is intentionally narrow; unsupported requested
  properties are returned in a per-property `404` propstat.
- `REPORT calendar-query` supports broad object listing and `VEVENT`
  time-range filtering. `REPORT calendar-multiget` supports explicit object
  href fetches for sync clients. More advanced CalDAV reports and filters are
  not implemented.
- Large calendars are not optimized for sync clients; this prototype favors
  correctness and smokeability over sync performance.
