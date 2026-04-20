# OpenPlanner JSON Runner Eval 2026-04-20-skill-context

- Model: `gpt-5.4-mini`
- Reasoning effort: `medium`
- Parallelism: `1`
- Cache mode: `shared`
- Cache prewarm seconds: `17.29`
- Harness elapsed seconds: `85.05`
- Effective parallel speedup: `0.41x`
- Parallel efficiency: `0.41`
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
| `aggregate_non_cached_tokens_reported` | true | 3/3 scenarios exposed usage; aggregate non-cached input tokens: 49340 |
| `expanded_category_coverage` | true | filtered run; full-suite category coverage not enforced |

## Scenario Coverage

| Category | Required | Passed | Feature State | Scenarios | Details |
| --- | ---: | ---: | --- | --- | --- |
| `advanced_recurrence` | true | true | `` |  | filtered run; full-suite category coverage not enforced |
| `future_surface` | true | true | `` |  | filtered run; full-suite category coverage not enforced |
| `migration` | true | true | `` |  | filtered run; full-suite category coverage not enforced |
| `multi_turn_disambiguation` | true | true | `` |  | filtered run; full-suite category coverage not enforced |
| `routine` | true | true | `supported` | create-dated-task, ensure-calendar | filtered run; full-suite category coverage not enforced |
| `update` | true | true | `` |  | filtered run; full-suite category coverage not enforced |
| `validation` | false | true | `supported` | ambiguous-short-date | filtered run; full-suite category coverage not enforced |

## Results

| Scenario | Category | Feature State | Passed | Tools | Commands | Assistant Calls | Non-Cached Tokens | Wall Seconds | Details |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `ensure-calendar` | `routine` | `supported` | true | 3 | 3 | 4 | 23261 | 14.76 | turn 1: expected calendar in DB and final answer |
| `create-dated-task` | `routine` | `supported` | true | 2 | 2 | 3 | 5546 | 14.56 | turn 1: expected tasks in DB and final answer |
| `ambiguous-short-date` | `validation` | `supported` | true | 0 | 0 | 1 | 20533 | 5.51 | turn 1: expected direct rejection or clarification without DB writes |

## Phase Timings

| Phase | Seconds |
| --- | ---: |
| prepare_run_dir | 0.00 |
| copy_repo | 0.05 |
| install_variant | 46.25 |
| warm_cache | 0.00 |
| seed_db | 0.00 |
| agent_run | 34.83 |
| parse_metrics | 0.00 |
| verify | 0.00 |
| total | 85.05 |

## Turn Details

- `production/ensure-calendar` turn 1: exit `0`, tools `3`, assistant calls `4`, wall `14.76`, raw `<run-root>/production/ensure-calendar/turn-1/events.jsonl`.
- `production/create-dated-task` turn 1: exit `0`, tools `2`, assistant calls `3`, wall `14.56`, raw `<run-root>/production/create-dated-task/turn-1/events.jsonl`.
- `production/ambiguous-short-date` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `5.51`, raw `<run-root>/production/ambiguous-short-date/turn-1/events.jsonl`.
