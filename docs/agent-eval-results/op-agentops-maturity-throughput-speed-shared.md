# OpenPlanner AgentOps Eval maturity-throughput-speed-shared

- Model: `gpt-5.4-mini`
- Reasoning effort: `medium`
- Parallelism: `4`
- Cache mode: `shared`
- Cache prewarm seconds: `20.05`
- Harness elapsed seconds: `60.13`
- Effective parallel speedup: `3.46x`
- Parallel efficiency: `0.87`
- Production score: `pass`
- Comparison status: not applicable: OpenPlanner has no human CLI baseline variant; this report scores the production AgentOps surface only
- Raw logs committed: `false`
- Raw logs note: Raw codex exec event logs and stderr files were retained under <run-root> during execution and intentionally not committed.

## Production Gates

| Criterion | Passed | Details |
| --- | ---: | --- |
| `production_passes_all_scenarios` | true | 22/22 scenarios passed |
| `invalid_inputs_are_final_answer_only` | true | invalid-input scenarios used no tools, no command executions, and at most one assistant answer |
| `no_forbidden_inspection_or_cli_usage` | true | no generated-file inspection, generated-path broad search, module-cache inspection, direct SQLite access, CLI usage, or routine broad repo search detected |
| `aggregate_non_cached_tokens_reported` | true | 22/22 scenarios exposed usage; aggregate non-cached input tokens: 109204 |

## Results

| Scenario | Passed | Tools | Commands | Assistant Calls | Non-Cached Tokens | Wall Seconds | Details |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `ensure-calendar` | true | 1 | 1 | 3 | 8364 | 11.74 | turn 1: expected calendar in DB and final answer |
| `create-timed-event` | true | 1 | 1 | 2 | 4387 | 13.56 | turn 1: expected events in DB and final answer |
| `create-all-day-event` | true | 1 | 1 | 3 | 4839 | 9.26 | turn 1: expected events in DB and final answer |
| `create-dated-task` | true | 1 | 1 | 2 | 4716 | 11.33 | turn 1: expected tasks in DB and final answer |
| `create-timed-task` | true | 1 | 1 | 2 | 4250 | 8.41 | turn 1: expected tasks in DB and final answer |
| `create-recurring-event` | true | 1 | 1 | 2 | 4358 | 11.55 | turn 1: expected events in DB and final answer |
| `create-recurring-task` | true | 1 | 1 | 3 | 4446 | 9.32 | turn 1: expected tasks in DB and final answer |
| `agenda-range` | true | 1 | 1 | 2 | 4296 | 7.90 | turn 1: expected bounded agenda chronologically |
| `list-events-filter-limit` | true | 1 | 1 | 2 | 4121 | 9.39 | turn 1: expected events in DB and final answer |
| `list-tasks-filter-limit` | true | 1 | 1 | 1 | 4068 | 8.98 | turn 1: expected tasks in DB and final answer |
| `complete-task` | true | 2 | 2 | 3 | 4239 | 11.58 | turn 1: expected tasks in DB and final answer |
| `complete-recurring-task` | true | 2 | 2 | 4 | 9275 | 14.59 | turn 1: expected recurring task occurrence completed |
| `mixed-event-task` | true | 2 | 2 | 3 | 5048 | 14.65 | turn 1: expected tasks in DB and final answer |
| `ambiguous-short-date` | true | 0 | 0 | 1 | 3645 | 4.65 | turn 1: expected direct rejection or clarification without DB writes |
| `year-first-slash-date` | true | 0 | 0 | 1 | 3663 | 3.72 | turn 1: expected direct rejection or clarification without DB writes |
| `invalid-rfc3339` | true | 0 | 0 | 1 | 3686 | 6.79 | turn 1: expected direct rejection or clarification without DB writes |
| `missing-title` | true | 0 | 0 | 1 | 3634 | 5.15 | turn 1: expected direct rejection or clarification without DB writes |
| `invalid-range` | true | 0 | 0 | 1 | 3644 | 3.53 | turn 1: expected direct rejection or clarification without DB writes |
| `unsupported-recurrence` | true | 0 | 0 | 1 | 3639 | 5.70 | turn 1: expected direct rejection or clarification without DB writes |
| `non-positive-limit` | true | 0 | 0 | 1 | 3615 | 6.42 | turn 1: expected direct rejection or clarification without DB writes |
| `mt-clarify-then-create` | true | 1 | 1 | 3 | 7976 | 11.59 | turn 1: expected direct rejection or clarification without DB writes; turn 2: expected tasks in DB and final answer |
| `mt-list-then-complete` | true | 2 | 2 | 4 | 9295 | 18.07 | turn 1: expected tasks in DB and final answer; turn 2: expected tasks in DB and final answer |

## Phase Timings

| Phase | Seconds |
| --- | ---: |
| prepare_run_dir | 0.00 |
| copy_repo | 0.47 |
| install_variant | 0.00 |
| warm_cache | 0.00 |
| seed_db | 0.06 |
| agent_run | 207.88 |
| parse_metrics | 0.00 |
| verify | 0.00 |
| total | 208.45 |

## Turn Details

- `production/ensure-calendar` turn 1: exit `0`, tools `1`, assistant calls `3`, wall `11.74`, raw `<run-root>/production/ensure-calendar/turn-1/events.jsonl`.
- `production/create-timed-event` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `13.56`, raw `<run-root>/production/create-timed-event/turn-1/events.jsonl`.
- `production/create-all-day-event` turn 1: exit `0`, tools `1`, assistant calls `3`, wall `9.26`, raw `<run-root>/production/create-all-day-event/turn-1/events.jsonl`.
- `production/create-dated-task` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `11.33`, raw `<run-root>/production/create-dated-task/turn-1/events.jsonl`.
- `production/create-timed-task` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `8.41`, raw `<run-root>/production/create-timed-task/turn-1/events.jsonl`.
- `production/create-recurring-event` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `11.55`, raw `<run-root>/production/create-recurring-event/turn-1/events.jsonl`.
- `production/create-recurring-task` turn 1: exit `0`, tools `1`, assistant calls `3`, wall `9.32`, raw `<run-root>/production/create-recurring-task/turn-1/events.jsonl`.
- `production/agenda-range` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `7.90`, raw `<run-root>/production/agenda-range/turn-1/events.jsonl`.
- `production/list-events-filter-limit` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `9.39`, raw `<run-root>/production/list-events-filter-limit/turn-1/events.jsonl`.
- `production/list-tasks-filter-limit` turn 1: exit `0`, tools `1`, assistant calls `1`, wall `8.98`, raw `<run-root>/production/list-tasks-filter-limit/turn-1/events.jsonl`.
- `production/complete-task` turn 1: exit `0`, tools `2`, assistant calls `3`, wall `11.58`, raw `<run-root>/production/complete-task/turn-1/events.jsonl`.
- `production/complete-recurring-task` turn 1: exit `0`, tools `2`, assistant calls `4`, wall `14.59`, raw `<run-root>/production/complete-recurring-task/turn-1/events.jsonl`.
- `production/mixed-event-task` turn 1: exit `0`, tools `2`, assistant calls `3`, wall `14.65`, raw `<run-root>/production/mixed-event-task/turn-1/events.jsonl`.
- `production/ambiguous-short-date` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `4.65`, raw `<run-root>/production/ambiguous-short-date/turn-1/events.jsonl`.
- `production/year-first-slash-date` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `3.72`, raw `<run-root>/production/year-first-slash-date/turn-1/events.jsonl`.
- `production/invalid-rfc3339` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `6.79`, raw `<run-root>/production/invalid-rfc3339/turn-1/events.jsonl`.
- `production/missing-title` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `5.15`, raw `<run-root>/production/missing-title/turn-1/events.jsonl`.
- `production/invalid-range` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `3.53`, raw `<run-root>/production/invalid-range/turn-1/events.jsonl`.
- `production/unsupported-recurrence` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `5.70`, raw `<run-root>/production/unsupported-recurrence/turn-1/events.jsonl`.
- `production/non-positive-limit` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `6.42`, raw `<run-root>/production/non-positive-limit/turn-1/events.jsonl`.
- `production/mt-clarify-then-create` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `4.54`, raw `<run-root>/production/mt-clarify-then-create/turn-1/events.jsonl`.
- `production/mt-clarify-then-create` turn 2: exit `0`, tools `1`, assistant calls `2`, wall `7.05`, raw `<run-root>/production/mt-clarify-then-create/turn-2/events.jsonl`.
- `production/mt-list-then-complete` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `8.33`, raw `<run-root>/production/mt-list-then-complete/turn-1/events.jsonl`.
- `production/mt-list-then-complete` turn 2: exit `0`, tools `1`, assistant calls `2`, wall `9.74`, raw `<run-root>/production/mt-list-then-complete/turn-2/events.jsonl`.
