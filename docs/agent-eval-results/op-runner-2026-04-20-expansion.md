# OpenPlanner JSON Runner Eval 2026-04-20-expansion

- Model: `gpt-5.4-mini`
- Reasoning effort: `medium`
- Parallelism: `4`
- Cache mode: `shared`
- Cache prewarm seconds: `21.60`
- Harness elapsed seconds: `162.87`
- Effective parallel speedup: `1.12x`
- Parallel efficiency: `0.28`
- Production score: `pass`
- Comparison status: not applicable: OpenPlanner has no human CLI baseline variant; this report scores the production JSON runner surface only
- Raw logs committed: `false`
- Raw logs note: Raw codex exec event logs and stderr files were retained under <run-root> during execution and intentionally not committed.

## Production Gates

| Criterion | Passed | Details |
| --- | ---: | --- |
| `production_passes_all_scenarios` | true | 11/11 scenarios passed |
| `invalid_inputs_are_final_answer_only` | true | invalid-input scenarios used no tools, no command executions, and at most one assistant answer |
| `no_forbidden_inspection_or_cli_usage` | true | no removed-interface path inspection, module-cache inspection, direct SQLite access, CLI usage, or routine broad repo search detected |
| `aggregate_non_cached_tokens_reported` | true | 11/11 scenarios exposed usage; aggregate non-cached input tokens: 69325 |
| `expanded_category_coverage` | true | filtered run; full-suite category coverage not enforced |

## Scenario Coverage

| Category | Required | Passed | Feature State | Scenarios | Details |
| --- | ---: | ---: | --- | --- | --- |
| `advanced_recurrence` | true | true | `supported` | monthly-recurrence-by-month-day, weekly-recurrence-by-weekday | filtered run; full-suite category coverage not enforced |
| `future_surface` | true | true | `unsupported_until_landed` | unsupported-delete, unsupported-import-export, unsupported-reminder, unsupported-task-metadata | filtered run; full-suite category coverage not enforced |
| `migration` | true | true | `supported` | migration-style-copy | filtered run; full-suite category coverage not enforced |
| `multi_turn_disambiguation` | true | true | `supported` | mt-disambiguate-calendar | filtered run; full-suite category coverage not enforced |
| `routine` | true | true | `` |  | filtered run; full-suite category coverage not enforced |
| `update` | true | true | `supported` | update-calendar-metadata, update-event-patch-clear, update-task-due-mode | filtered run; full-suite category coverage not enforced |

## Results

| Scenario | Category | Feature State | Passed | Tools | Commands | Assistant Calls | Non-Cached Tokens | Wall Seconds | Details |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `update-calendar-metadata` | `update` | `supported` | true | 4 | 4 | 4 | 6166 | 25.09 | turn 1: expected calendar metadata in DB and final answer |
| `update-event-patch-clear` | `update` | `supported` | true | 3 | 3 | 4 | 5785 | 21.64 | turn 1: expected events in DB and final answer |
| `update-task-due-mode` | `update` | `supported` | true | 3 | 3 | 4 | 5860 | 14.98 | turn 1: expected tasks in DB and final answer |
| `weekly-recurrence-by-weekday` | `advanced_recurrence` | `supported` | true | 2 | 2 | 3 | 5732 | 16.96 | turn 1: expected recurrence occurrences in agenda |
| `monthly-recurrence-by-month-day` | `advanced_recurrence` | `supported` | true | 2 | 2 | 3 | 5724 | 14.86 | turn 1: expected recurrence occurrences in agenda |
| `migration-style-copy` | `migration` | `supported` | true | 7 | 7 | 5 | 7249 | 30.40 | turn 1: expected copied Work items while Legacy items remain |
| `unsupported-import-export` | `future_surface` | `unsupported_until_landed` | true | 1 | 1 | 2 | 5194 | 12.36 | turn 1: expected unsupported-workflow answer without DB writes |
| `unsupported-delete` | `future_surface` | `unsupported_until_landed` | true | 1 | 1 | 2 | 5126 | 6.32 | turn 1: expected unsupported-workflow answer without DB writes |
| `unsupported-reminder` | `future_surface` | `unsupported_until_landed` | true | 1 | 1 | 2 | 5201 | 6.40 | turn 1: expected unsupported-workflow answer without DB writes |
| `unsupported-task-metadata` | `future_surface` | `unsupported_until_landed` | true | 1 | 1 | 2 | 5246 | 8.43 | turn 1: expected unsupported-workflow answer without DB writes |
| `mt-disambiguate-calendar` | `multi_turn_disambiguation` | `supported` | true | 3 | 3 | 5 | 12042 | 24.19 | turn 1: expected clarification before creating destination copy; turn 2: expected copied Work task while Legacy task remains |

## Phase Timings

| Phase | Seconds |
| --- | ---: |
| prepare_run_dir | 0.00 |
| copy_repo | 0.21 |
| install_variant | 376.17 |
| warm_cache | 0.00 |
| seed_db | 0.03 |
| agent_run | 181.63 |
| parse_metrics | 0.00 |
| verify | 0.00 |
| total | 558.09 |

## Turn Details

- `production/update-calendar-metadata` turn 1: exit `0`, tools `4`, assistant calls `4`, wall `25.09`, raw `<run-root>/production/update-calendar-metadata/turn-1/events.jsonl`.
- `production/update-event-patch-clear` turn 1: exit `0`, tools `3`, assistant calls `4`, wall `21.64`, raw `<run-root>/production/update-event-patch-clear/turn-1/events.jsonl`.
- `production/update-task-due-mode` turn 1: exit `0`, tools `3`, assistant calls `4`, wall `14.98`, raw `<run-root>/production/update-task-due-mode/turn-1/events.jsonl`.
- `production/weekly-recurrence-by-weekday` turn 1: exit `0`, tools `2`, assistant calls `3`, wall `16.96`, raw `<run-root>/production/weekly-recurrence-by-weekday/turn-1/events.jsonl`.
- `production/monthly-recurrence-by-month-day` turn 1: exit `0`, tools `2`, assistant calls `3`, wall `14.86`, raw `<run-root>/production/monthly-recurrence-by-month-day/turn-1/events.jsonl`.
- `production/migration-style-copy` turn 1: exit `0`, tools `7`, assistant calls `5`, wall `30.40`, raw `<run-root>/production/migration-style-copy/turn-1/events.jsonl`.
- `production/unsupported-import-export` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `12.36`, raw `<run-root>/production/unsupported-import-export/turn-1/events.jsonl`.
- `production/unsupported-delete` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `6.32`, raw `<run-root>/production/unsupported-delete/turn-1/events.jsonl`.
- `production/unsupported-reminder` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `6.40`, raw `<run-root>/production/unsupported-reminder/turn-1/events.jsonl`.
- `production/unsupported-task-metadata` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `8.43`, raw `<run-root>/production/unsupported-task-metadata/turn-1/events.jsonl`.
- `production/mt-disambiguate-calendar` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `12.26`, raw `<run-root>/production/mt-disambiguate-calendar/turn-1/events.jsonl`.
- `production/mt-disambiguate-calendar` turn 2: exit `0`, tools `2`, assistant calls `3`, wall `11.93`, raw `<run-root>/production/mt-disambiguate-calendar/turn-2/events.jsonl`.
