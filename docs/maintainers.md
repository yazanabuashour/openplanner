# Maintainer Notes

This repository uses **Beads** (`bd`) in embedded mode for maintainer task tracking.

This repository is public and its release surface is an embeddable Go module plus source-release metadata. There is still no hosted service, no auth-backed product surface, no background daemon, and no package registry beyond the Go module/tag flow. Keep maintainer docs honest about that status.

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
- The distributable surface is the Go module resolved from those tags plus source-only release assets.
- Release packaging stays source-only: deterministic source archive, `SHA256SUMS`, SPDX SBOM, and GitHub attestations.
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
- Pushing a `v0.y.z` tag runs the same verification and packaging steps, then publishes a GitHub Release and uploads the source archive, `SHA256SUMS`, and SBOM.

Users consume the SDK with `go get github.com/yazanabuashour/openplanner@v0.y.z`.

Before tagging:

1. Run the release workflow in `workflow_dispatch` mode against the intended ref.
2. Inspect the uploaded source archive, `SHA256SUMS`, and SBOM.
3. Confirm the release assets match [docs/release-verification.md](../docs/release-verification.md).
4. Tag the release only after manual review is complete.

The publish job is the only workflow path that needs `contents: write` and the `release` environment. Do not widen those permissions to pull requests or non-release jobs unless the product surface changes.
