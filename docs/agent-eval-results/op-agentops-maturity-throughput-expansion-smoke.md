# OpenPlanner AgentOps Eval maturity-throughput-expansion-smoke

- Model: `gpt-5.4-mini`
- Reasoning effort: `medium`
- Parallelism: `3`
- Cache mode: `shared`
- Cache prewarm seconds: `22.11`
- Harness elapsed seconds: `26.15`
- Effective parallel speedup: `2.37x`
- Parallel efficiency: `0.79`
- Production score: `pass`
- Comparison status: not applicable: OpenPlanner has no human CLI baseline variant; this report scores the production AgentOps surface only
- Raw logs committed: `false`
- Raw logs note: Raw codex exec event logs and stderr files were retained under <run-root> during execution and intentionally not committed.

## Production Gates

| Criterion | Passed | Details |
| --- | ---: | --- |
| `production_passes_all_scenarios` | true | 6/6 scenarios passed |
| `invalid_inputs_are_final_answer_only` | true | invalid-input scenarios used no tools, no command executions, and at most one assistant answer |
| `no_forbidden_inspection_or_cli_usage` | true | no generated-file inspection, generated-path broad search, module-cache inspection, direct SQLite access, CLI usage, or routine broad repo search detected |
| `aggregate_non_cached_tokens_reported` | true | 6/6 scenarios exposed usage; aggregate non-cached input tokens: 36239 |

## Results

| Scenario | Passed | Tools | Commands | Assistant Calls | Non-Cached Tokens | Wall Seconds | Details |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `create-timed-event` | true | 1 | 1 | 2 | 4036 | 7.84 | turn 1: expected events in DB and final answer |
| `agenda-range` | true | 1 | 1 | 2 | 4083 | 9.36 | turn 1: expected bounded agenda chronologically |
| `complete-task` | true | 2 | 2 | 3 | 4410 | 11.31 | turn 1: expected tasks in DB and final answer |
| `year-first-slash-date` | true | 0 | 0 | 1 | 3555 | 7.59 | turn 1: expected direct rejection or clarification without DB writes |
| `mt-clarify-then-create` | true | 1 | 1 | 3 | 11454 | 11.16 | turn 1: expected direct rejection or clarification without DB writes; turn 2: expected tasks in DB and final answer |
| `mt-list-then-complete` | true | 2 | 2 | 4 | 8701 | 14.77 | turn 1: expected tasks in DB and final answer; turn 2: expected tasks in DB and final answer |

## Phase Timings

| Phase | Seconds |
| --- | ---: |
| prepare_run_dir | 0.00 |
| copy_repo | 0.15 |
| install_variant | 0.00 |
| warm_cache | 0.00 |
| seed_db | 0.03 |
| agent_run | 62.03 |
| parse_metrics | 0.00 |
| verify | 0.00 |
| total | 62.22 |

## Turn Details

- `production/create-timed-event` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `7.84`, raw `<run-root>/production/create-timed-event/turn-1/events.jsonl`.
- `production/agenda-range` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `9.36`, raw `<run-root>/production/agenda-range/turn-1/events.jsonl`.
- `production/complete-task` turn 1: exit `0`, tools `2`, assistant calls `3`, wall `11.31`, raw `<run-root>/production/complete-task/turn-1/events.jsonl`.
- `production/year-first-slash-date` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `7.59`, raw `<run-root>/production/year-first-slash-date/turn-1/events.jsonl`.
- `production/mt-clarify-then-create` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `4.53`, raw `<run-root>/production/mt-clarify-then-create/turn-1/events.jsonl`.
- `production/mt-clarify-then-create` turn 2: exit `0`, tools `1`, assistant calls `2`, wall `6.63`, raw `<run-root>/production/mt-clarify-then-create/turn-2/events.jsonl`.
- `production/mt-list-then-complete` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `8.71`, raw `<run-root>/production/mt-list-then-complete/turn-1/events.jsonl`.
- `production/mt-list-then-complete` turn 2: exit `0`, tools `1`, assistant calls `2`, wall `6.06`, raw `<run-root>/production/mt-list-then-complete/turn-2/events.jsonl`.
