# colony

<p align="center">
  <img src="colony-logo.png" alt="colony logo" width="320">
</p>

colony is a terminal tool for parallel AI coding sessions.

Each minion gets its own git worktree, tmux session and coding agent.
Use Claude Code, Codex or OpenCode, and switch between sessions from your terminal.

## Build and run

Supports macOS and Linux on amd64 and arm64, with cgo disabled. Building requires
Go 1.22 or newer on Linux; use Go 1.25.5 or newer on current macOS.
Running minions requires git, tmux ≥ 3.2, the system `cp` command and your chosen
agent CLI on PATH. Install and authenticate the agent before spawning a minion.

```sh
make build
./bin/colony
./bin/colony version
./bin/colony --help
```

For a local install, run `go install ./cmd/colony` with your Go bin directory on
PATH. Source builds report `dev`; snapshot binaries include the Git commit.

## Start a minion

With a main repository at `~/projects/api` and an `origin` remote:

```sh
colony spawn --repo api --branch feat/412-fx-cache --ticket 412 --name "FX cache"
```

This fetches `origin/main` (or `origin/master` when only a local master exists),
creates and pushes the new branch, copies local artifacts, and starts Claude Code
in tmux. It switches your current tmux client, or attaches this terminal when
outside tmux. Detach with tmux's `Ctrl-b d`; the session keeps running.

Choose another agent or leave it running in the background:

```sh
colony spawn --repo api --branch feat/cache-tests --agent codex --detach
colony spawn --repo api --branch feat/cache-docs --agent opencode --detach
colony ls
colony attach 412-fx-cache
colony switch feat-cache-tests
```

`--repo` is an exact directory name under `repos_root`. The branch must be new
locally. Use letters, digits, slashes, dots, underscores and hyphens in branch
names. Worktrees are created under
`<worktrees_root>/<repo>/<branch-with-slashes-replaced-by-hyphens>`.

Minion ids use the ticket and name when supplied, otherwise the branch with
slashes replaced by hyphens. A conflicting id gets a repository prefix; the
spawn output prints the actual id and attach command.

The main checkout's files stay untouched. Local `.env` and `.env.*` files and
`graphify-out` directories are copied recursively, preserving attributes and
existing destination files. Searches exclude `.git` and `node_modules`.

When the agent exits, its pane becomes a shell. `colony ls` reports **alive** while
the tmux session exists and **dead** after it ends; this describes the session,
not whether the agent is currently working. Manifests live in
`${XDG_STATE_HOME:-~/.local/state}/colony/minions/`. If creation fails after a
worktree has been made, colony reports the error and retains that worktree.

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
Running `colony` without arguments displays the configured roots.

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
not publish releases. Tests require git, tmux, cp and bash. They use temporary
HOME/config/state directories, fixture repositories with local bare remotes,
fake agents and separate `tmux -L colony-test-<random>` servers. They never use
your working repositories or active tmux server. Artifact-copy tests compare
against the original wt script and a golden inventory.
