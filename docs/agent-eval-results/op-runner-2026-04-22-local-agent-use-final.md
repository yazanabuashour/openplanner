# OpenPlanner JSON Runner Eval 2026-04-22-local-agent-use-final

- Model: `gpt-5.4-mini`
- Reasoning effort: `medium`
- Parallelism: `4`
- Cache mode: `shared`
- Cache prewarm seconds: `17.01`
- Harness elapsed seconds: `489.15`
- Effective parallel speedup: `1.63x`
- Parallel efficiency: `0.41`
- Production score: `pass`
- Comparison status: not applicable: OpenPlanner has no human CLI baseline variant; this report scores the production JSON runner surface only
- Raw logs committed: `false`
- Raw logs note: Raw codex exec event logs and stderr files were retained under <run-root> during execution and intentionally not committed.

## Production Gates

| Criterion | Passed | Details |
| --- | ---: | --- |
| `production_passes_all_scenarios` | true | 39/39 scenarios passed |
| `invalid_inputs_are_final_answer_only` | true | invalid-input scenarios used no tools, no command executions, and at most one assistant answer |
| `no_forbidden_inspection_or_cli_usage` | true | no removed-interface path inspection, module-cache inspection, direct SQLite access, CLI usage, or routine broad repo search detected |
| `aggregate_non_cached_tokens_reported` | true | 39/39 scenarios exposed usage; aggregate non-cached input tokens: 275945 |
| `expanded_category_coverage` | true | expanded production categories covered |

## Scenario Coverage

| Category | Required | Passed | Feature State | Scenarios | Details |
| --- | ---: | ---: | --- | --- | --- |
| `advanced_recurrence` | true | true | `supported` | monthly-recurrence-by-month-day, weekly-recurrence-by-weekday | category present |
| `migration` | true | true | `supported` | import-icalendar, migration-style-copy | category present |
| `multi_turn_disambiguation` | true | true | `supported` | mt-clarify-then-create, mt-disambiguate-calendar, mt-list-then-complete | category present |
| `routine` | true | true | `supported` | agenda-range, complete-recurring-task, complete-task, create-all-day-event, create-dated-task, create-recurring-event, create-recurring-task, create-timed-event, create-timed-task, delete-empty-calendar, delete-event, delete-task, ensure-calendar, list-events-filter-limit, list-tasks-filter-limit, list-tasks-metadata-filter, mixed-event-task, reminder-create-query-dismiss | category present |
| `update` | true | true | `supported` | task-metadata-create, update-calendar-metadata, update-event-patch-clear, update-task-due-mode | category present |
| `validation` | false | true | `supported` | ambiguous-short-date, invalid-range, invalid-rfc3339, invalid-task-priority, invalid-task-status, invalid-task-tag, missing-title, non-positive-limit, unsupported-recurrence, year-first-slash-date | category present |

## Results

| Scenario | Category | Feature State | Passed | Tools | Commands | Assistant Calls | Non-Cached Tokens | Wall Seconds | Details |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `ensure-calendar` | `routine` | `supported` | true | 2 | 2 | 3 | 7213 | 14.27 | turn 1: expected calendar in DB and final answer |
| `create-timed-event` | `routine` | `supported` | true | 4 | 4 | 5 | 8094 | 29.62 | turn 1: expected events in DB and final answer |
| `create-all-day-event` | `routine` | `supported` | true | 3 | 3 | 4 | 7091 | 33.98 | turn 1: expected events in DB and final answer |
| `create-dated-task` | `routine` | `supported` | true | 3 | 3 | 4 | 3728 | 70.84 | turn 1: expected tasks in DB and final answer |
| `create-timed-task` | `routine` | `supported` | true | 3 | 3 | 4 | 7608 | 20.56 | turn 1: expected tasks in DB and final answer |
| `create-recurring-event` | `routine` | `supported` | true | 2 | 2 | 3 | 7383 | 21.13 | turn 1: expected events in DB and final answer |
| `create-recurring-task` | `routine` | `supported` | true | 2 | 2 | 3 | 7085 | 14.18 | turn 1: expected tasks in DB and final answer |
| `agenda-range` | `routine` | `supported` | true | 3 | 3 | 4 | 7738 | 16.49 | turn 1: expected bounded agenda chronologically |
| `list-events-filter-limit` | `routine` | `supported` | true | 4 | 4 | 5 | 11303 | 21.00 | turn 1: expected events in DB and final answer |
| `list-tasks-filter-limit` | `routine` | `supported` | true | 4 | 4 | 5 | 5956 | 19.27 | turn 1: expected tasks in DB and final answer |
| `list-tasks-metadata-filter` | `routine` | `supported` | true | 3 | 3 | 4 | 7053 | 17.31 | turn 1: expected tasks in DB and final answer |
| `complete-task` | `routine` | `supported` | true | 4 | 4 | 5 | 7647 | 21.88 | turn 1: expected tasks in DB and final answer |
| `complete-recurring-task` | `routine` | `supported` | true | 3 | 3 | 4 | 7787 | 23.19 | turn 1: expected recurring task occurrence completed |
| `delete-task` | `routine` | `supported` | true | 4 | 4 | 4 | 8043 | 18.52 | turn 1: expected target task deleted while keep task remains |
| `delete-event` | `routine` | `supported` | true | 5 | 5 | 5 | 8024 | 22.06 | turn 1: expected target event deleted while keep event remains |
| `delete-empty-calendar` | `routine` | `supported` | true | 5 | 5 | 4 | 8745 | 23.27 | turn 1: expected empty calendar deleted before verification recreated it |
| `mixed-event-task` | `routine` | `supported` | true | 3 | 3 | 3 | 8155 | 19.18 | turn 1: expected tasks in DB and final answer |
| `ambiguous-short-date` | `validation` | `supported` | true | 0 | 0 | 1 | 2809 | 7.12 | turn 1: expected direct rejection or clarification without DB writes |
| `year-first-slash-date` | `validation` | `supported` | true | 0 | 0 | 1 | 2827 | 6.50 | turn 1: expected direct rejection or clarification without DB writes |
| `invalid-rfc3339` | `validation` | `supported` | true | 0 | 0 | 1 | 2850 | 6.76 | turn 1: expected direct rejection or clarification without DB writes |
| `missing-title` | `validation` | `supported` | true | 0 | 0 | 1 | 2798 | 9.24 | turn 1: expected direct rejection or clarification without DB writes |
| `invalid-range` | `validation` | `supported` | true | 0 | 0 | 1 | 2808 | 4.98 | turn 1: expected direct rejection or clarification without DB writes |
| `unsupported-recurrence` | `validation` | `supported` | true | 0 | 0 | 1 | 2803 | 4.64 | turn 1: expected direct rejection or clarification without DB writes |
| `non-positive-limit` | `validation` | `supported` | true | 0 | 0 | 1 | 2779 | 8.31 | turn 1: expected direct rejection or clarification without DB writes |
| `invalid-task-priority` | `validation` | `supported` | true | 0 | 0 | 1 | 2820 | 9.78 | turn 1: expected direct rejection or clarification without DB writes |
| `invalid-task-status` | `validation` | `supported` | true | 0 | 0 | 1 | 2815 | 7.58 | turn 1: expected direct rejection or clarification without DB writes |
| `invalid-task-tag` | `validation` | `supported` | true | 0 | 0 | 1 | 2817 | 5.54 | turn 1: expected direct rejection or clarification without DB writes |
| `update-calendar-metadata` | `update` | `supported` | true | 5 | 5 | 5 | 15665 | 28.53 | turn 1: expected calendar metadata in DB and final answer |
| `update-event-patch-clear` | `update` | `supported` | true | 3 | 3 | 4 | 7623 | 22.66 | turn 1: expected events in DB and final answer |
| `update-task-due-mode` | `update` | `supported` | true | 3 | 3 | 4 | 7857 | 14.07 | turn 1: expected tasks in DB and final answer |
| `weekly-recurrence-by-weekday` | `advanced_recurrence` | `supported` | true | 2 | 2 | 3 | 6714 | 12.09 | turn 1: expected recurrence occurrences in agenda |
| `monthly-recurrence-by-month-day` | `advanced_recurrence` | `supported` | true | 2 | 2 | 3 | 7433 | 18.21 | turn 1: expected recurrence occurrences in agenda |
| `task-metadata-create` | `update` | `supported` | true | 4 | 4 | 4 | 8013 | 22.71 | turn 1: expected tasks in DB and final answer |
| `migration-style-copy` | `migration` | `supported` | true | 9 | 9 | 6 | 9154 | 47.78 | turn 1: expected copied Work items while Legacy items remain |
| `import-icalendar` | `migration` | `supported` | true | 7 | 7 | 4 | 10114 | 43.20 | turn 1: expected imported calendar and UID-backed event in DB and final answer |
| `reminder-create-query-dismiss` | `routine` | `supported` | true | 8 | 8 | 5 | 9396 | 31.49 | turn 1: expected reminder stored, queried, dismissed, and absent from pending range |
| `mt-clarify-then-create` | `multi_turn_disambiguation` | `supported` | true | 2 | 2 | 4 | 9981 | 16.24 | turn 1: expected direct rejection or clarification without DB writes; turn 2: expected tasks in DB and final answer |
| `mt-list-then-complete` | `multi_turn_disambiguation` | `supported` | true | 3 | 3 | 5 | 15225 | 23.12 | turn 1: expected tasks in DB and final answer; turn 2: expected tasks in DB and final answer |
| `mt-disambiguate-calendar` | `multi_turn_disambiguation` | `supported` | true | 5 | 5 | 6 | 11991 | 40.82 | turn 1: expected clarification before creating destination copy; turn 2: expected copied Work task while Legacy task remains |

## Phase Timings

| Phase | Seconds |
| --- | ---: |
| prepare_run_dir | 0.00 |
| copy_repo | 2.14 |
| install_variant | 942.12 |
| warm_cache | 0.00 |
| seed_db | 0.13 |
| agent_run | 798.12 |
| parse_metrics | 0.00 |
| verify | 0.02 |
| total | 1817.87 |

## Turn Details

- `production/ensure-calendar` turn 1: exit `0`, tools `2`, assistant calls `3`, wall `14.27`, raw `<run-root>/production/ensure-calendar/turn-1/events.jsonl`.
- `production/create-timed-event` turn 1: exit `0`, tools `4`, assistant calls `5`, wall `29.62`, raw `<run-root>/production/create-timed-event/turn-1/events.jsonl`.
- `production/create-all-day-event` turn 1: exit `0`, tools `3`, assistant calls `4`, wall `33.98`, raw `<run-root>/production/create-all-day-event/turn-1/events.jsonl`.
- `production/create-dated-task` turn 1: exit `0`, tools `3`, assistant calls `4`, wall `70.84`, raw `<run-root>/production/create-dated-task/turn-1/events.jsonl`.
- `production/create-timed-task` turn 1: exit `0`, tools `3`, assistant calls `4`, wall `20.56`, raw `<run-root>/production/create-timed-task/turn-1/events.jsonl`.
- `production/create-recurring-event` turn 1: exit `0`, tools `2`, assistant calls `3`, wall `21.13`, raw `<run-root>/production/create-recurring-event/turn-1/events.jsonl`.
- `production/create-recurring-task` turn 1: exit `0`, tools `2`, assistant calls `3`, wall `14.18`, raw `<run-root>/production/create-recurring-task/turn-1/events.jsonl`.
- `production/agenda-range` turn 1: exit `0`, tools `3`, assistant calls `4`, wall `16.49`, raw `<run-root>/production/agenda-range/turn-1/events.jsonl`.
- `production/list-events-filter-limit` turn 1: exit `0`, tools `4`, assistant calls `5`, wall `21.00`, raw `<run-root>/production/list-events-filter-limit/turn-1/events.jsonl`.
- `production/list-tasks-filter-limit` turn 1: exit `0`, tools `4`, assistant calls `5`, wall `19.27`, raw `<run-root>/production/list-tasks-filter-limit/turn-1/events.jsonl`.
- `production/list-tasks-metadata-filter` turn 1: exit `0`, tools `3`, assistant calls `4`, wall `17.31`, raw `<run-root>/production/list-tasks-metadata-filter/turn-1/events.jsonl`.
- `production/complete-task` turn 1: exit `0`, tools `4`, assistant calls `5`, wall `21.88`, raw `<run-root>/production/complete-task/turn-1/events.jsonl`.
- `production/complete-recurring-task` turn 1: exit `0`, tools `3`, assistant calls `4`, wall `23.19`, raw `<run-root>/production/complete-recurring-task/turn-1/events.jsonl`.
- `production/delete-task` turn 1: exit `0`, tools `4`, assistant calls `4`, wall `18.52`, raw `<run-root>/production/delete-task/turn-1/events.jsonl`.
- `production/delete-event` turn 1: exit `0`, tools `5`, assistant calls `5`, wall `22.06`, raw `<run-root>/production/delete-event/turn-1/events.jsonl`.
- `production/delete-empty-calendar` turn 1: exit `0`, tools `5`, assistant calls `4`, wall `23.27`, raw `<run-root>/production/delete-empty-calendar/turn-1/events.jsonl`.
- `production/mixed-event-task` turn 1: exit `0`, tools `3`, assistant calls `3`, wall `19.18`, raw `<run-root>/production/mixed-event-task/turn-1/events.jsonl`.
- `production/ambiguous-short-date` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `7.12`, raw `<run-root>/production/ambiguous-short-date/turn-1/events.jsonl`.
- `production/year-first-slash-date` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `6.50`, raw `<run-root>/production/year-first-slash-date/turn-1/events.jsonl`.
- `production/invalid-rfc3339` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `6.76`, raw `<run-root>/production/invalid-rfc3339/turn-1/events.jsonl`.
- `production/missing-title` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `9.24`, raw `<run-root>/production/missing-title/turn-1/events.jsonl`.
- `production/invalid-range` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `4.98`, raw `<run-root>/production/invalid-range/turn-1/events.jsonl`.
- `production/unsupported-recurrence` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `4.64`, raw `<run-root>/production/unsupported-recurrence/turn-1/events.jsonl`.
- `production/non-positive-limit` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `8.31`, raw `<run-root>/production/non-positive-limit/turn-1/events.jsonl`.
- `production/invalid-task-priority` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `9.78`, raw `<run-root>/production/invalid-task-priority/turn-1/events.jsonl`.
- `production/invalid-task-status` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `7.58`, raw `<run-root>/production/invalid-task-status/turn-1/events.jsonl`.
- `production/invalid-task-tag` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `5.54`, raw `<run-root>/production/invalid-task-tag/turn-1/events.jsonl`.
- `production/update-calendar-metadata` turn 1: exit `0`, tools `5`, assistant calls `5`, wall `28.53`, raw `<run-root>/production/update-calendar-metadata/turn-1/events.jsonl`.
- `production/update-event-patch-clear` turn 1: exit `0`, tools `3`, assistant calls `4`, wall `22.66`, raw `<run-root>/production/update-event-patch-clear/turn-1/events.jsonl`.
- `production/update-task-due-mode` turn 1: exit `0`, tools `3`, assistant calls `4`, wall `14.07`, raw `<run-root>/production/update-task-due-mode/turn-1/events.jsonl`.
- `production/weekly-recurrence-by-weekday` turn 1: exit `0`, tools `2`, assistant calls `3`, wall `12.09`, raw `<run-root>/production/weekly-recurrence-by-weekday/turn-1/events.jsonl`.
- `production/monthly-recurrence-by-month-day` turn 1: exit `0`, tools `2`, assistant calls `3`, wall `18.21`, raw `<run-root>/production/monthly-recurrence-by-month-day/turn-1/events.jsonl`.
- `production/task-metadata-create` turn 1: exit `0`, tools `4`, assistant calls `4`, wall `22.71`, raw `<run-root>/production/task-metadata-create/turn-1/events.jsonl`.
- `production/migration-style-copy` turn 1: exit `0`, tools `9`, assistant calls `6`, wall `47.78`, raw `<run-root>/production/migration-style-copy/turn-1/events.jsonl`.
- `production/import-icalendar` turn 1: exit `0`, tools `7`, assistant calls `4`, wall `43.20`, raw `<run-root>/production/import-icalendar/turn-1/events.jsonl`.
- `production/reminder-create-query-dismiss` turn 1: exit `0`, tools `8`, assistant calls `5`, wall `31.49`, raw `<run-root>/production/reminder-create-query-dismiss/turn-1/events.jsonl`.
- `production/mt-clarify-then-create` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `4.67`, raw `<run-root>/production/mt-clarify-then-create/turn-1/events.jsonl`.
- `production/mt-clarify-then-create` turn 2: exit `0`, tools `2`, assistant calls `3`, wall `11.57`, raw `<run-root>/production/mt-clarify-then-create/turn-2/events.jsonl`.
- `production/mt-list-then-complete` turn 1: exit `0`, tools `2`, assistant calls `3`, wall `14.19`, raw `<run-root>/production/mt-list-then-complete/turn-1/events.jsonl`.
- `production/mt-list-then-complete` turn 2: exit `0`, tools `1`, assistant calls `2`, wall `8.93`, raw `<run-root>/production/mt-list-then-complete/turn-2/events.jsonl`.
- `production/mt-disambiguate-calendar` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `13.22`, raw `<run-root>/production/mt-disambiguate-calendar/turn-1/events.jsonl`.
- `production/mt-disambiguate-calendar` turn 2: exit `0`, tools `5`, assistant calls `5`, wall `27.60`, raw `<run-root>/production/mt-disambiguate-calendar/turn-2/events.jsonl`.
