# Release Verification

OpenPlanner ships a source-first release for the embeddable Go SDK. Releases do not install a host service, open a port, or create background state on the machine by themselves.

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

Install the tagged module:

```bash
go get github.com/yazanabuashour/openplanner@v0.y.z
```

Then open the SDK locally with the default SQLite path:

```go
client, err := sdk.OpenLocal(sdk.Options{})
```

By default, the SQLite file is created at `${XDG_DATA_HOME:-~/.local/share}/openplanner/openplanner.db`. If a caller sets `DatabasePath`, OpenPlanner uses that path instead.

## Runtime expectations

- The OpenAPI contract drives the generated client and request/response types.
- The runtime stays in process through the SDK transport layer.
- No host service, daemon, or port binding is required for normal use.
