# Maintainer Notes

This repository uses **Beads** (`bd`) in embedded mode for maintainer task tracking.
The embedded Dolt database is authoritative; `.beads/issues.jsonl` is only a local JSONL export/backup and must stay untracked.

This repository is public and its release surface is an installed JSON runner,
an Agent Skills-compatible skill, and release integrity metadata. There is still
no hosted service, no auth-backed product surface, no background daemon, and no
package registry. Keep maintainer docs honest about that status.

## Product surface decision

The earlier library-first research direction is superseded for v1 by the
JSON-runner-first direction. Roadmap and architecture work under `op-2vv` should
preserve the installed `openplanner planning` JSON runner and portable
`skills/openplanner` payload as the agent-facing contract.

Internal Go packages may be refactored to support the runner, but they should
not be documented as public APIs or compatibility surfaces. Do not introduce
public package/API compatibility promises, deploy workflows, ports, OpenAPI
contracts, or UI docs without a new issue that explicitly approves that product
surface.

## SQLite migrations

SQLite schema changes live in `internal/store/sqlite.go` as ordered embedded
migrations. The current bootstrap schema is migration version `1`, recorded in
`schema_migrations(version INTEGER PRIMARY KEY)`.

When changing the local data model:

- Append a new migration with the next integer version; do not renumber or edit
  merged migration SQL except for documented compatibility fixes.
- Keep `store.Open` as the schema entrypoint so the JSON runner upgrades local
  databases before any read or write operation.
- Add store tests for both fresh databases and upgrades from the previous schema
  shape, including preservation of existing calendar, event, task, and related
  rows when relevant.
- Reject databases with migration versions newer than the current binary knows
  instead of attempting a downgrade.

## Initial Setup

Preferred tool install:

```bash
mise install
```

Alternative:

```bash
brew install beads dolt
```

## Clone Bootstrap

For a fresh maintainer clone or a second machine:

```bash
git clone git@github.com:yazanabuashour/openplanner.git
cd openplanner
bd bootstrap
bd hooks install
```

If role detection warns in a maintainer clone, set:

```bash
git config beads.role maintainer
```

## Sync Between Machines

Push local Beads state before switching machines, then pull on the other machine:

```bash
bd dolt push
bd dolt pull
```

If `bd dolt pull` reports uncommitted Dolt changes, commit them first and retry:

```bash
bd dolt commit
bd dolt pull
```

## Public repo expectations

- Outside contributors must be able to contribute without Beads.
- Policy and workflow files are part of the public contract and should stay reviewable in Git alone.
- Do not document machine-absolute filesystem paths in committed docs.
- Do not assume private infrastructure, deploy secrets, or internal services exist unless they have been added explicitly.

## Repository administration

Current readiness assumptions:

- `main` is the protected default branch.
- Pull requests run only untrusted-safe validation with read-only token scope.
- GitHub Releases are created from version tags in the `v0.y.z` form.
- The distributable surface is platform archives for the `openplanner` runner,
  the portable skill archive, and release integrity assets.
- Release packaging publishes binary archives, a skill archive, deterministic
  source archive, `SHA256SUMS`, SPDX SBOM, and GitHub attestations.
- The runtime remains in process. Do not add deploy workflows, ports, or daemons unless the product surface changes intentionally.
- Security reports are expected through GitHub private vulnerability reporting.

Current review enforcement nuance:

- The repository currently has a single maintainer account.
- `main` requires pull requests, status checks, conversation resolution, and one approving review, but code-owner review enforcement and admin enforcement remain off so the repository does not become unmergeable.
- Tighten code-owner review enforcement, admin bypass, and maintainer isolation once a second maintainer can satisfy the review requirement.

When changing GitHub settings, keep the repo aligned with:

- [SECURITY.md](../SECURITY.md) for disclosure handling and patch timing.
- [.github/CODEOWNERS](../.github/CODEOWNERS) for sensitive file ownership.
- [.github/workflows/pull-request.yml](../.github/workflows/pull-request.yml) for fork-safe checks.
- [.github/workflows/release.yml](../.github/workflows/release.yml) for release verification, packaging, attestations, and publication.

## Release notes

The release workflow has two paths:

- `workflow_dispatch` packages and attests a snapshot from a chosen ref, then uploads workflow artifacts for manual inspection.
- Pushing a `v0.y.z` tag runs the same verification and packaging steps, then
  publishes a GitHub Release and uploads runner archives, the skill archive, the
  source archive, `SHA256SUMS`, and SBOM.

The first public tag should be `v0.1.0`. Users consume OpenPlanner through the
runner archive or installer plus the matching portable skill payload.

Before tagging:

1. Run the release workflow in `workflow_dispatch` mode against the intended ref.
2. Inspect the uploaded runner archives, skill archive, source archive,
   `SHA256SUMS`, and SBOM.
3. Confirm the release assets match [docs/release-verification.md](../docs/release-verification.md).
4. Tag the release only after manual review is complete.

The publish job is the only workflow path that needs `contents: write` and the `release` environment. Do not widen those permissions to pull requests or non-release jobs unless the product surface changes.
