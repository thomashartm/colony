# motley

motley is a terminal tool for parallel AI coding sessions.

Each member gets its own git worktree, tmux session and coding agent.
Use Claude Code, Codex or OpenCode, and switch between sessions from your terminal.

Run it as **`motley`** or **`mtly`**. A running agent session is a **member**,
a group of members is a **crew**, and a package of work is a **gig**.

## Install

Download, build and install the latest source from `main` with one command:

```sh
wget -qO motley-install.sh https://raw.githubusercontent.com/thomashartm/motley/main/install.sh && bash motley-install.sh
```

On macOS, where `curl` is included:

```sh
curl -fsSL https://raw.githubusercontent.com/thomashartm/motley/main/install.sh -o motley-install.sh && bash motley-install.sh
```

The installer supports macOS/Linux on amd64/arm64 and bash, zsh or fish. It:

- Installs missing Git, Go and tmux using Homebrew, apt, dnf or pacman. Homebrew
  must already be installed on macOS; Linux package installation may ask for sudo.
- Builds with Go 1.25.5, [downloaded automatically by Go](https://go.dev/doc/toolchain) when needed (requires
  an existing or package-installed Go ≥ 1.21), and installs to `~/.local/bin/motley`
  with `~/.local/bin/mtly` as its short command.
- Adds PATH to your shell startup files, creates default motley configuration,
  and enables the tmux popup. Existing settings are preserved; modified shell
  and tmux files get a `.motley-backup.*` copy.
- Installs Claude reporting hooks if `claude` is already on PATH.

**Open a new terminal, then run `motley` or `mtly`.** No manual PATH or configuration edits
are needed. In the current terminal you can immediately run `~/.local/bin/motley`.
Rerun the same command to upgrade. Install and authenticate your chosen agent
CLI separately; restart existing Claude sessions after hook installation.
Existing tmux must be version 3.2 or newer.

Motley uses `${XDG_CONFIG_HOME:-~/.config}/motley` for configuration and
`${XDG_STATE_HOME:-~/.local/state}/motley` for state. When moving from an earlier
pre-v1 setup, run the installer and copy any configuration or blueprints you want
to retain into these directories. Existing sessions are not imported
automatically; use `motley adopt` from their linked worktrees, then follow its
printed environment and agent-restart instructions.

### Install from a checkout

To install your local code, including changes not yet published:

```sh
bash install.sh --local
```

To build without installing or changing your configuration:

```sh
make build
./bin/motley
```

## Try it

First check installation and open the overview:

```sh
motley version
motley config
motley ls
motley
```

An empty member list is normal on first use. Press **q** to close the overview.
By default, your main Git repositories go under `~/projects` and motley creates
worktrees under `~/worktrees`.

To test a real agent session:

1. Have a repository under `~/projects`, with an `origin` remote and a local
   `main` or `master` branch. Authenticate your agent CLI before continuing.
2. Run `motley`, press **s**, select the repository and enter a name such as
   `Motley test`. Choose your installed agent and **none** for the blueprint.
3. Review the branch and launch. **This creates and pushes a new branch to origin.**
   The main checkout is preserved.
4. Press **Enter** on the new member. Ask the agent to describe the repository
   without changing files. With Claude hooks installed, another terminal running
   `motley` should show **working**, then **ready** when the turn finishes.
5. Press **Ctrl-b d** to detach. Run `motley` again; use **/** to filter for the
   test member, **i** to send a follow-up, or **Enter** to return to it.
6. From another terminal, run `motley monitor`. Open the agent in a separate tab
   with `motley attach <id>` (use the id from `motley ls`). The monitor's Enter
   key switches that work tab while leaving the monitor visible. **Ctrl-b h**
   opens the popup in a tmux work tab.
7. Finish from outside the member with `motley retire <id>`, or **x** in the
   overview. Retirement refuses unsaved or unpushed work. It removes the local
   worktree and branch; the remote test branch remains for you to delete.

For automated checks, see [Development checks](#development-checks).

## Start a member

With a main repository at `~/projects/api` and an `origin` remote:

```sh
motley spawn --repo api --branch feat/412-fx-cache --ticket 412 --name "FX cache"
```

This fetches `origin/main` (or `origin/master` when only a local master exists),
creates and pushes the new branch, copies local artifacts, and starts Claude Code
in tmux. It switches your current tmux client, or attaches this terminal when
outside tmux. Detach with tmux's `Ctrl-b d`; the session keeps running.

Choose another agent or leave it running in the background:

```sh
motley spawn --repo api --branch feat/cache-tests --agent codex --detach
motley spawn --repo api --branch feat/cache-docs --agent opencode --detach
motley ls
motley attach 412-fx-cache
motley switch feat-cache-tests
```

`--repo` is an exact directory name under `repos_root`. The branch must be new
locally. Use letters, digits, slashes, dots, underscores and hyphens in branch
names. Worktrees are created under
`<worktrees_root>/<repo>/<branch-with-slashes-replaced-by-hyphens>`.

Member ids use the ticket and name when supplied, otherwise the branch with
slashes replaced by hyphens. A conflicting id gets a repository prefix; the
spawn output prints the actual id and attach command.

The main checkout's files stay untouched. Local `.env` and `.env.*` files and
`graphify-out` directories are copied recursively, preserving attributes and
existing destination files. Searches exclude `.git` and `node_modules`.

When the agent exits, its pane becomes a shell. `motley ls` shows both the agent status
and whether its tmux session is **alive** or **dead**. After Claude exits, its
status is **ended** while the shell session remains alive. Manifests live in
`${XDG_STATE_HOME:-~/.local/state}/motley/members/`. If creation fails after a
worktree has been made, motley reports the error and retains that worktree.

## Finish, adopt and resume

Retire a member from the monitor, another tmux session, or outside tmux:

```sh
motley retire 412-fx-cache
motley retire 412-fx-cache --keep-branch
```

Retirement refuses uncommitted/untracked files and commits ahead of the local
upstream reference. Without an upstream, it checks commits beyond the base
branch. `--force` explicitly discards that work. Motley kills the tmux session,
removes the worktree, and deletes its local branch unless `--keep-branch` is set
or the branch is `main`, `master` or `develop`. Remote branches are kept.

The manifest, event log and any prompt move to `members/archive/`, with a
retirement timestamp. Reused ids get a timestamp suffix in the archive. If
cleanup fails, the active manifest is retained so you can correct the reported
problem and retry. A main checkout or a worktree whose branch has changed is
refused even with `--force`.

Motley refuses to retire the session running the command or popup: killing that
pane would interrupt cleanup. Use the monitor or another terminal instead.

To bring an existing worktree session into motley, run from that linked
worktree inside tmux:

```sh
motley adopt --agent claude --ticket 412 --name "FX cache"
```

This records its Git state and renames the current tmux session to the member id.
Main checkouts cannot be adopted. Existing processes keep their environment;
run the printed `export MOTLEY_MEMBER=...` command in your current shell and
restart the agent to enable reporting. New panes inherit the id automatically.
Retirement manages the whole adopted tmux session, including its other panes.

To restart a **dead** member whose worktree still exists:

```sh
motley revive 412-fx-cache
motley attach 412-fx-cache
```

Claude resumes the latest session id recorded in its event log. Without a
recorded id, it starts fresh. Codex and OpenCode start fresh. Revive leaves the
new tmux session detached; it does not restore retired members or deleted
worktrees. An agent that exited into a shell still has a live tmux session;
restart it in that shell, or close that session before using revive.

## Crews, gigs and colours

Group members into a crew and describe its gig (package of work):

```sh
mtly crew add --title "FX & Banking" --gig "Ship FX caching" --color blue
motley crew assign 412-fx-cache fx-banking
motley crew list
motley crew edit fx-banking --title "Banking" --color purple
motley crew edit fx-banking --gig "Roll out payments"
```

A crew can have an optional `--gig` description and `--url` link. Clear its gig
with `crew edit <id> --gig ""`. Gigs appear in crew lists and TUI details.
Titles are entered manually; motley
makes no network request. New ids use the title's slug, with a numeric suffix
when needed. Without `--color`, a crew takes the next unused palette colour.

Use `--crew <id>` on spawn or adopt. A member inherits its crew's colour unless
it has a `--color` override; unassigned members get a stable colour from their id.
The palette is **red, orange, yellow, green, blue, purple, brown, grey**.
The overview uses a colour bar for identity and separate status icons. Agent
badges are **CC** (orange), **CX** (green), and **OC** (purple).

Crew edits immediately update live members' tmux status bars and emoji titles,
while preserving individual colour overrides. In the overview, **e** edits a
member's name, ticket, crew and colour. Clear its colour to restore inheritance.
For older sessions, assigning a crew or saving an edit also applies the styling.

```sh
motley crew assign 412-fx-cache none
motley crew rm fx-banking
motley crew rm fx-banking --force
motley crew list --json
```

Removal refuses crews referenced by live or dead members. `--force` unassigns
those members; their worktrees and sessions remain. Archived manifests retain
their historical crew id. Crews are stored in
`${XDG_STATE_HOME:-~/.local/state}/motley/crews.toml`.

## Blueprints

Blueprints are Markdown templates for a member's initial prompt. Store them in
`${XDG_CONFIG_HOME:-~/.config}/motley/blueprints/`, or in the main repository's
`.motley/blueprints/` directory. A repository blueprint overrides a global one
with the same name. An optional `repos` list restricts where it is available.

Start with [feature-plan-first.md](examples/blueprints/feature-plan-first.md):

```sh
mkdir -p ~/.config/motley/blueprints
cp examples/blueprints/feature-plan-first.md ~/.config/motley/blueprints/
motley blueprint list --repo api
motley blueprint show feature-plan-first --repo api
motley blueprint validate --repo api
motley spawn --repo api --branch feat/412-fx-cache --ticket 412 --name "FX cache" \
  --blueprint feature-plan-first --var 'constraints=Keep the change small'
```

Use your XDG config directory instead of `~/.config` if you have customized it.
Without `--repo`, the authoring commands use global files. `validate [name]`
checks syntax and renders with empty values, so templates should handle absent
optional inputs.

Each file starts with TOML between `+++` lines, followed by a Go text/template
body. Front matter supports `schema = 1`, `name`, `description`, `agent`, `args`,
`repos` and `vars`. Omitted name defaults to the filename without `.md`; omitted
agent defaults to Claude. Unknown front-matter keys are ignored.

Template fields are `Repo`, `Branch`, `Base`, `Ticket`, `Name`, `Worktree`,
`Crew.Title`, `Crew.URL`, `Crew.Kind`, and `Vars.<name>`. Unset fields and missing
variable keys render empty. Repeat `--var key=value` to supply variables; values
can contain commas, equals signs and newlines. The last value for a key wins.
`vars` lists suggested inputs; it does not make them mandatory.

The CLI's `--agent` overrides the blueprint's agent. If the blueprint has arguments
for a different agent, motley refuses the mismatch before creating a worktree.
Blueprint arguments are passed unchanged, so the example's Claude plan mode is
selected by its `args = ["--permission-mode", "plan"]`.

Claude and Codex receive the rendered prompt as one positional argument;
OpenCode uses `--prompt`. Prompts are never interpolated into a shell command.
Rendered prompts are limited to 64 KiB and saved privately beside the manifest
as `<id>.prompt.md`, with a schema comment that is removed before delivery.
The manifest records the blueprint name and agent arguments. **Revive preserves
those arguments without replaying the initial prompt.** Retirement archives the
prompt with the rest of the member's state.

## Configuration

Reads `${XDG_CONFIG_HOME:-~/.config}/motley/config.toml`. Missing files/settings
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

`motley version` and `--help` remain available even with invalid configuration.
`motley config` displays the configured roots.

## Session overview

Run `motley` to browse your members in a terminal overview. The left list shows
**NEEDS YOU** (permission, question, ready, idle), **WORKING**, and **ENDED / DEAD**.
Waiting members appear oldest first, with a status icon and elapsed time.
The right pane shows the request or response, repository, branch, agent, worktree
and attached terminals, plus the selected blueprint. Ready members also show
their commit and diff summary.
It refreshes every second, including details for the selected member.

| Key | Action |
| --- | --- |
| ↑/↓ or j/k | Select a member |
| Enter | Switch this tmux client, or attach from outside tmux |
| s | Spawn a member with repository, agent and blueprint selection |
| i | Send a one-line reply to the member's active pane; Enter sends, Esc cancels |
| t | Send the selected member to another attached work tab |
| / | Filter by id, name, ticket, repository or branch; Enter keeps the filter, Esc clears it |
| Page Up / Page Down | Scroll the details |
| x | Retire: inspect checks, then confirm; f toggles force, k keeps the branch, Esc cancels |
| r | Revive a dead member |
| g | Cycle attention, crew and repository grouping |
| e | Edit the selected member; Tab changes fields, Ctrl-s saves, Esc cancels |
| G | Crew manager: a adds, e edits, c cycles colour, x deletes |
| q | Close the overview |

In crew grouping, each collapsed row shows a crew and its status counts. Select
it to see a member table, then **Tab** into the table and use **↑/↓** to choose a
member. **Enter**, **e**, **x** and **r** act on that member; **Esc** returns to
the crew list. **→** or **Space** expands a crew in the list; **←** collapses it.
**H** shows crews with no live members. Unassigned members appear under **No crew**.
Crew titles with links are clickable in terminals that support hyperlinks.

The overview needs a terminal at least 60 columns wide and 10 rows high.
Use `motley ls` for plain text output.

### Spawn and steer

Press **s** to create a member from the overview:

1. Type to filter repositories; use arrows and Enter to choose one.
2. Enter a ticket (optional) and name. The branch previews as `feat/<ticket>-<name>`
   or `feat/<name>`; Tab moves between fields, including an editable branch.
3. Select Claude, Codex or OpenCode, then a compatible blueprint or **none**.
4. Fill any blueprint variables, then review the rendered prompt.
5. Press **e** to edit the prompt in `$EDITOR` (default `vi`), or Enter to launch.

Preparing the preview makes no worktree or state changes. Editing also works
with **none** selected as the blueprint. Keep the prompt file's schema comment;
motley removes it before delivery. The form shows fetch/create/push/copy progress
and selects the new member on success. Esc cancels before launch; created
worktrees are retained if launch fails. Use **e** on the created member to assign
a crew or colour.

Filtering is case-insensitive and matches characters in order: `fxc` matches
`FX cache`. Attention ordering is preserved. While entering a filter, Enter
returns to navigation; Esc clears it. In a crew member table, Esc first returns
to the crew list.

**i** replies with literal text plus Enter in the member's active tmux pane.
Permission requests require jumping into the agent; replies are disabled for
permission, ended and dead states. **t** selects another attached work tab;
the monitor's client is excluded.

The same tab action is available from the CLI:

```sh
motley tabs
motley send 412-fx-cache --tab /dev/ttys012
```

Use the TTY printed by `motley tabs`. Motley rechecks that the member and tab
are still attached before switching, and refuses to send to the monitor tab.

### Claude status reporting

Install hooks once, with motley on PATH:

```sh
motley hooks install claude
```

The hook installer merges into `~/.claude/settings.json`, preserves other settings and
hooks, and backs up an existing file before changing it. Running it again makes
no changes. Restart existing Claude sessions to load the hooks. To start Claude
again in an existing member, exit Claude and run `claude` in that same pane.
Sessions outside motley are ignored.

| State | Meaning |
| --- | --- |
| ⚠ permission | Approval needed; details show the requested tool/command |
| ? question | Claude is asking a question; details show its options |
| ✓ ready | A turn finished; details show the last response |
| ◌ idle | Session started or Claude is waiting for input |
| ● working / starting | Claude is running or the session is starting |
| ■ ended / ✗ dead | Agent exited / tmux session ended |

Use **Enter** to jump to a member and answer Claude there. Codex and OpenCode
currently show session availability without live agent attention states.

Hooks produce no terminal output and always exit successfully. Diagnostics go to
`${XDG_STATE_HOME:-~/.local/state}/motley/report.log`; per-member event history is
beside its manifest as `<id>.events.jsonl`. Hooks do not change manifests.

New member sessions show their status and ticket in tmux's status bar. For
existing sessions, add this to your tmux configuration and reload it:

```tmux
set -g status-interval 2
set -g status-left-length 50
set -g status-left "#{?#{@motley_member},#{@motley_status} #{@motley_ticket} ,}"
```

### Popup

The installer enables **Ctrl-b h** automatically. If you built the binary manually,
run:

```sh
motley init
```

This creates missing configuration files and prints a `source-file` line for
your `~/.tmux.conf`. Add the line and reload your tmux configuration, then press
your tmux prefix followed by **h** to open the overview in a popup. The default
prefix is Ctrl-b. Existing config files are preserved.

### Persistent monitor

Open a separate terminal tab or window and run:

```sh
motley monitor
```

The monitor runs in the `_motley` tmux session. **Enter switches another work
tab**, keeping the monitor visible. It chooses the most recently active tmux
client outside the monitor. Press **T** to pin a work tab, or select **Automatic**
to follow activity again. If there is no work tab, open another terminal and run
`motley attach <id>`.

In the monitor, **q** detaches the terminal and leaves the overview running.
Closing the terminal also leaves it running; `motley monitor` reconnects to it.
The `_motley` session does not appear in the member list.

The header shows per-status counts and **NEW ATTENTION** when a member needs you.
Selecting another member or jumping acknowledges the marker. To also ring the
terminal bell, add `monitor_bell = true` to motley's configuration, then restart
the overview process. Terminal notification behavior depends on your terminal's
bell settings.

After upgrading motley, restart an existing monitor to load the new binary
(`q` only detaches it):

```sh
tmux kill-session -t _motley
motley monitor
```

This restarts the overview; member sessions keep running.

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
fake agents and separate `tmux -L motley-test-<random>` servers. They never use
your working repositories or active tmux server. Artifact-copy tests compare
against the original wt script and a golden inventory.
