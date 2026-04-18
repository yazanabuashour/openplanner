# OpenPlanner AgentOps Eval maturity-throughput-speed-isolated

- Model: `gpt-5.4-mini`
- Reasoning effort: `medium`
- Parallelism: `4`
- Cache mode: `isolated`
- Harness elapsed seconds: `235.84`
- Effective parallel speedup: `2.32x`
- Parallel efficiency: `0.58`
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
| `aggregate_non_cached_tokens_reported` | true | 22/22 scenarios exposed usage; aggregate non-cached input tokens: 131160 |

## Results

| Scenario | Passed | Tools | Commands | Assistant Calls | Non-Cached Tokens | Wall Seconds | Details |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `ensure-calendar` | true | 1 | 1 | 2 | 4048 | 38.41 | turn 1: expected calendar in DB and final answer |
| `create-timed-event` | true | 1 | 1 | 3 | 9194 | 40.53 | turn 1: expected events in DB and final answer |
| `create-all-day-event` | true | 1 | 1 | 1 | 9687 | 39.32 | turn 1: expected events in DB and final answer |
| `create-dated-task` | true | 1 | 1 | 3 | 8437 | 41.40 | turn 1: expected tasks in DB and final answer |
| `create-timed-task` | true | 1 | 1 | 5 | 5941 | 28.64 | turn 1: expected tasks in DB and final answer |
| `create-recurring-event` | true | 1 | 1 | 2 | 4288 | 27.24 | turn 1: expected events in DB and final answer |
| `create-recurring-task` | true | 1 | 1 | 4 | 5229 | 26.73 | turn 1: expected tasks in DB and final answer |
| `agenda-range` | true | 1 | 1 | 1 | 8913 | 24.73 | turn 1: expected bounded agenda chronologically |
| `list-events-filter-limit` | true | 1 | 1 | 2 | 4059 | 30.41 | turn 1: expected events in DB and final answer |
| `list-tasks-filter-limit` | true | 1 | 1 | 4 | 5224 | 34.42 | turn 1: expected tasks in DB and final answer |
| `complete-task` | true | 2 | 2 | 5 | 10477 | 39.91 | turn 1: expected tasks in DB and final answer |
| `complete-recurring-task` | true | 2 | 2 | 3 | 4452 | 41.12 | turn 1: expected recurring task occurrence completed |
| `mixed-event-task` | true | 2 | 2 | 1 | 4819 | 24.33 | turn 1: expected tasks in DB and final answer |
| `ambiguous-short-date` | true | 0 | 0 | 1 | 3610 | 5.76 | turn 1: expected direct rejection or clarification without DB writes |
| `year-first-slash-date` | true | 0 | 0 | 1 | 3628 | 5.00 | turn 1: expected direct rejection or clarification without DB writes |
| `invalid-rfc3339` | true | 0 | 0 | 1 | 3651 | 5.96 | turn 1: expected direct rejection or clarification without DB writes |
| `missing-title` | true | 0 | 0 | 1 | 3599 | 4.49 | turn 1: expected direct rejection or clarification without DB writes |
| `invalid-range` | true | 0 | 0 | 1 | 3609 | 4.10 | turn 1: expected direct rejection or clarification without DB writes |
| `unsupported-recurrence` | true | 0 | 0 | 1 | 3604 | 6.08 | turn 1: expected direct rejection or clarification without DB writes |
| `non-positive-limit` | true | 0 | 0 | 1 | 3580 | 4.15 | turn 1: expected direct rejection or clarification without DB writes |
| `mt-clarify-then-create` | true | 1 | 1 | 4 | 8926 | 35.39 | turn 1: expected direct rejection or clarification without DB writes; turn 2: expected tasks in DB and final answer |
| `mt-list-then-complete` | true | 3 | 3 | 8 | 12185 | 39.87 | turn 1: expected tasks in DB and final answer; turn 2: expected tasks in DB and final answer |

## Phase Timings

| Phase | Seconds |
| --- | ---: |
| prepare_run_dir | 0.03 |
| copy_repo | 0.82 |
| install_variant | 0.01 |
| warm_cache | 293.84 |
| seed_db | 0.10 |
| agent_run | 547.99 |
| parse_metrics | 0.00 |
| verify | 0.01 |
| total | 842.88 |

## Turn Details

- `production/ensure-calendar` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `38.41`, raw `<run-root>/production/ensure-calendar/turn-1/events.jsonl`.
- `production/create-timed-event` turn 1: exit `0`, tools `1`, assistant calls `3`, wall `40.53`, raw `<run-root>/production/create-timed-event/turn-1/events.jsonl`.
- `production/create-all-day-event` turn 1: exit `0`, tools `1`, assistant calls `1`, wall `39.32`, raw `<run-root>/production/create-all-day-event/turn-1/events.jsonl`.
- `production/create-dated-task` turn 1: exit `0`, tools `1`, assistant calls `3`, wall `41.40`, raw `<run-root>/production/create-dated-task/turn-1/events.jsonl`.
- `production/create-timed-task` turn 1: exit `0`, tools `1`, assistant calls `5`, wall `28.64`, raw `<run-root>/production/create-timed-task/turn-1/events.jsonl`.
- `production/create-recurring-event` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `27.24`, raw `<run-root>/production/create-recurring-event/turn-1/events.jsonl`.
- `production/create-recurring-task` turn 1: exit `0`, tools `1`, assistant calls `4`, wall `26.73`, raw `<run-root>/production/create-recurring-task/turn-1/events.jsonl`.
- `production/agenda-range` turn 1: exit `0`, tools `1`, assistant calls `1`, wall `24.73`, raw `<run-root>/production/agenda-range/turn-1/events.jsonl`.
- `production/list-events-filter-limit` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `30.41`, raw `<run-root>/production/list-events-filter-limit/turn-1/events.jsonl`.
- `production/list-tasks-filter-limit` turn 1: exit `0`, tools `1`, assistant calls `4`, wall `34.42`, raw `<run-root>/production/list-tasks-filter-limit/turn-1/events.jsonl`.
- `production/complete-task` turn 1: exit `0`, tools `2`, assistant calls `5`, wall `39.91`, raw `<run-root>/production/complete-task/turn-1/events.jsonl`.
- `production/complete-recurring-task` turn 1: exit `0`, tools `2`, assistant calls `3`, wall `41.12`, raw `<run-root>/production/complete-recurring-task/turn-1/events.jsonl`.
- `production/mixed-event-task` turn 1: exit `0`, tools `2`, assistant calls `1`, wall `24.33`, raw `<run-root>/production/mixed-event-task/turn-1/events.jsonl`.
- `production/ambiguous-short-date` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `5.76`, raw `<run-root>/production/ambiguous-short-date/turn-1/events.jsonl`.
- `production/year-first-slash-date` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `5.00`, raw `<run-root>/production/year-first-slash-date/turn-1/events.jsonl`.
- `production/invalid-rfc3339` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `5.96`, raw `<run-root>/production/invalid-rfc3339/turn-1/events.jsonl`.
- `production/missing-title` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `4.49`, raw `<run-root>/production/missing-title/turn-1/events.jsonl`.
- `production/invalid-range` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `4.10`, raw `<run-root>/production/invalid-range/turn-1/events.jsonl`.
- `production/unsupported-recurrence` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `6.08`, raw `<run-root>/production/unsupported-recurrence/turn-1/events.jsonl`.
- `production/non-positive-limit` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `4.15`, raw `<run-root>/production/non-positive-limit/turn-1/events.jsonl`.
- `production/mt-clarify-then-create` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `6.36`, raw `<run-root>/production/mt-clarify-then-create/turn-1/events.jsonl`.
- `production/mt-clarify-then-create` turn 2: exit `0`, tools `1`, assistant calls `3`, wall `29.03`, raw `<run-root>/production/mt-clarify-then-create/turn-2/events.jsonl`.
- `production/mt-list-then-complete` turn 1: exit `0`, tools `2`, assistant calls `6`, wall `31.50`, raw `<run-root>/production/mt-list-then-complete/turn-1/events.jsonl`.
- `production/mt-list-then-complete` turn 2: exit `0`, tools `1`, assistant calls `2`, wall `8.37`, raw `<run-root>/production/mt-list-then-complete/turn-2/events.jsonl`.
