# motley

<p align="center">
  <img src="motley-logo.png" alt="Motley logo: a crew of robot musicians connected to a terminal" width="320">
</p>

Run Claude Code, Codex and OpenCode in parallel, each in its own Git worktree and
tmux session. Use **`motley`** or **`mtly`**.

An agent session is a **member**, a group of members is a **crew**, and its package
of work is a **gig**.

## Install

macOS or Linux, amd64 or arm64; bash, zsh or fish:

```sh
curl -fsSL https://raw.githubusercontent.com/thomashartm/motley/main/install.sh -o motley-install.sh && bash motley-install.sh
```

The installer builds from `main`, installs both commands in `~/.local/bin`, and
sets up PATH, the tmux popup and Claude hooks. It installs missing Git, Go and
tmux through your package manager; macOS requires Homebrew. Existing tmux must
be 3.2+ and existing Go 1.21+.

Install and authenticate your agent CLI separately. Open a new terminal and run
`mtly`. Restart existing Claude sessions to load hooks. Rerun the installer to
upgrade.

From a checkout:

```sh
bash install.sh --local  # install local source
make build              # build only; run ./bin/mtly
```

## Start working

Keep main repositories under `~/projects`; worktrees go under `~/worktrees`.
Repositories need an `origin` remote and a local `main` or `master` branch.

Run `mtly`, press **s**, choose a repository and agent, then review and launch.
**Launching creates and pushes a new branch.** Your main checkout stays intact.
Press **Enter** to attach; **Ctrl-b d** detaches without stopping the session.

Or use the CLI:

```sh
mtly spawn --repo api --branch feat/412-fx-cache --ticket 412 --name "FX cache" --agent codex
mtly ls
mtly attach 412-fx-cache
```

`--repo` names a directory under the repository root. Add `--detach` to launch in
the background. Claude is the default agent. Local `.env`, `.env.*` and
`graphify-out` artifacts are copied into the worktree.

## Overview

| Key | Action |
| --- | --- |
| ↑/↓ or j/k | Select a member |
| Enter | Attach or switch to it |
| s | Spawn a member |
| i | Send a reply |
| t | Send the member to another work tab |
| / | Filter members |
| g | Group by attention, crew or repository |
| e | Edit member details |
| G | Manage crews |
| x / r | Retire / revive |
| Page Up / Page Down | Scroll details |
| q | Close |

In crew view, **Tab** enters the member table; **Esc** returns. **→/←** expands
or collapses a crew; **H** shows inactive crews. Detail editors use **Tab** to move,
**Ctrl-s** to save and **Esc** to cancel.

**Ctrl-b h** opens the popup. For a persistent overview in a separate tab, run
`mtly monitor`: **Enter** switches another work tab, **T** chooses that tab, and
**q** detaches the monitor. After upgrading, restart it with
`tmux kill-session -t _motley`, then `mtly monitor`.

Claude hooks report working, permission, question and ready states. Jump into
the agent for permission requests. Codex and OpenCode currently show session
availability only. To install Claude hooks manually, run
`mtly hooks install claude` and restart Claude.

## Crews and gigs

```sh
mtly crew add --title "Banking" --gig "Ship FX caching" --color blue
mtly crew assign 412-fx-cache banking
mtly crew list
mtly crew edit banking --gig "Roll out payments"
```

Use `--crew banking` when spawning or adopting. Members inherit their crew's
colour unless overridden. Crews support an optional `--url`; `--gig ""` clears
the gig. Use `crew assign <member> none` to unassign a member.

## Finish or resume

```sh
mtly retire 412-fx-cache               # remove session, worktree and local branch
mtly retire 412-fx-cache --keep-branch # retain the local branch
mtly revive 412-fx-cache               # restart a dead session, then attach
```

Retire from another session or the monitor. It refuses unsaved or unpushed work;
`--force` discards that work. Remote branches remain. Revive requires the
worktree to exist and cannot restore retired members. Claude resumes its last
recorded session; other agents start fresh. If the agent exited but its tmux
shell is still alive, restart the agent in that shell instead.

To adopt an existing linked worktree, run inside its tmux session:

```sh
mtly adopt --agent claude --name "FX cache"
```

Follow the printed environment and restart instructions. Adoption manages the
whole tmux session, including its other panes.

## Blueprints

Blueprints supply initial prompts. Put Markdown templates in
`~/.config/motley/blueprints/` or the main repository's `.motley/blueprints/`.
Repository templates take precedence. Start with
[feature-plan-first.md](examples/blueprints/feature-plan-first.md).

```sh
mtly blueprint list --repo api
mtly blueprint validate --repo api
mtly spawn --repo api --branch feat/example --blueprint feature-plan-first --var 'constraints=Keep it small'
```

The spawn form also lets you choose a blueprint and edit its prompt in `$EDITOR`.
Revive keeps agent arguments without replaying the initial prompt.

## Configuration

Edit `~/.config/motley/config.toml`:

```toml
schema = 1
repos_root = "~/projects"
worktrees_root = "~/worktrees"
```

`mtly config` shows the roots. State lives in `~/.local/state/motley`.
`XDG_CONFIG_HOME` and `XDG_STATE_HOME` override these locations. For an earlier
pre-v1 setup, copy configuration and blueprints into the new directories and
adopt existing sessions. Manual builds can use `mtly init` to create config and
print the tmux popup setup instructions.

## Development

Requires Go 1.22+, Git, tmux, cp and bash; golangci-lint 2.14.0 and GoReleaser
2.18.2 for the full checks.

```sh
make test     # installer and Go tests
make check    # tests, vet, lint, macOS/Linux builds for amd64/arm64
make snapshot # release archives in dist/; no publishing
```

Tests use temporary repositories, fake agents and isolated tmux servers.
Set `GOLANGCI_LINT` or `GORELEASER` to use tools outside PATH.
Delivery status is tracked in [DELIVERY.md](DELIVERY.md).
