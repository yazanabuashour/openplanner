# Release Verification

OpenPlanner ships a source-first release for the embeddable Go SDK and the
machine-facing AgentOps JSON runner. Releases do not install a host service,
open a port, or create background state on the machine by themselves.

The first planned public tag is `v0.1.0`. Until that tag exists, treat the commands below as the intended verification contract for the first release and use a local `replace` or pseudo-version from `main` during development.

## Published assets

Each tagged release publishes:

- a deterministic source archive
- `SHA256SUMS`
- an SPDX SBOM
- GitHub attestations for the source archive and SBOM

## Verify the release assets

Download the tagged release assets from GitHub Releases, then verify the checksum:

```bash
sha256sum --check openplanner-v0.y.z.SHA256SUMS
```

On macOS, run `shasum -a 256 openplanner-v0.y.z-source.tar.gz` and compare the digest with the matching line in `openplanner-v0.y.z.SHA256SUMS`.

Verify the GitHub attestation for the source archive:

```bash
gh attestation verify openplanner-v0.y.z-source.tar.gz --repo yazanabuashour/openplanner
```

## Verify the module install story

Install the tagged SDK package:

```bash
go get github.com/yazanabuashour/openplanner/sdk@v0.1.0
```

Go resolves that package version from the root module tag at `github.com/yazanabuashour/openplanner`.

Then open the SDK locally with the default SQLite path:

```go
client, err := sdk.OpenLocal(sdk.Options{})
```

By default, the SQLite file is created at `${XDG_DATA_HOME:-~/.local/share}/openplanner/openplanner.db`. If a caller sets `DatabasePath`, OpenPlanner uses that path instead.

## Verify the agent runner story

Run a validation request through the production AgentOps runner:

```bash
printf '%s\n' '{"action":"validate"}' | go run ./cmd/openplanner-agentops planning
```

The command should print JSON with `rejected` set to `false` and `summary` set
to `valid`.

## Runtime expectations

- The OpenAPI contract drives the generated client and request/response types.
- The SDK is the Go developer surface.
- The AgentOps runner is the production agent surface.
- The runtime stays in process through the SDK transport layer.
- No host service, daemon, or port binding is required for normal use.
