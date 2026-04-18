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
- bounded agenda listing with chronological output
- event and task listing with limits and calendar filters
- task completion for non-recurring and recurring occurrences
- invalid input rejection for ambiguous short dates, year-first slash dates,
  invalid RFC3339 values, invalid ranges, unsupported recurrence, missing
  titles, and non-positive limits

Every scenario should use a fresh isolated repo copy, a fresh local database
path, and reduced JSON/Markdown artifacts. Raw logs are not committed; reduced
reports refer to them with `<run-root>` placeholders. The copied repo omits root
`AGENTS.md`, stale `.agents` content, eval docs, reports, and harness code
before installing the selected production skill so repo-level maintainer
instructions do not contaminate user-data tasks.

## Metrics

Reports should include:

- database verification and runner-answer verification
- command/tool counts, wall time, non-cache input tokens, and output tokens when
  the harness is running real agent sessions
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

## Current Reports

Current reduced reports belong under `docs/agent-eval-results/`. Historical
iteration artifacts should be archived under `docs/agent-eval-results/archive/`.
