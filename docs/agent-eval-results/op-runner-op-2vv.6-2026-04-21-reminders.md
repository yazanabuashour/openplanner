# OpenPlanner JSON Runner Eval op-2vv.6-2026-04-21-reminders

- Model: `gpt-5.4-mini`
- Reasoning effort: `medium`
- Parallelism: `1`
- Cache mode: `shared`
- Cache prewarm seconds: `17.50`
- Harness elapsed seconds: `44.19`
- Effective parallel speedup: `0.60x`
- Parallel efficiency: `0.60`
- Production score: `pass`
- Comparison status: not applicable: OpenPlanner has no human CLI baseline variant; this report scores the production JSON runner surface only
- Raw logs committed: `false`
- Raw logs note: Raw codex exec event logs and stderr files were retained under <run-root> during execution and intentionally not committed.

## Production Gates

| Criterion | Passed | Details |
| --- | ---: | --- |
| `production_passes_all_scenarios` | true | 1/1 scenarios passed |
| `invalid_inputs_are_final_answer_only` | true | invalid-input scenarios used no tools, no command executions, and at most one assistant answer |
| `no_forbidden_inspection_or_cli_usage` | true | no removed-interface path inspection, module-cache inspection, direct SQLite access, CLI usage, or routine broad repo search detected |
| `aggregate_non_cached_tokens_reported` | true | 1/1 scenarios exposed usage; aggregate non-cached input tokens: 11336 |
| `expanded_category_coverage` | true | filtered run; full-suite category coverage not enforced |

## Scenario Coverage

| Category | Required | Passed | Feature State | Scenarios | Details |
| --- | ---: | ---: | --- | --- | --- |
| `advanced_recurrence` | true | true | `` |  | filtered run; full-suite category coverage not enforced |
| `future_surface` | true | true | `` |  | filtered run; full-suite category coverage not enforced |
| `migration` | true | true | `` |  | filtered run; full-suite category coverage not enforced |
| `multi_turn_disambiguation` | true | true | `` |  | filtered run; full-suite category coverage not enforced |
| `routine` | true | true | `supported` | reminder-create-query-dismiss | filtered run; full-suite category coverage not enforced |
| `update` | true | true | `` |  | filtered run; full-suite category coverage not enforced |

## Results

| Scenario | Category | Feature State | Passed | Tools | Commands | Assistant Calls | Non-Cached Tokens | Wall Seconds | Details |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `reminder-create-query-dismiss` | `routine` | `supported` | true | 6 | 6 | 5 | 11336 | 26.43 | turn 1: expected reminder stored, queried, dismissed, and absent from pending range |

## Phase Timings

| Phase | Seconds |
| --- | ---: |
| prepare_run_dir | 0.00 |
| copy_repo | 0.02 |
| install_variant | 15.84 |
| warm_cache | 0.00 |
| seed_db | 0.00 |
| agent_run | 26.43 |
| parse_metrics | 0.00 |
| verify | 0.00 |
| total | 44.18 |

## Turn Details

- `production/reminder-create-query-dismiss` turn 1: exit `0`, tools `6`, assistant calls `5`, wall `26.43`, raw `<run-root>/production/reminder-create-query-dismiss/turn-1/events.jsonl`.
