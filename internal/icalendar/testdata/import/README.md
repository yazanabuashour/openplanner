# Provider Import Fixtures

These iCalendar fixtures are synthesized migration samples for OpenPlanner
tests. They are not real user exports. Names, email addresses, locations, UIDs,
and dates are deterministic fake data, and all addresses use `example.com`.

The fixtures model provider-shaped export patterns while still exercising only
OpenPlanner's generic iCalendar importer:

- `google.ics` represents a Google Calendar timed recurring event with `TZID`,
  `EXDATE`, `RECURRENCE-ID`, attendees, reminder, description, location,
  `X-WR-CALNAME`, and calendar `COLOR`.
- `apple.ics` represents Apple Calendar all-day and multi-day events with
  exclusive `DTEND`, date recurrence, date exceptions, occurrence rescheduling,
  Apple `X-*` metadata, `X-WR-CALNAME`, and calendar `COLOR`.
- `microsoft.ics` represents Microsoft Outlook event and task exports with
  UTC timed fields, task priority/status/categories, reminders, Microsoft `X-*`
  metadata, `X-WR-CALNAME`, and calendar `COLOR`.

Unsupported provider extension fields are included only to prove they are
tolerated by parsing. They are not migration fidelity targets. Fidelity here
means supported OpenPlanner fields survive import and repeat imports update by
iCalendar UID without duplicating rows.
