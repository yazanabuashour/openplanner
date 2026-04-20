# Agent Evaluation Protocol

OpenPlanner agent evals measure the same production skill a real agent receives.
Do not add hidden evaluator-only instructions to improve a result; if an
instruction is needed, put it in the production skill first.

## Active Surface

- `production`: the installed `skills/openplanner/SKILL.md` skill plus an
  installed `openplanner planning` JSON runner on `PATH`.

OpenPlanner follows the OpenHealth AgentOps runner pattern for production evals:
evaluate the same installed runner and production skill that real agents use,
not a source-checkout helper, public SDK, REST API, hosted service, or web UI.

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
before installing the production skill and a private `openplanner` runner binary.

Single-turn scenarios use `codex exec --ephemeral`. Multi-turn scenarios use one
persisted eval session per scenario: the first turn creates a session in the
throwaway run directory context, and later turns use `codex exec resume` with
explicit writable roots for the scenario run directory and shared Go cache.

The harness runs independent scenario jobs with `--parallel 4` by default. Use
`--parallel 1` when serial execution is needed for debugging or manual log
comparison.

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
- stale removed-interface path inspection
- broad repo search
- Go module-cache inspection
- source-checkout runner usage instead of the installed `openplanner` binary
- direct SQLite access

The production path is expected to call:

```bash
openplanner planning
```

Routine production runs should not inspect source files, module caches, or
SQLite directly, and should not use source-checkout command discovery.

Production passes only when:

- production passes every selected scenario
- rule-covered invalid-input scenarios are final-answer-only: no tools, no
  command executions, and at most one assistant answer
- production has no stale removed-interface inspection, module-cache inspection,
  direct SQLite access, source-checkout runner usage, or routine broad repo
  search
- aggregate command/tool counts and non-cached input token totals are reported

OpenPlanner currently has no human CLI baseline variant, so CLI comparison gates
are intentionally `n/a` unless a separate baseline is approved.

## Current Reports

Current reduced reports belong under `docs/agent-eval-results/`. Historical
iteration artifacts should be archived under `docs/agent-eval-results/archive/`.
The current eval-maturity and throughput reports predate the installed-runner
rename and remain as historical evidence until the next eval run updates them.
