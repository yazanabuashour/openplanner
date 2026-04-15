# openplanner

`openplanner` is now bootstrapped as a Go module with a minimal CLI entrypoint and CI checks. The repository is still early-stage scaffolding and does not publish runnable artifacts yet.

## Repository contents

- [CONTRIBUTING.md](CONTRIBUTING.md) explains how outside contributors should propose changes.
- [SECURITY.md](SECURITY.md) explains how to report vulnerabilities privately and what response timing to expect.
- [docs/maintainers.md](docs/maintainers.md) documents Beads-based maintainer workflow and repo administration notes.
- [cmd/openplanner](cmd/openplanner) contains the initial CLI entrypoint.
- [internal/app](internal/app) contains the first internal package and unit tests.
- [LICENSE](LICENSE) defines the project license.

## Release contract

The initial release surface is GitHub Releases with semantic version tags in the `0.y.z` range. Release notes are generated from protected tags. This repository does not currently publish packages or downloadable build artifacts.

## Development

Install the pinned toolchain with:

```bash
mise install
```

Run the CLI with:

```bash
go run ./cmd/openplanner
```

Run the local quality gates with:

```bash
make check
```

## Contributing

Outside contributors can work entirely through GitHub issues and pull requests. Beads is maintainer-only workflow tooling and is not required for community contributions.

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution expectations and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for community standards.
