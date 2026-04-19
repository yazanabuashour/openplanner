# Release Verification

Tagged OpenPlanner releases publish integrity-focused assets alongside the Git
tag:

- `openplanner_<version>_<os>_<arch>.tar.gz`
- `openplanner_<version>_skill.tar.gz`
- `openplanner_<version>_source.tar.gz`
- `openplanner_<version>_checksums.txt`
- `openplanner_<version>_sbom.spdx.json`
- `install.sh`

The platform archives contain the production `openplanner` runner binary. The
skill archive contains the single shipped `SKILL.md` payload. The source archive
is the canonical source artifact for the local runtime. The
installer script downloads and verifies the matching platform archive before
installing the same-tag runner. It then prints the required second step:
register the same-tag skill source or archive with the user's agent using that
agent's native skill system. The checksums file and GitHub attestations let
users verify that the assets were produced by this repository's release
workflow.

## Verify a release

Download the assets from the GitHub Release page for the tag you want to verify,
then run:

```bash
shasum -a 256 -c openplanner_<version>_checksums.txt
gh attestation verify openplanner_<version>_<os>_<arch>.tar.gz --repo yazanabuashour/openplanner
gh attestation verify openplanner_<version>_skill.tar.gz --repo yazanabuashour/openplanner
gh attestation verify openplanner_<version>_source.tar.gz --repo yazanabuashour/openplanner
gh attestation verify install.sh --repo yazanabuashour/openplanner
```

If these commands succeed, the assets match the published checksums and have
valid GitHub attestations for this repository.

## Verify the runner story

Put the matching platform archive's `openplanner` binary on `PATH`, then run a
validation request:

```bash
printf '%s\n' '{"action":"validate"}' | openplanner planning
```

The command should print JSON with `rejected` set to `false` and `summary` set
to `valid`.

The release installer performs the same runner validation:

```bash
curl -fsSL https://github.com/yazanabuashour/openplanner/releases/latest/download/install.sh | sh
```

## Verify the skill archive

The skill archive contains the portable Agent Skills folder payload. Install or
copy it using the target agent's own skill installation workflow, then ask that
agent to use OpenPlanner for a local calendar or task request.

## SBOM

The SPDX JSON SBOM asset is intended for audit tooling and manual inspection:

```bash
jq '.packages | length' openplanner_<version>_sbom.spdx.json
```

The SBOM is generated from the tagged source contents during the release
workflow and attached to the same GitHub Release as the binary, skill, and
source archives.
