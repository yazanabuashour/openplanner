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

Historical iteration artifacts should move under
`docs/agent-eval-results/archive/`.
