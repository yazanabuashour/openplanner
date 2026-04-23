# Security Operations

This runbook defines recurring security work for OpenPlanner maintainers. It complements the public reporting and response policy in [SECURITY.md](../SECURITY.md); do not put private vulnerability details in public issues, pull requests, release notes, or this document.

## Cadence

- Weekly: triage Dependabot pull requests, dependency-review failures, and new vulnerability alerts for Go modules and GitHub Actions.
- Monthly: review the GitHub Security tab, private vulnerability reporting state, Dependabot alert backlog, and any deferred security issues in Beads.
- Quarterly: rehearse the advisory workflow, refresh the local planning data threat model, and confirm that release, automation, and maintainer-isolation assumptions still match the repository.
- Release-bound: review security impact before tagging any release that changes `.github/workflows/release.yml`, `scripts/install.sh`, `skills/openplanner/SKILL.md`, SQLite migrations, runner write/update/delete behavior, iCalendar import/export, CalDAV behavior, or release verification docs.

## High-Risk Surfaces

- Local SQLite calendar and task data in user-selected database paths and the default OpenPlanner data directory.
- Runner JSON operations in `cmd/openplanner`, `internal/runner`, `internal/service`, and `internal/store`, especially write, update, delete, import, export, completion, and reminder behavior.
- iCalendar import/export parsing and normalization in `internal/icalendar`, including recurrence, attendees, reminders, task status, priority, tags, UID-based repeat imports, and provider-shaped fixtures.
- The experimental CalDAV adapter in `internal/caldav`, especially local exposure, loopback enforcement, request body limits, XML parsing, resource resolution, `PUT`, and `DELETE`.
- Agent-facing task policy in `skills/openplanner/SKILL.md`, including direct-reject rules, date/time validation, recurrence limits, delete guidance, import/export guidance, and direct SQLite prohibitions.
- Install and release pipeline files: `scripts/install.sh`, `.github/workflows/release.yml`, `docs/release-verification.md`, `CHANGELOG.md`, and `docs/release-notes`.
- GitHub Actions and repository policy files under `.github`, including token permissions, environment protection, CODEOWNERS, dependency review, and branch protection assumptions.
- Contributor pull request paths, especially any workflow that runs code from untrusted forks or exposes repository secrets.

## Review Workflow

1. Open or update a Beads issue for any recurring security review that finds follow-up work.
2. Classify findings using the severity expectations in `SECURITY.md`.
3. Keep exploit details private until a fix or mitigation is available.
4. For dependency updates, prefer the smallest reviewable update that clears the alert and keeps `mise exec -- make check` passing.
5. For workflow or release-pipeline changes, verify token permissions remain job-scoped and no release, deployment, package, or private infrastructure write permission is granted to untrusted pull request execution.
6. For skill or runner policy changes, confirm the public docs, skill contract, tests, and release notes remain aligned.

## Deeper Testing Expectations

- Runner write/update/delete behavior should have focused validation, idempotency, and rejection tests before release.
- SQLite migrations should include fresh-database and upgrade tests, including preservation of existing calendar, event, task, reminder, link, recurrence, and completion rows when relevant.
- iCalendar changes should include malformed input, oversized input, provider fixture, repeat-import, recurrence, attendee, reminder, and unsupported-component tests.
- CalDAV changes should include discovery, `PROPFIND`, `REPORT`, `calendar-multiget`, `GET`, `HEAD`, `PUT`, `DELETE`, ETag, cross-calendar href, content-type, loopback binding, and body-limit tests.
- Skill policy changes should run `./scripts/validate-agent-skill.sh skills/openplanner` and the relevant OpenPlanner agent eval gate before release.
- Release-pipeline changes should run `./scripts/validate-release-docs.sh <tag>` for the target tag and verify the expected release asset, checksum, SBOM, and attestation behavior.
- Add fuzzing or property-style tests when parsing, normalization, target resolution, or import logic becomes complex enough that table tests no longer cover realistic malformed input.
- Abuse-case tests should be added before introducing remote APIs, hosted services, secrets-backed integrations, self-hosted runners, or broad automation write privileges.

## Advisory Rehearsal

At least quarterly, maintainers should rehearse the private advisory flow without publishing a real advisory:

- Confirm GitHub private vulnerability reporting is enabled and reachable from the repository Security tab.
- Confirm the private fix path, release notes redaction approach, patch-tag process, and release verification steps are still documented.
- Confirm emergency release expectations still match the current artifact set: binary archives, skill archive, installer, checksums, SBOM, and attestations.
- File Beads issues for any gap found during the rehearsal.
