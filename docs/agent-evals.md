# Agent Evaluation Protocol

OpenPlanner agent evals measure the same production skill a real agent receives.
Do not add hidden evaluator-only instructions to improve a result; if an
instruction is needed, put it in the production skill first.

## Active Surface

- `production`: the installed `skills/openplanner` AgentOps JSON runner skill.

The SDK and generated client remain supported Go developer/runtime APIs, but
they are not production agent-facing eval variants. Comparison variants, if
added later, must live under eval assets or archives and must not be mixed into
the production skill.

## Scenario Coverage

The production matrix covers routine local planning tasks:

- calendar ensure and duplicate-calendar avoidance
- timed and all-day event creation
- dated, timed, and recurring task creation
- recurring event creation
- bounded agenda listing with chronological output
- event and task listing with limits and calendar filters
- task completion for non-recurring and recurring occurrences
- mixed event and task requests in one user task
- invalid input rejection for ambiguous short dates, year-first slash dates,
  invalid RFC3339 values, invalid ranges, unsupported recurrence, missing
  titles, and non-positive limits
- true multi-turn requests that require clarification or conversational context

Every scenario uses a fresh isolated repo copy, a fresh local database path, and
reduced JSON/Markdown artifacts. Raw logs are not committed; reduced reports
refer to them with `<run-root>` placeholders. The copied repo omits root
`AGENTS.md`, stale `.agents` content, eval docs, reports, and harness code
before installing eval-specific production instructions so repo-level maintainer
instructions do not contaminate user-data tasks.

Single-turn scenarios use `codex exec --ephemeral`. Multi-turn scenarios use one
persisted eval session per scenario: the first turn creates a session in the
throwaway run directory context, and later turns use `codex exec resume` with
explicit writable roots for the scenario run directory and shared Go cache.
Per-turn raw logs live under `<run-root>/production/<scenario>/turn-N/`.

The harness runs independent scenario jobs with `--parallel 4` by default. Use
`--parallel 1` when serial execution is needed for debugging or manual log
comparison.

The harness defaults to `--cache-mode shared`, which prewarms one shared Go
module/build cache under `<run-root>/shared-cache` while keeping databases,
temporary directories, repo copies, and raw logs isolated per job. The eval
database lives inside each copied repo at
`<run-root>/production/<scenario>/repo/openplanner.db` so resumed multi-turn
sessions can use the same local path as single-turn runs. Use
`--cache-mode isolated` for apples-to-apples comparison with older reports.

## Metrics

Reports should include:

- database verification and runner-answer verification
- configured harness parallelism and elapsed harness wall time
- cache mode, cache prewarm time, effective parallel speedup, and parallel
  efficiency
- per-job phase timing totals for setup, cache warm, agent run, metrics parsing,
  and verification
- per-turn metrics and raw log references for multi-turn scenarios
- tool calls, assistant calls, wall time, non-cache input tokens, and output
  tokens when the harness is running real agent sessions
- generated-file inspection
- generated paths surfaced from broad search
- broad repo search
- Go module-cache inspection
- human-facing CLI usage
- direct SQLite access
- configured parallelism and actual harness elapsed seconds

The production path is expected to call:

```bash
go run ./cmd/openplanner-agentops planning
```

Routine production runs should not inspect generated files, module caches, or
SQLite directly, and should not use human-facing command discovery.

Production AgentOps passes only when:

- production passes every selected scenario
- rule-covered invalid-input scenarios are final-answer-only: no tools, no
  command executions, and at most one assistant answer
- production has no direct generated-file inspection, generated-path broad
  search, module-cache inspection, direct SQLite access, or CLI usage
- production has no routine broad repo search
- aggregate command/tool counts and non-cached input token totals are reported

OpenPlanner currently has no human CLI baseline variant, so CLI comparison gates
are intentionally `n/a` instead of mirrored from OpenHealth.

## Current Reports

Current reduced reports belong under `docs/agent-eval-results/`. Historical
iteration artifacts should be archived under `docs/agent-eval-results/archive/`.
The current eval-maturity and throughput reports are:

- `docs/agent-eval-results/op-agentops-maturity-throughput-validation-smoke.md`
- `docs/agent-eval-results/op-agentops-maturity-throughput-expansion-smoke.md`
- `docs/agent-eval-results/op-agentops-maturity-throughput-speed-isolated.md`
- `docs/agent-eval-results/op-agentops-maturity-throughput-speed-shared.md`
- `docs/agent-eval-results/op-agentops-maturity-throughput-final.md`
