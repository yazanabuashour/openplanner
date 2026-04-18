# OpenPlanner AgentOps Eval maturity-throughput-final

- Model: `gpt-5.4-mini`
- Reasoning effort: `medium`
- Parallelism: `4`
- Cache mode: `shared`
- Cache prewarm seconds: `18.77`
- Harness elapsed seconds: `61.96`
- Effective parallel speedup: `3.31x`
- Parallel efficiency: `0.83`
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
| `aggregate_non_cached_tokens_reported` | true | 22/22 scenarios exposed usage; aggregate non-cached input tokens: 98852 |

## Results

| Scenario | Passed | Tools | Commands | Assistant Calls | Non-Cached Tokens | Wall Seconds | Details |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `ensure-calendar` | true | 1 | 1 | 2 | 4110 | 8.09 | turn 1: expected calendar in DB and final answer |
| `create-timed-event` | true | 1 | 1 | 1 | 4049 | 10.76 | turn 1: expected events in DB and final answer |
| `create-all-day-event` | true | 1 | 1 | 2 | 4338 | 8.54 | turn 1: expected events in DB and final answer |
| `create-dated-task` | true | 1 | 1 | 3 | 4623 | 10.50 | turn 1: expected tasks in DB and final answer |
| `create-timed-task` | true | 1 | 1 | 1 | 4285 | 8.35 | turn 1: expected tasks in DB and final answer |
| `create-recurring-event` | true | 1 | 1 | 2 | 4327 | 9.34 | turn 1: expected events in DB and final answer |
| `create-recurring-task` | true | 1 | 1 | 2 | 4316 | 9.72 | turn 1: expected tasks in DB and final answer |
| `agenda-range` | true | 1 | 1 | 1 | 4281 | 9.22 | turn 1: expected bounded agenda chronologically |
| `list-events-filter-limit` | true | 1 | 1 | 3 | 4574 | 13.58 | turn 1: expected events in DB and final answer |
| `list-tasks-filter-limit` | true | 1 | 1 | 2 | 4104 | 7.68 | turn 1: expected tasks in DB and final answer |
| `complete-task` | true | 2 | 2 | 3 | 4652 | 14.92 | turn 1: expected tasks in DB and final answer |
| `complete-recurring-task` | true | 2 | 2 | 3 | 4337 | 15.41 | turn 1: expected recurring task occurrence completed |
| `mixed-event-task` | true | 2 | 2 | 3 | 4854 | 12.54 | turn 1: expected tasks in DB and final answer |
| `ambiguous-short-date` | true | 0 | 0 | 1 | 3639 | 4.39 | turn 1: expected direct rejection or clarification without DB writes |
| `year-first-slash-date` | true | 0 | 0 | 1 | 3657 | 4.01 | turn 1: expected direct rejection or clarification without DB writes |
| `invalid-rfc3339` | true | 0 | 0 | 1 | 3680 | 7.17 | turn 1: expected direct rejection or clarification without DB writes |
| `missing-title` | true | 0 | 0 | 1 | 3116 | 6.97 | turn 1: expected direct rejection or clarification without DB writes |
| `invalid-range` | true | 0 | 0 | 1 | 3638 | 4.29 | turn 1: expected direct rejection or clarification without DB writes |
| `unsupported-recurrence` | true | 0 | 0 | 1 | 3121 | 3.74 | turn 1: expected direct rejection or clarification without DB writes |
| `non-positive-limit` | true | 0 | 0 | 1 | 3609 | 4.29 | turn 1: expected direct rejection or clarification without DB writes |
| `mt-clarify-then-create` | true | 1 | 1 | 3 | 7876 | 11.86 | turn 1: expected direct rejection or clarification without DB writes; turn 2: expected tasks in DB and final answer |
| `mt-list-then-complete` | true | 3 | 3 | 5 | 9666 | 19.44 | turn 1: expected tasks in DB and final answer; turn 2: expected tasks in DB and final answer |

## Phase Timings

| Phase | Seconds |
| --- | ---: |
| prepare_run_dir | 0.00 |
| copy_repo | 0.40 |
| install_variant | 0.00 |
| warm_cache | 0.00 |
| seed_db | 0.06 |
| agent_run | 204.81 |
| parse_metrics | 0.00 |
| verify | 0.00 |
| total | 205.32 |

## Turn Details

- `production/ensure-calendar` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `8.09`, raw `<run-root>/production/ensure-calendar/turn-1/events.jsonl`.
- `production/create-timed-event` turn 1: exit `0`, tools `1`, assistant calls `1`, wall `10.76`, raw `<run-root>/production/create-timed-event/turn-1/events.jsonl`.
- `production/create-all-day-event` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `8.54`, raw `<run-root>/production/create-all-day-event/turn-1/events.jsonl`.
- `production/create-dated-task` turn 1: exit `0`, tools `1`, assistant calls `3`, wall `10.50`, raw `<run-root>/production/create-dated-task/turn-1/events.jsonl`.
- `production/create-timed-task` turn 1: exit `0`, tools `1`, assistant calls `1`, wall `8.35`, raw `<run-root>/production/create-timed-task/turn-1/events.jsonl`.
- `production/create-recurring-event` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `9.34`, raw `<run-root>/production/create-recurring-event/turn-1/events.jsonl`.
- `production/create-recurring-task` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `9.72`, raw `<run-root>/production/create-recurring-task/turn-1/events.jsonl`.
- `production/agenda-range` turn 1: exit `0`, tools `1`, assistant calls `1`, wall `9.22`, raw `<run-root>/production/agenda-range/turn-1/events.jsonl`.
- `production/list-events-filter-limit` turn 1: exit `0`, tools `1`, assistant calls `3`, wall `13.58`, raw `<run-root>/production/list-events-filter-limit/turn-1/events.jsonl`.
- `production/list-tasks-filter-limit` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `7.68`, raw `<run-root>/production/list-tasks-filter-limit/turn-1/events.jsonl`.
- `production/complete-task` turn 1: exit `0`, tools `2`, assistant calls `3`, wall `14.92`, raw `<run-root>/production/complete-task/turn-1/events.jsonl`.
- `production/complete-recurring-task` turn 1: exit `0`, tools `2`, assistant calls `3`, wall `15.41`, raw `<run-root>/production/complete-recurring-task/turn-1/events.jsonl`.
- `production/mixed-event-task` turn 1: exit `0`, tools `2`, assistant calls `3`, wall `12.54`, raw `<run-root>/production/mixed-event-task/turn-1/events.jsonl`.
- `production/ambiguous-short-date` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `4.39`, raw `<run-root>/production/ambiguous-short-date/turn-1/events.jsonl`.
- `production/year-first-slash-date` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `4.01`, raw `<run-root>/production/year-first-slash-date/turn-1/events.jsonl`.
- `production/invalid-rfc3339` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `7.17`, raw `<run-root>/production/invalid-rfc3339/turn-1/events.jsonl`.
- `production/missing-title` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `6.97`, raw `<run-root>/production/missing-title/turn-1/events.jsonl`.
- `production/invalid-range` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `4.29`, raw `<run-root>/production/invalid-range/turn-1/events.jsonl`.
- `production/unsupported-recurrence` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `3.74`, raw `<run-root>/production/unsupported-recurrence/turn-1/events.jsonl`.
- `production/non-positive-limit` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `4.29`, raw `<run-root>/production/non-positive-limit/turn-1/events.jsonl`.
- `production/mt-clarify-then-create` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `4.12`, raw `<run-root>/production/mt-clarify-then-create/turn-1/events.jsonl`.
- `production/mt-clarify-then-create` turn 2: exit `0`, tools `1`, assistant calls `2`, wall `7.74`, raw `<run-root>/production/mt-clarify-then-create/turn-2/events.jsonl`.
- `production/mt-list-then-complete` turn 1: exit `0`, tools `2`, assistant calls `3`, wall `14.12`, raw `<run-root>/production/mt-list-then-complete/turn-1/events.jsonl`.
- `production/mt-list-then-complete` turn 2: exit `0`, tools `1`, assistant calls `2`, wall `5.32`, raw `<run-root>/production/mt-list-then-complete/turn-2/events.jsonl`.
