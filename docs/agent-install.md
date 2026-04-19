# Agent Install Examples

OpenPlanner's install contract is agent-neutral:

1. Install and verify the `openplanner` runner.
2. Register `skills/openplanner/SKILL.md` using the target agent's native skill
   system.

The examples below are not canonical install paths. They are translation notes
for agents that already document their own skill locations or installers.

## Codex

Codex reads repository, user, admin, and system skills. For a repo-scoped
example, place the portable skill folder at:

```text
<repo>/.agents/skills/openplanner/SKILL.md
```

Use the release installer or platform archive for the runner, then make sure
`openplanner planning` is available on `PATH`.

## Claude Code

Claude Code documents personal, project, enterprise, and plugin skill scopes.
For a project example, place the portable skill folder at:

```text
<project>/.claude/skills/openplanner/SKILL.md
```

Use the release installer or platform archive for the runner, then make sure
`openplanner planning` is available on `PATH`.

## OpenClaw

OpenClaw supports Agent Skills-compatible folders and native skill installation
flows such as workspace skill installs. For a workspace example, place the
portable skill folder at:

```text
<workspace>/skills/openplanner/SKILL.md
```

Use the release installer or platform archive for the runner. If the agent runs
inside a sandbox, make sure the same `openplanner` runner is also available
inside that runtime.

## Hermes

Hermes supports hub, well-known, GitHub, and local skill sources. For a local
example, place the portable skill folder at:

```text
<user-home>/.hermes/skills/openplanner/SKILL.md
```

Use the release installer or platform archive for the runner, then make sure
`openplanner planning` is available to the Hermes agent process.
