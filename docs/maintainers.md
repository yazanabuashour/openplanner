# Maintainer Notes

This repository uses **Beads** (`bd`) in embedded mode for maintainer task tracking.

This repository is public and now ships a tagged Go module as its first distributable artifact. There is still no hosted service, no auth-backed product surface, and no package registry beyond the Go module/tag flow. Keep maintainer docs honest about that status.

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
- The first distributable surface is the Go module resolved from those tags.
- Security reports are expected through GitHub private vulnerability reporting.

Current review enforcement nuance:

- The repository currently has a single maintainer account.
- `main` requires pull requests, status checks, conversation resolution, and one approving review, but code-owner review enforcement and admin enforcement remain off so the repository does not become unmergeable.
- Tighten code-owner review enforcement, admin bypass, and maintainer isolation once a second maintainer can satisfy the review requirement.

When changing GitHub settings, keep the repo aligned with:

- [SECURITY.md](../SECURITY.md) for disclosure handling and patch timing.
- [.github/CODEOWNERS](../.github/CODEOWNERS) for sensitive file ownership.
- [.github/workflows/pull-request.yml](../.github/workflows/pull-request.yml) for fork-safe checks.
- [.github/workflows/release.yml](../.github/workflows/release.yml) for release-note generation.

## Release notes

The current release contract is a tagged Go module plus GitHub Releases notes. Tag a version like `v0.1.0`, push the tag, and let the release workflow generate notes from the tag. Users consume the SDK with `go get github.com/yazanabuashour/openplanner@v0.1.0`.

Do not attach build artifacts, checksums, provenance, or SBOMs until the stronger release process in the artifact-hardening issues is implemented.
