# Agent Evaluation Plan

This project evaluates agent behavior against the same surface a real
OpenPlanner agent receives. The production eval must use only the installed
`skills/openplanner` payload and a fresh session. Do not add hidden evaluator
instructions that tell the agent which API to call unless those instructions are
also present in the production skill.

## Primary Production Eval

- Start from a fresh agent session with the production `openplanner` skill.
- Provide natural user prompts such as `add standup tomorrow at 9 for an hour`
  or `show my agenda for next week`.
- Use the normal local Go/tool environment and default OpenPlanner data path.
- Judge success by final database state, agenda verification, duplicate calendar
  behavior, tool calls, assistant calls, wall time, non-cache input tokens, and
  whether the agent read generated files or the Go module cache.
- The expected production path is `sdk.OpenLocal(...)` plus the ergonomic
  planning helpers on `sdk.Client`.

## Isolated Variants

Keep comparison variants outside the production skill so the real skill stays
opinionated and narrow.

- Baseline A: current or archived generated-client skill surface.
- Variant B: production code-first SDK skill surface.
- Variant C: CLI-oriented harness or alternate skill payload, if a CLI is added.

Each variant should have its own skill payload or harness instructions. Do not
combine generated-client, SDK, and CLI recipes in the same production skill.

## Core Scenarios

- Create a calendar from a natural-language prompt and repeat the request
  without creating a duplicate calendar.
- Add timed and all-day events, then verify them through an agenda range.
- Add dated and timed tasks, including at least one recurring task.
- Complete a non-recurring task and a specific recurring task occurrence.
- List a bounded agenda range and verify chronological output.
- Reject or clarify ambiguous short dates when the year cannot be inferred.
- Avoid generated client files and module cache reads for routine workflows.
