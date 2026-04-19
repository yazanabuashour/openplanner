# OpenPlanner JSON Runner Eval 2026-04-19-runner-smoke

- Model: `gpt-5.4-mini`
- Reasoning effort: `medium`
- Parallelism: `1`
- Cache mode: `shared`
- Cache prewarm seconds: `14.74`
- Harness elapsed seconds: `38.33`
- Effective parallel speedup: `0.65x`
- Parallel efficiency: `0.65`
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
| `aggregate_non_cached_tokens_reported` | true | 1/1 scenarios exposed usage; aggregate non-cached input tokens: 7131 |

## Results

| Scenario | Passed | Tools | Commands | Assistant Calls | Non-Cached Tokens | Wall Seconds | Details |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `ensure-calendar` | true | 5 | 5 | 5 | 7131 | 25.02 | turn 1: expected calendar in DB and final answer |

## Phase Timings

| Phase | Seconds |
| --- | ---: |
| prepare_run_dir | 0.00 |
| copy_repo | 0.01 |
| install_variant | 13.29 |
| warm_cache | 0.00 |
| seed_db | 0.00 |
| agent_run | 25.02 |
| parse_metrics | 0.00 |
| verify | 0.00 |
| total | 38.33 |

## Turn Details

- `production/ensure-calendar` turn 1: exit `0`, tools `5`, assistant calls `5`, wall `25.02`, raw `<run-root>/production/ensure-calendar/turn-1/events.jsonl`.
