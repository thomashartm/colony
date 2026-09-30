# colony

<p align="center">
  <img src="colony-logo.png" alt="colony logo" width="320">
</p>

colony is a terminal tool for parallel AI coding sessions. The project is being
built one usable slice at a time; see [REQUIREMENTS.md](REQUIREMENTS.md).

W0 provides the CLI shell, configuration loading and build/release checks. Run
`colony` to inspect the configured roots and `colony version` for build identity.
Session management begins in W1; the TUI begins in W2.

## Build and run

Requires Go 1.22 or newer. Supports macOS and Linux on amd64 and arm64, with cgo
disabled. W0 does not require tmux, git, gh or agent CLIs at runtime.
Use Go 1.25.5 or newer to build on current macOS: the Go 1.22 linker produces
test executables that this machine's macOS loader rejects. CI verifies the
Go 1.22 minimum on Linux.

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
use defaults. File values override defaults; W0 has no per-repo settings, setting
environment overrides or config flags. `XDG_CONFIG_HOME` only selects the config
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
schema 1 is supported. Before v1.0, a documented reset may replace a migration;
the W0 reset is to back up the config and recreate it from the example.

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

## Delivery checkpoints

After each work item, stop for user feedback before starting the next. W0 uses
the literal work-item numbering: its release tag is `v0.0.0`; W1 is `v0.1.0`.
Local validation, hosted CI and a release tag are reported separately, so a local
pass is never presented as a hosted CI result.

The original [wt 1.2.0](reference/wt) and [wt-clean 1.0.0](reference/wt-clean)
remain unchanged references. Continue using those tools until W10.
