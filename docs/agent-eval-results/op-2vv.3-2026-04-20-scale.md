# OpenPlanner Scale Eval 2026-04-20

- Issue: `op-2vv.3`
- Harness: go runner scale eval using runner.RunPlanningTask against an isolated SQLite database
- Threshold policy: local maintainer thresholds; failures create beads blockers but thresholds are not portable CI guarantees
- Run root: `<run-root>`
- Database path: `<run-root>/scale/openplanner.db`
- Harness wall seconds: `2.55`
- Scale score: `pass`
- Raw artifacts: Raw scale database and transient artifacts were retained under <run-root>/scale during execution and intentionally not committed.

## Dataset

| Calendars | Events | Tasks | Recurring Events | Recurring Tasks | Recurrence Rules | Completion Rows | Agenda Range Days | Limit |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 2 | 1200 | 1200 | 200 | 200 | 400 | 500 | 30 | 50 |

## Results

| Scenario | Passed | Wall Seconds | Threshold Seconds | Items Returned | Pages | Events | Tasks | Recurrence Rules | Completion Rows | Notes |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `large-agenda-window` | true | 0.02 | 5.00 | 50 | 1 | 1200 | 1200 | 400 | 500 |  |
| `recurring-event-expansion` | true | 0.13 | 3.00 | 300 | 6 | 1200 | 1200 | 400 | 500 | timed recurring event items observed: 7; all-day recurring event items observed: 67 |
| `recurring-task-completion-lookup` | true | 0.01 | 5.00 | 200 | 1 | 1200 | 1200 | 400 | 500 | completed agenda items observed: 116 |
| `list-pagination` | true | 0.18 | 3.00 | 2400 | 48 | 1200 | 1200 | 400 | 500 | events=1200/1200 tasks=1200/1200 |
