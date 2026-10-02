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
sets up PATH, the tmux popup and reporting for installed agents. It installs missing Git, Go and
tmux through your package manager; macOS requires Homebrew. Existing tmux must
be 3.2+ and existing Go 1.21+.

Install and authenticate your agent CLI separately. Open a new terminal and run
`mtly`. Restart agents to load hooks. In Codex, review and trust the Motley
hooks with **`/hooks`**. Rerun the installer to
upgrade.

The installer shows a short status checklist; full output is saved under
`~/.motley/install-history/`.

From a checkout:

```sh
bash install.sh --local  # install local source
make build              # build only; run ./bin/mtly
```

### Update or uninstall

Release updates require `gh` and a writable installation directory:

```sh
mtly update --check # check GitHub's latest release
mtly update         # verify checksums and replace both commands
```

Updates preserve settings and worktrees. Details go to `~/.motley/update-history/`.
Restart the monitor after updating.

To uninstall (requires Python 3):

```sh
curl -fsSL https://raw.githubusercontent.com/thomashartm/motley/main/uninstall.sh -o motley-uninstall.sh && bash motley-uninstall.sh
```

From a checkout or release archive, run `bash uninstall.sh`. It removes the
commands installed in `~/.local/bin` and disconnects agent hooks and the tmux
popup. Settings, shared PATH entries, worktrees, branches and running sessions
stay intact. Restart agents and tmux after your sessions finish. Full output
and backup paths are saved under `~/.motley/uninstall-history/`.

## Start working

By default, main repositories live under `~/projects` and worktrees under
`~/worktrees`. Change these locations with `repos_root` and `worktrees_root` in
[`~/.config/motley/config.toml`](#configuration).
Repositories need an `origin` remote and a local `main` or `master` branch.

Run `mtly`, press **s**, choose a repository and agent, then review and launch.
**Launching creates and pushes a new branch.** Your main checkout stays intact.
Press **Enter** to attach; **Ctrl-a d** detaches without stopping the session.

Or use the CLI:

```sh
mtly spawn --repo api --branch feat/412-fx-cache --ticket 412 --name "FX cache" --agent codex
mtly ls
mtly attach 412-fx-cache
```

`--repo` names a directory under the repository root. Add `--detach` to launch in
the background. Claude is the default agent. Local `.env`, `.env.*` and
`graphify-out` artifacts are copied into the worktree.

### Claude permission modes

Pick a mode with `--mode`, or in the spawn form after choosing a blueprint:
`manual`, `acceptEdits`, `plan`, `auto`, `dontAsk`, `bypassPermissions` or
`sandbox`. Without one, Claude uses its own configured default. A blueprint that
sets `--permission-mode` keeps its choice; adding `--mode` to it is refused.

`sandbox` accepts edits and runs Bash in Claude Code's sandbox. Commands can
write inside the worktree and temp only; `git commit` still works. The settings
are passed with `--settings`, so nothing is written to the worktree. Claude won't
start if the sandbox is unavailable (Linux needs `bubblewrap` and `socat`). Claude
can still ask to rerun a blocked command outside the sandbox; approve that only
if you mean it.

```sh
mtly spawn --repo api --branch feat/412-fx-cache --mode sandbox
```

## Overview

Select a member and click **Open agent** (or press **o** / **Enter**). With one
tab, it opens there; with a separate work tab, it opens in that tab. Click
**Back to monitor** in the agent footer to return. No tmux shortcuts are needed.

The list groups members by crew within each status section, with columns for
title, ticket and crew alongside the status icon and agent badge. Ticket cells
open in your browser with a left click, including inside tmux popups. Explicit
web URLs work with any tracker; numeric tickets
link to issues on the member's GitHub or GitLab.com remote.

Click the pinned **Overview** entry or press **Home** for main actions: spawn a
member, add an existing Claude, open an agent or manage crews. **Open agent** here
shows a picker of running sessions, including members hidden by the list filter.
Selecting a member gives it a separate action menu. You can also reach Overview
with **↑** from the first list entry, then **Enter** or **→** to open its actions.

Details use bold labels, aligned values and compact rows with spacing between
sections. Narrow panels stack labels above their values; long paths wrap without
losing their indentation.

The footer groups controls by purpose and fits them into up to two rows. Small
windows show essential controls; use **→** to reach the full Actions menu.
Clickable navigation buttons have their own bottom row; at minimum height,
only the buttons are shown while navigating the overview.

| Key | Action |
| --- | --- |
| ↑/↓ or j/k | Select a member, option or field |
| 1 / 2 / 3 | Focus List / Details / Actions (or click the footer button) |
| Home | Select Overview and open main actions |
| → / ← | Move from list to details to Actions, or back |
| Enter / o | Open the selected agent (Enter runs the selected action in Actions) |
| s | Spawn a member |
| i | Send a reply |
| t | Send the member to another work tab |
| / | Filter members |
| g | Group by attention, crew or repository |
| e | Edit member details |
| m | Manage crews |
| d | Terminate session (keep work) |
| x / r | Retire / revive |
| Page Up / Page Down | Scroll details |
| c | Copy the message above the footer (or click it) |
| q | Close |

The line above the footer shows Motley's messages and errors, cut to the window
width. **c**, a click on that line, or **Copy message** in Actions copies the full
text to the clipboard; the line then shows **✓ copied**. Inside tmux the copy goes
through tmux, which keeps it as a paste buffer and passes it to the terminal when
`set-clipboard` is `external` or `on` (the default). If it is `off`, Motley says
the text is only in tmux's paste buffer. Outside tmux, Motley sends the standard
OSC 52 clipboard sequence, which Ghostty accepts by default.

Press **3** or click **3 Actions**, then **↑/↓** and **Enter**
to run an action on the selected member. **Terminate agent** stops its session
and keeps the worktree, branch and history; **Revive** restarts it.
Editors use **↑/↓** to move through fields, **Save** and **Cancel**; **Enter** activates and **Esc** cancels. **←/→**
move the text cursor while editing. Click a field to focus it, or click the
highlighted **Save**, **Send**, **Delete** or **Cancel** buttons. **Tab/Shift-Tab**
and **Ctrl-s** still work. Colour fields are selectors: use **←/→** or click
the arrows to cycle through colour swatches, including **Inherit** for members
and **Automatic** for crews. The member's **Crew** field uses the same arrows
to select an existing crew by name or **No crew**.

In crew view, the first **→** expands a crew; the next enters its member table.
**←/Esc** returns to the list, where **←** collapses the crew. **Tab** also enters
the table; **h** shows inactive crews. In the crew manager, **→** opens its actions.

### Inside an agent's tmux session

Shortcuts stay visible at the bottom, below the normal tmux status row. Existing
sessions gain the footer when you attach or switch to them with the updated Motley.
Click the underlined footer controls, or use the keys below. In the overview,
click **Open agent**, **List**, **Details**, **Actions**, a member or an action; the mouse wheel
scrolls lists and details. Forms still use the keyboard.

With Motley’s tmux configuration loaded, press **Ctrl-a**, release both keys, then press
the next key. Use your own prefix if you changed it.

The generated tmux config enables mouse support and binds **Ctrl-a Ctrl-a** to
send a literal Ctrl-a to the agent. `motley init` preserves existing config files;
for an older installation, add these settings to `motley.tmux.conf` and reload it
with `tmux source-file ~/.config/motley/motley.tmux.conf`:

```tmux
set -g mouse on
set -g prefix C-a
unbind C-b
bind C-a send-prefix
if -F '#{>=:#{version},3.4}' 'set -as terminal-features ",xterm*:hyperlinks"'
```

- **Details:** **Ctrl-a h** opens Motley. Select a member with **↑/↓**, then
  **→** focuses its details. Scroll with **↑/↓** or **Page Up/Page Down**;
  **←/Esc** returns to the list.
- **Monitor:** **Ctrl-a m** opens the persistent monitor in this tab;
  press it again to return to the previous session. The agent keeps running.
- **Back to the agent:** **q** closes the popup. **Enter** switches to the
  selected member instead.
- **Other tmux windows/panes:** **Ctrl-a w** opens the window picker;
  **Ctrl-a n/p** selects the next/previous window; **Ctrl-a arrow** selects a pane.
- **Scrollback:** **Ctrl-a [**, then arrows or **Page Up/Page Down**;
  **q** leaves scrollback.
- **Detach:** **Ctrl-a d** returns to your shell and keeps the agent running.

### Open the agent in another Ghostty tab

1. Press **Cmd-T** in Ghostty on macOS to open a tab (default shortcut).
2. Run `mtly ls` to find the member ID, then `mtly attach <id>` in that tab.
   This attaches to the existing session; it does not start another agent.
3. To move rather than share the view, detach the original tab with **Ctrl-a d**.

Motley's **t** sends a member to an already attached work tab; it does not create
a Ghostty tab. Ghostty shortcuts are [configurable](https://ghostty.org/docs/config/keybind).

For a persistent overview in a separate tab, run
`mtly monitor`: **Open agent** uses an attached work tab, or the current tab if
it is the only monitor tab. **p** chooses a work tab, and
**q** detaches the monitor. After upgrading, restart it with
`tmux kill-session -t _motley`, then `mtly monitor`.

Claude, Codex and OpenCode report working, permission, question and ready states;
an interrupted turn shows idle. When the agent quits or crashes, the member shows
ended and replies are refused rather than typed into the shell. Codex and OpenCode
report nothing until the first prompt, so a bare or revived member shows starting
until then. Jump into the agent for permission requests. To set up reporting
individually:

```sh
mtly hooks install claude
mtly hooks install codex
mtly hooks install opencode
```

Restart the agent afterward. Codex requires native hooks (verified live with 0.159.3)
and trust approval through **`/hooks`**; Motley preserves its approval settings.
Codex asks questions only in Plan mode.
OpenCode uses a plugin (verified with 1.18.21). Hooks are silent outside Motley.

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

Opening `mtly` inside an agent returns to the independent monitor. Select a
member → **Actions** → **Retire member + worktree** to remove it. Retirement
refuses unsaved or unpushed work; the **Force** toggle (CLI: `--force`) discards it. Remote branches remain. Revive requires the
worktree to exist and cannot restore retired members. Claude resumes its last
recorded session, as do Codex and OpenCode when reporting captured a session id.
Without a recorded id, the agent starts fresh. If the agent exited but its tmux
shell is still alive, restart the agent in that shell instead.

### Add an existing Claude session

In the monitor press **a**, or **3 Actions → Add existing Claude**, then select
its name and press **Enter**. Claude keeps running in its original terminal;
no restart, tmux setup or worktree conversion is needed. Requires Claude Code's
`claude agents --json` command.

```sh
mtly import --list
mtly import <session-id> --name "Existing work"
```

Status updates come from Claude. The session stays in its original terminal, which
Motley never controls: **Open agent** tells you where it runs. Switch to that tab
and reply there. **Terminate** stops Claude; **Revive** resumes the saved conversation
in Motley’s tmux session. **Retire** keeps imported directories and branches,
even with Force.

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

Motley creates `~/.motley/config.toml` on first launch, preserving it on later
starts. Existing XDG settings are copied there once. Edit this file:

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

Requires Go 1.22+, Git, tmux, cp, bash, Python 3 and Node 24 for plugin tests;
golangci-lint 2.14.0 and GoReleaser
2.18.2 for the full checks.

```sh
make test     # install/uninstall, plugin and Go tests
make check    # tests, vet, lint, macOS/Linux builds for amd64/arm64
make snapshot # release archives in dist/; no publishing
```

Tests use temporary repositories, fake agents and isolated tmux servers.
Set `GOLANGCI_LINT` or `GORELEASER` to use tools outside PATH.
Delivery status is tracked in [DELIVERY.md](DELIVERY.md).
