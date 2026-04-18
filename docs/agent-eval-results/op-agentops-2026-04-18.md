# OpenPlanner AgentOps Eval 2026-04-18

- Harness: `openplanner-agentops-runner`
- Parallelism: `4`
- Harness elapsed seconds: `5.00`
- Raw logs committed: `false`
- Raw logs note: Raw logs are stored outside the repo and referenced with <run-root> placeholders.

| Scenario | Passed | Commands | Wall Seconds | Details |
| --- | ---: | ---: | ---: | --- |
| `ensure-calendar` | true | 2 | 3.70 | expected created then already_exists |
| `create-event` | true | 1 | 3.37 | expected one created event |
| `create-task` | true | 1 | 3.01 | expected one dated task |
| `agenda-range` | true | 3 | 4.12 | expected date task then timed event |
| `complete-task` | true | 2 | 1.99 | expected completed task write |
| `invalid-date` | true | 1 | 1.28 | expected date validation rejection |
