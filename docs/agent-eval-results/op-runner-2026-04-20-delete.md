# OpenPlanner JSON Runner Eval 2026-04-20-delete

- Model: `gpt-5.4-mini`
- Reasoning effort: `medium`
- Parallelism: `1`
- Cache mode: `shared`
- Cache prewarm seconds: `19.43`
- Harness elapsed seconds: `90.62`
- Effective parallel speedup: `0.42x`
- Parallel efficiency: `0.42`
- Production score: `pass`
- Comparison status: not applicable: OpenPlanner has no human CLI baseline variant; this report scores the production JSON runner surface only
- Raw logs committed: `false`
- Raw logs note: Raw codex exec event logs and stderr files were retained under <run-root> during execution and intentionally not committed.

## Production Gates

| Criterion | Passed | Details |
| --- | ---: | --- |
| `production_passes_all_scenarios` | true | 3/3 scenarios passed |
| `invalid_inputs_are_final_answer_only` | true | invalid-input scenarios used no tools, no command executions, and at most one assistant answer |
| `no_forbidden_inspection_or_cli_usage` | true | no removed-interface path inspection, module-cache inspection, direct SQLite access, CLI usage, or routine broad repo search detected |
| `aggregate_non_cached_tokens_reported` | true | 3/3 scenarios exposed usage; aggregate non-cached input tokens: 71009 |
| `expanded_category_coverage` | true | filtered run; full-suite category coverage not enforced |

## Scenario Coverage

| Category | Required | Passed | Feature State | Scenarios | Details |
| --- | ---: | ---: | --- | --- | --- |
| `advanced_recurrence` | true | true | `` |  | filtered run; full-suite category coverage not enforced |
| `future_surface` | true | true | `` |  | filtered run; full-suite category coverage not enforced |
| `migration` | true | true | `` |  | filtered run; full-suite category coverage not enforced |
| `multi_turn_disambiguation` | true | true | `` |  | filtered run; full-suite category coverage not enforced |
| `routine` | true | true | `supported` | delete-empty-calendar, delete-event, delete-task | filtered run; full-suite category coverage not enforced |
| `update` | true | true | `` |  | filtered run; full-suite category coverage not enforced |

## Results

| Scenario | Category | Feature State | Passed | Tools | Commands | Assistant Calls | Non-Cached Tokens | Wall Seconds | Details |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `delete-task` | `routine` | `supported` | true | 4 | 4 | 4 | 23931 | 14.63 | turn 1: expected target task deleted while keep task remains |
| `delete-event` | `routine` | `supported` | true | 3 | 3 | 4 | 23830 | 13.79 | turn 1: expected target event deleted while keep event remains |
| `delete-empty-calendar` | `routine` | `supported` | true | 2 | 2 | 3 | 23248 | 10.05 | turn 1: expected empty calendar deleted before verification recreated it |

## Phase Timings

| Phase | Seconds |
| --- | ---: |
| prepare_run_dir | 0.00 |
| copy_repo | 0.06 |
| install_variant | 52.07 |
| warm_cache | 0.00 |
| seed_db | 0.02 |
| agent_run | 38.47 |
| parse_metrics | 0.00 |
| verify | 0.00 |
| total | 90.62 |

## Turn Details

- `production/delete-task` turn 1: exit `0`, tools `4`, assistant calls `4`, wall `14.63`, raw `<run-root>/production/delete-task/turn-1/events.jsonl`.
- `production/delete-event` turn 1: exit `0`, tools `3`, assistant calls `4`, wall `13.79`, raw `<run-root>/production/delete-event/turn-1/events.jsonl`.
- `production/delete-empty-calendar` turn 1: exit `0`, tools `2`, assistant calls `3`, wall `10.05`, raw `<run-root>/production/delete-empty-calendar/turn-1/events.jsonl`.
