# colony

<p align="center">
  <img src="colony-logo.png" alt="colony logo" width="320">
</p>

colony is a terminal tool for parallel AI coding sessions.

Run `colony` to display the configured repository and worktree paths,
`colony version` to print the version, or `colony --help` for available commands.

## Build and run

Supports macOS and Linux on amd64 and arm64, with cgo disabled. Building requires
Go 1.22 or newer on Linux; use Go 1.25.5 or newer on current macOS.

```sh
make build
./bin/colony
./bin/colony version
./bin/colony --help
```

For a local install, run `go install ./cmd/colony` with your Go bin directory on
PATH. Source builds report `dev`; snapshot binaries include the Git commit.

## Configuration

Reads `${XDG_CONFIG_HOME:-~/.config}/colony/config.toml`. Missing files/settings
use defaults. File values override defaults. `XDG_CONFIG_HOME` selects the config
directory.

To customize the roots, create that file using [config.example.toml](config.example.toml):

```toml
schema = 1
repos_root = "~/projects"
worktrees_root = "~/worktrees"
```

`~` and `~/` expand to your home directory. Loading config creates no files or
directories. Invalid TOML, incorrect field types and empty roots produce errors.
Unknown keys are ignored; missing keys, including schema, use defaults. Only
schema 1 is supported.

`colony version` and `--help` remain available even with invalid configuration.

## Development checks

Install golangci-lint v2.14.0 and GoReleaser v2.18.2 using their official
[installation instructions](https://golangci-lint.run/docs/welcome/install/) and
[release packages](https://goreleaser.com/install/), then run:

```sh
make check     # tests, vet, lint, four OS/architecture builds
make snapshot # four release archives and checksums in dist/; no publishing
```

`GOLANGCI_LINT` and `GORELEASER` make variables may point to locally installed
tool binaries. GoReleaser needs a Git checkout with at least one commit for build
metadata. The release configuration follows the upstream
[Go builder](https://goreleaser.com/customization/builds/builders/go/) and
[snapshot workflow](https://goreleaser.com/blog/goreleaser-v2/).

CI runs tests and vet on macOS/Linux, checks the Go 1.22 minimum on Linux, lints,
cross-builds darwin/linux × amd64/arm64, and uploads snapshot archives. CI does
not publish releases. The executable smoke test uses temporary HOME/config
directories and verifies defaults, file overrides, help, version and errors.
