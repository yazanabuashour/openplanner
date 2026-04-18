# OpenPlanner AgentOps Eval maturity-throughput-validation-smoke

- Model: `gpt-5.4-mini`
- Reasoning effort: `medium`
- Parallelism: `2`
- Cache mode: `shared`
- Cache prewarm seconds: `17.78`
- Harness elapsed seconds: `7.75`
- Effective parallel speedup: `1.75x`
- Parallel efficiency: `0.88`
- Production score: `pass`
- Comparison status: not applicable: OpenPlanner has no human CLI baseline variant; this report scores the production AgentOps surface only
- Raw logs committed: `false`
- Raw logs note: Raw codex exec event logs and stderr files were retained under <run-root> during execution and intentionally not committed.

## Production Gates

| Criterion | Passed | Details |
| --- | ---: | --- |
| `production_passes_all_scenarios` | true | 2/2 scenarios passed |
| `invalid_inputs_are_final_answer_only` | true | invalid-input scenarios used no tools, no command executions, and at most one assistant answer |
| `no_forbidden_inspection_or_cli_usage` | true | no generated-file inspection, generated-path broad search, module-cache inspection, direct SQLite access, CLI usage, or routine broad repo search detected |
| `aggregate_non_cached_tokens_reported` | true | 2/2 scenarios exposed usage; aggregate non-cached input tokens: 7387 |

## Results

| Scenario | Passed | Tools | Commands | Assistant Calls | Non-Cached Tokens | Wall Seconds | Details |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `ensure-calendar` | true | 1 | 1 | 2 | 3865 | 7.72 | turn 1: expected calendar in DB and final answer |
| `ambiguous-short-date` | true | 0 | 0 | 1 | 3522 | 5.83 | turn 1: expected direct rejection or clarification without DB writes |

## Phase Timings

| Phase | Seconds |
| --- | ---: |
| prepare_run_dir | 0.00 |
| copy_repo | 0.04 |
| install_variant | 0.00 |
| warm_cache | 0.00 |
| seed_db | 0.00 |
| agent_run | 13.55 |
| parse_metrics | 0.00 |
| verify | 0.00 |
| total | 13.61 |

## Turn Details

- `production/ensure-calendar` turn 1: exit `0`, tools `1`, assistant calls `2`, wall `7.72`, raw `<run-root>/production/ensure-calendar/turn-1/events.jsonl`.
- `production/ambiguous-short-date` turn 1: exit `0`, tools `0`, assistant calls `1`, wall `5.83`, raw `<run-root>/production/ambiguous-short-date/turn-1/events.jsonl`.
