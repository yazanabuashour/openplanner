# OpenPlanner Agent Eval Results

Current reduced reports for the production JSON runner belong in this directory.
Raw logs are not committed. Reduced reports should refer to raw logs with
`<run-root>` placeholders and use repo-relative artifact paths.

Current recommendation:

- Use the installed `openplanner planning` JSON runner for routine local planning tasks.
- Treat the portable `skills/openplanner` payload as the production skill
  contract.
- Keep reduced reports aligned with the OpenHealth AgentOps runner pattern:
  evaluate the installed runner and production skill that real agents use.
- Keep CLI and human-baseline comparisons `n/a` unless a separate baseline is
  approved.

Expanded production reports use the `op-runner-<date>-expansion` naming pattern
and should include update, richer recurrence, migration-style, multi-turn
disambiguation, and future-surface unsupported categories. Filtered smoke
reports may cover fewer categories when the report marks category coverage as
filtered.

Scale eval reports use the `op-2vv.3-<date>-scale` naming pattern. They measure
the JSON runner directly on larger local datasets for agenda generation,
recurring event expansion, recurring task completion lookup, and list
pagination. They should record dataset size, wall time, threshold seconds,
pass/fail status, and any beads blocker issues created for failed local
maintainer thresholds. Raw scale databases and logs stay under `<run-root>` and
are not committed.

Historical iteration artifacts should move under
`docs/agent-eval-results/archive/`.
