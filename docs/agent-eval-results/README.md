# OpenPlanner Agent Eval Results

Current reduced reports for the production AgentOps runner belong in this
directory. Raw logs are not committed. Reduced reports should refer to raw logs
with `<run-root>` placeholders and use repo-relative artifact paths.

Current recommendation:

- Use the production AgentOps JSON runner for routine local planning tasks.
- Keep the SDK as the Go developer surface.
- Treat generated client files as API-contract substrate, not routine agent
  instructions.

Historical iteration artifacts should move under
`docs/agent-eval-results/archive/`.
