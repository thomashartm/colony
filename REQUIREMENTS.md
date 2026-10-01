# motley — Requirements

Document schema: `1`  
Status: W0–W7 implemented (Done); W8 implementation in progress; W9–W11 pending. See DELIVERY.md for roadmap tickets, validation and release checkpoints.
Source: user specification, 2026-09-30.

## Delivery agreement: MVP mode

Build one thin, usable vertical slice at a time, in the W0–W11 order in §12.
Start with W0. After each item, stop, report what was built, what was left out,
validation results and open questions, and wait for user feedback before starting
the next item. Feedback may reorder, change or remove later items and revise the
target design.

Hard-code sensible defaults before adding configuration. Add edge cases and polish
when actual use demonstrates the need. Do not implement later items early, create
placeholder commands or keybindings, or introduce dependencies on future items.
Every item must end usable and green. Explicit safety and parity requirements of
the active item remain part of that item.

Every file motley writes carries a numeric `schema`. Loaders ignore unknown keys
and default missing keys. Breaking changes require a small automatic migration or
a documented reset; a reset is acceptable before v1.0. The representation in
non-TOML files and third-party configuration needs the clarification in §14.

Sections 3–11 describe the evolving target state. Section 12 controls when a
capability is delivered. Package boundaries in §2 are a destination, not a reason
to create empty packages in W0.

## Product and terminology

motley is a terminal tool for running and governing parallel AI coding sessions.

| Term | Meaning |
| --- | --- |
| motley / mtly | The tool and its short command |
| member | One git worktree, one tmux session and one coding agent |
| crew | A group of members; a text title and optional link |
| gig | A package of work, described by the crew's optional gig field |

Agents: Claude Code, Codex and OpenCode. Platforms: macOS and Linux. Delivery:
a static Go executable per supported OS/architecture, without cgo, available
as both `motley` and `mtly`.

Ghostty displays tmux. motley never controls Ghostty directly.

## 0. Principles and non-goals

- tmux is the source of truth for live state. Durable state is stored in TOML
  manifests. No database.
- No background daemon. The TUI polls tmux; agents push status through hooks.
- No automatic GitHub synchronization. `gh` runs only on explicit user action,
  plus an optional lookup at spawn.
- No AppleScript or Ghostty API. Session control goes through tmux; repository
  operations through git; GitHub operations through gh.
- Hooks must never slow down or break an agent. `motley report` returns fast and
  always exits 0.
- motley gradually replaces `wt` 1.2.0 and `wt-clean` 1.0.0. The unchanged scripts
  are in `reference/wt` and `reference/wt-clean`. Match their behavior except for
  deviations explicitly recorded here. Early work items port only member needs.
- The user continues using wt and wt-clean alongside motley until W10.

## 1. Technology

| Concern | Choice |
| --- | --- |
| Language | Go ≥ 1.22, no cgo |
| CLI | `spf13/cobra` |
| TUI | `charmbracelet/bubbletea`, `bubbles`, `lipgloss` |
| Config/manifests | `pelletier/go-toml/v2` |
| Fuzzy matching | `sahilm/fuzzy` |
| External tools | tmux ≥ 3.2 for popups, git, optional gh, agent CLIs |
| Release | GoReleaser; darwin/linux × amd64/arm64 |

Use `os/exec` through small wrapper packages for external tools. Do not use tmux
or git libraries. Add dependencies when a work item first needs them.

## 2. Target package layout

```text
cmd/motley/main.go
internal/config      config loading, precedence, legacy import
internal/state       state paths, atomic writes, events.jsonl
internal/gitx        git wrapper, worktree porcelain parser, remote → web URL
internal/worktree    wt/wt-clean port: create/list/rebase/rm/clean/artifact copy
internal/tmux        sessions, options, clients, popups, capture-pane
internal/member      manifests; spawn/adopt/retire/revive/list (tmux joined to manifests)
internal/report      hook entry point and event → status mapping
internal/agents      adapter interface and claude/, codex/, opencode/
internal/blueprint   discovery, parsing, rendering
internal/gh          on-demand issue lookup, PR status and actions
internal/tui         main/detail/spawn/pickers/dialogs
integrations/claude/settings.hooks.json
integrations/opencode/motley.ts
integrations/codex/config.snippet.toml
reference/wt
reference/wt-clean
```

## 3. Configuration

Global path: `${XDG_CONFIG_HOME:-~/.config}/motley/config.toml`.

Target example (W0 introduces only schema and the two roots):

```toml
schema = 1
repos_root = "~/projects"             # main repos; never modify their working tree
worktrees_root = "~/worktrees"
worktree_dir = "{repo}/{prefix}{branch_slug}" # relative to worktrees_root
base_branch = "main"
prefix = ""                          # non-empty prefix renders as "<prefix>-"
push_on_create = true
env_globs = [".env", ".env.*"]
artifact_dirs = ["graphify-out"]
protected_branches = ["main", "master", "develop"]
branch_template = "feat/{ticket}-{slug}"
ticket_pattern = '(?:#|^|/)(\d+)(?:-|$)'
crew_suggest = "parent"               # parent | milestone | none
default_agent = "claude"
stale_after = "10m"

[tmux]
popup_key = "h"
color_status_bar = true
color_titles = true

[agent_badges]
claude = { label = "CC", color = "orange" }
codex = { label = "CX", color = "green" }
opencode = { label = "OC", color = "purple" }

[repos.aderis-api]
base_branch = "main"
env_globs = [".env", ".env.*", "config/*.local.yaml"]
artifact_dirs = ["graphify-out"]
setup = ["pnpm install --frozen-lockfile"]
```

Optional `<main-repo>/.motley.toml` accepts the same keys as `[repos.<name>]`.
Target precedence, independently per setting:

```text
CLI flag > environment > repo .motley.toml > [repos.<name>] > global > default
```

Environment: `MOTLEY_BASE_BRANCH`, `MOTLEY_WORKTREES_ROOT`. Honor legacy
`WT_BASE_BRANCH`, `WT_WORKTREE_DIR` (maps to `worktrees_root`) and
`WT_CLEAN_PROTECTED` (space-separated protected branches).

Invalid TOML is a hard error. A present `protected_branches` must be a non-empty
list of whitespace-free strings; invalid values are hard errors, never ignored.

Target `motley init`:

- Scaffold defaults without overwriting an existing config file.
- Import `base_branch`, `worktree_dir`, `prefix`, `env_globs`, `artifact_dirs` and
  `protected_branches` from existing `~/.config/bash-tools/wt/config.json` and
  `~/.config/bash-tools/wt-clean/config.json`, subject to the legacy-path mapping
  clarification in §14.
- Write motley's `motley.tmux.conf` in its config directory and print the
  `source-file` line.
- Print next steps for `motley hooks install <agent>`.

Additional target settings referenced below: optional `pop_command` (§9.5),
optional `monitor_bell = true` (§9.6), and configurable Codex capture patterns
(§7.3). Introduce each only when its slice requires it.

## 4. State on disk

Root: `${XDG_STATE_HOME:-~/.local/state}/motley/`.

```text
members/<id>.toml          manifest, written only by user-triggered commands
members/<id>.events.jsonl  hook-written event log
members/<id>.prompt.md     rendered initial prompt
members/archive/          all three files for retired members
crews.toml                user-triggered writes only
pending-attach            expiring handoff for pop/claim
report.log                hook error log
```

Manifest writes use a temporary file in the same directory, fsync and rename.
Hooks never write manifests.

### 4.1 Manifest

```toml
schema = 1
id = "412-fx-cache"
name = "FX cache"
repo = "aderis-api"
repo_path = "/Users/thomas/projects/aderis-api"
worktree = "/Users/thomas/worktrees/aderis-api/feat-412-fx-cache"
branch = "feat/412-fx-cache"
base = "main"
remote_url = "git@github.com:AderisERP/aderis-api.git"
web_url = "https://github.com/AderisERP/aderis-api"
ticket = "412"
crew = "fx-banking"                   # optional crew id
color = "blue"                        # optional override
agent = "claude"
agent_args = ["--permission-mode", "plan"]
# mode = "sandbox"                    # optional Claude preset; its args are in agent_args
blueprint = "feature-plan-first"
created_at = 2026-09-30T10:12:00Z
# retired_at = 2026-10-02T16:00:00Z    # archive only

[issue]                              # spawn lookup or explicit refresh
title = "Cache FX rates per business day"
url = "https://github.com/AderisERP/aderis-api/issues/412"

[gh]                                 # explicit refresh only
pr = 231
pr_url = "https://github.com/AderisERP/aderis-api/pull/231"
pr_state = "OPEN"                     # OPEN | MERGED | CLOSED
draft = false
review = "APPROVED"                   # reviewDecision
checks = "SUCCESS"                   # SUCCESS | FAILURE | PENDING | NONE
fetched_at = 2026-09-30T11:40:00Z
```

### 4.2 Events

One JSON object per line, at most 4 KB per line; truncate detail fields. At 1 MB,
rotate while keeping the newest half (rotation arrives in W11).

```json
{"schema":1,"ts":"2026-09-30T10:41:02Z","agent":"claude","event":"Notification","status":"permission","summary":"Bash: pnpm test --filter fx","agent_session_id":"…","detail":{"message":"…"}}
```

Append only on status transitions and for `SessionStart`, `Stop`, `Notification`
and `SessionEnd`. Use the latest logged `agent_session_id` for revive.

## 5. Worktree package: wt and wt-clean port

### 5.1 Base and creation

Resolve a requested base against local `refs/heads/<branch>`, then local `main`,
then local `master`; otherwise error.

1. Source is the main repo under `repos_root`. **Deviation:** wt uses the current
   checkout. Never modify the main repo's working tree.
2. Target is `worktrees_root` plus rendered `worktree_dir`. `branch_slug` replaces
   `/` with `-`. **Deviation:** wt uses `<parent>/<repo>-<prefix>-<slug>`.
3. Existing local branch: require CLI `--existing` or a TUI confirmation, then
   `git worktree add <path> <branch>`. **Deviation:** wt asks interactively.
4. New branch: `git fetch origin <base>`, falling back to `git fetch origin`;
   `git worktree add -b <branch> <path> origin/<base>`; if `push_on_create`, run
   `git -C <path> push -u origin <branch>`.
5. Copy artifacts as specified below.
6. Run configured setup commands in the new worktree, streaming output. A failing
   step is reported but does not delete the worktree. Setup is new behavior.

### 5.2 Artifact copying: exact parity

- Recursively find regular files whose **basename** matches an `env_globs` entry.
  Prune `.git`, `node_modules` and every `artifact_dirs` entry. Copy to the same
  relative destination, preserving mode and mtime (`cp -p`).
- Recursively find directories named in `artifact_dirs`, pruning `.git` and
  `node_modules`. Copy recursively preserving attributes (`cp -Rp`).
- Skip anything inside the target root when the target is nested in the source.
- Never overwrite an existing destination path. Create parent directories as
  needed. Report each copied item and a total count.

### 5.3 Full worktree commands

Exposed at W10 as `motley wt create|list|cd|rebase|rm|clean`.

| Command | Required behavior |
| --- | --- |
| `list` | Parse `git worktree list --porcelain`; path, branch or `(detached)`/`(no branch)`, 8-character HEAD, `(main)`/`(bare)` markers, `*` for the worktree containing CWD |
| `rebase [target] [-b base]` | Resolve base, fetch `origin <base>`, rebase onto `origin/<base>` in target; failure exits 1 with resolve-conflicts/`git rebase --continue` hint |
| `rm <target> [-f]` | Confirm; offer force on modified/untracked files; optional branch deletion using `-d`, then `-D` |
| `cd <target>` | Print only the path on stdout; all messages go to stderr |

Target lookup: exact path, then branch, then substring of branch/path. Multiple
substring matches are an error listing candidates. **Deviation:** wt takes the
first substring match.

### 5.4 Clean

`motley wt clean [days]`:

- Candidates exclude bare, main and the worktree containing CWD. Use a path `/`
  boundary so `repo-feat-1` does not match `repo-feat-10`.
- Age is days since `git log -1 --format=%ct`, or 0 if unknown. With `days`, show
  only worktrees idle at least that long.
- Multi-select: arrows or j/k, space toggle, a toggle all, g/G, Enter, q/Esc.
  Branch labels: branch; `<branch> (kept)` if protected; `(detached <sha7>)`.
- Confirm the selected list before cleaning. A failure never aborts the batch.

For each worktree, `clean_one`:

1. `git worktree remove --force --force <path>`; double force handles locks.
2. If the directory remains, use an `rm -rf` fallback.
3. Attempt `git worktree unlock <path>` ignoring errors, then prune.
4. Verify the worktree is absent from git's list. If present, fail and skip branch
   deletion.
5. Delete the local branch with `git branch -D` unless detached or protected.
6. Never touch remote branches.

Report `N cleaned · M failed` and failed items. If a cleaned worktree belongs to a
member, kill its tmux session and archive its manifest and associated files.

## 6. Members

### 6.1 Identity

Use `{ticket}-{slug}` when a ticket exists, otherwise `branch_slug`. If occupied
by a live or manifested member, prefix `{repo}-`. A tmux session name is the id
with `.` and `:` replaced by `_`. A tmux session is a member if and only if its
`@motley_member` user option is set.

### 6.2 tmux options and derived states

| Option | Writer | Meaning |
| --- | --- | --- |
| `@motley_member` | spawn/adopt | Member id |
| `@motley_status` | report; initialized by spawn | starting, working, permission, question, ready, idle, ended |
| `@motley_since` | report | Unix timestamp of last status change |
| `@motley_context` | report | Bounded schema-1 event JSON for the latest mapped hook; retains question/tool context without logging every tool call |
| `@motley_seen` | report | Unix timestamp of last hook call, for any event |
| `@motley_ticket`, `@motley_crew`, `@motley_agent` | spawn/edit | Status-line fields; crew is the title |
| `@motley_color`, `@motley_emoji` | spawn/edit | Resolved palette color and emoji |

Derived, never persisted: `dead` when a manifest exists without a tmux session;
`stale` when status is working and the last hook exceeds `stale_after`.

### 6.3 Spawn

```text
motley spawn --repo <r> [--ticket <t>] [--branch <b>] [--base <b>]
  [--name <n>] [--crew <id>] [--color <c>] [--blueprint <bp>] [--agent <a>]
  [--mode <claude-permission-preset>]
  [--var k=v]... [--existing] [--no-gh] [--switch|--attach|--detach]
```

1. Resolve a git repo under `repos_root`, with fuzzy directory-name matching.
2. Use `--branch` or render `branch_template` from ticket and the kebab-case name
   slug.
3. Unless `--no-gh`, an issue-number ticket triggers title/body/URL lookup and a
   parent-epic or milestone suggestion per `crew_suggest`. Match existing crews
   by URL; otherwise offer to create a crew. Failure warns, never fails spawn.
   Explicit `--crew` wins.
4. Render the blueprint into `members/<id>.prompt.md`.
5. Create the worktree (§5.1).
6. Write the manifest.
7. Start tmux detached, with the worktree CWD and `MOTLEY_MEMBER=<id>`:
   `tmux new-session -d -s <session> -c <worktree> -e MOTLEY_MEMBER=<id>
   "motley exec-agent <id>; exec $SHELL -l"`.
8. Set motley options, color and initial `starting` status.
9. Default inside tmux: switch the client. Default outside: exec tmux attach.
   `--detach` leaves the session detached; explicit mode flags select behavior.

`motley exec-agent <id>` builds adapter argv and execs the agent, reading the
prompt from the file and avoiding shell quoting of prompt text. Agent exit leaves
a login shell in the pane.

### 6.4 Lifecycle

- `adopt [--ticket …] [--crew …] [--color …] [--agent …]`: inside an existing tmux
  session whose CWD is a git worktree, derive a manifest and set options.
- `switch <id>`: switch-client inside tmux; attach outside.
- `attach [<id>]`: attach this terminal; without an id, show a fuzzy picker.
- `send <id> --tab <client>`: `tmux switch-client -c <client_tty> -t <session>`.
- `ls [--json]`: list members and status for humans or scripts.
- `revive <id>`: for a dead member, recreate tmux in its existing worktree and
  resume using the latest agent session id. Without an id, start fresh without
  replaying the initial prompt.
- `retire <id> [--force] [--keep-branch]`:
  1. Unless forced, refuse dirty work (`git status --porcelain`) or unpushed
     commits (`git rev-list @{u}..HEAD`; without upstream, commits beyond base).
     When gh is available, warn but do not refuse for an open PR.
  2. Kill the tmux session.
  3. Run `clean_one`, respecting protected branches and `--keep-branch`.
  4. Archive manifest, events and prompt with `retired_at`.

### 6.5 Crews

A member belongs to zero or one crew. Crews have a required free-text title,
optional gig description and URL, unique slug id, derived kind and palette color.
A gig names the package of work; the crew identifies the members doing it.

```toml
schema = 1

[[crew]]
id = "fx-banking"
title = "FX & Banking"
gig = "Ship FX caching"
url = "https://github.com/AderisERP/aderis-api/issues/400"
kind = "issue"
color = "blue"

[[crew]]
id = "q4-platform"
title = "Q4 platform hardening"
url = "https://github.com/orgs/AderisERP/projects/7"
kind = "project"
color = "purple"

[[crew]]
id = "spikes"
title = "Spikes & experiments"
kind = "text"
color = "grey"
```

Kind detection: GitHub `/orgs/<o>/projects/<n>` or `/users/<u>/projects/<n>` →
project; GitHub `/<o>/<r>/issues/<n>` → issue; another URL → link; no URL → text.

Commands:

- `crew add --title <t> [--gig <g>] [--url <u>] [--color <c>]`: slugify title, suffix `-2`,
  `-3`, etc. on collision. Default to the next unused palette color. At W9,
  title may be omitted for a GitHub issue/project URL and fetched once with gh.
- `crew list [--json]`; `crew edit <id> [--title …] [--gig …] [--url …] [--color …]`.
  An empty `--gig` clears the gig; descriptions cannot contain control characters.
- `crew rm <id> [--force]`: refuse references from members; force unassigns them.
- `crew assign <member> <crew-id|none>`.

Title/color changes immediately refresh `@motley_crew`, `@motley_color` and
`@motley_emoji` on live member sessions, using the color resolution below.

### 6.6 Colors

| Name | tmux | Lipgloss | Emoji | Contrast foreground |
| --- | --- | --- | --- | --- |
| red | colour160 | #d70000 | 🔴 | white |
| orange | colour208 | #ff8700 | 🟠 | black |
| yellow | colour220 | #ffd700 | 🟡 | black |
| green | colour34 | #00af00 | 🟢 | black |
| blue | colour33 | #0087ff | 🔵 | white |
| purple | colour135 | #af5fff | 🟣 | white |
| brown | colour130 | #af5f00 | 🟤 | white |
| grey | colour245 | #8a8a8a | ⚪ | black |

Resolve member color: manifest override → crew color → deterministic hash of id
into the palette. Status icon colors remain separate from identity colors.

- TUI: member-color `▌` at the beginning of rows and detail headers.
- When enabled, session `status-style` is `bg=<color>,fg=<contrast>`.
- When enabled, per-session `set-titles-string` is `<emoji> <ticket> <name>` and
  `set-titles` is on, giving Ghostty tabs the emoji title.
- Agent badges use their own configured label/color in rows and details,
  independently of member color.

## 7. Reporting and agents

### 7.1 Hook entry point

`motley report --agent <claude|codex|opencode> [--event <name>]` reads stdin for
Claude/OpenCode or argv for Codex notify.

- Resolve `$MOTLEY_MEMBER`. If unset, silently exit 0; hooks are global.
- Map event to status; always update `@motley_seen`. On a transition, update
  `@motley_status` and `@motley_since`, and append an event. Also append the
  explicitly retained lifecycle events in §4.2.
- Exit 0 on **every** error path. Never write stdout. Log errors to
  `<state-root>/report.log`.
- Performance budget: p95 < 15 ms; at most two tmux invocations, batching commands
  with `\;`.

### 7.2 Claude Code

`motley hooks install claude` backs up and idempotently merges hooks into
`~/.claude/settings.json`. Each runs `motley report --agent claude`.

| Hook | Status | Summary/data |
| --- | --- | --- |
| SessionStart | idle | Record session_id as agent_session_id |
| UserPromptSubmit | working | First prompt line |
| PreToolUse: AskUserQuestion | question | Question text and options |
| Other PreToolUse / PostToolUse | working | Tool and short input |
| Notification: permission prompt | permission | Message |
| Notification: idle prompt | idle | No required summary |
| Stop | ready | Prefer last_assistant_message; otherwise read only the final 64 KiB of transcript |
| SessionEnd | ended | No required summary |

Classify notifications by `notification_type` when available, otherwise message
text (permission versus waiting for input). Verify installed-version fields and
transcript format; retain recorded payloads as golden fixtures.

W3 verified Claude Code 2.1.285. AskUserQuestion also emits a generic permission
notification: retain the question state and options when this notification is
from the same agent session. Other permission notifications retain the latest
tool input for that session. Stop provides `last_assistant_message`; its
transcript may not yet contain the final record when the hook runs.

### 7.3 Codex

- Installer adds `notify = ["motley", "report", "--agent", "codex"]` to
  `~/.codex/config.toml`. Turn complete maps to ready with last assistant message.
- Prefer lifecycle hooks if supported by the installed version.
- Otherwise infer working/permission/question from `tmux capture-pane -p -t
  <session>` on each TUI poll, using configurable patterns for approval prompts,
  questions, spinner/working indicators.
- Verify notify payload, session-id field and resume command against the installed
  version.

### 7.4 OpenCode

Installer places `integrations/opencode/motley.ts` in the OpenCode plugin
directory. Subscribe to session/permission events and spawn report with JSON
stdin. Map prompt submitted → working; permission asked → permission; session
idle → ready; error → idle with summary. Verify plugin API, event names and
prompt/resume flags against the installed version.

### 7.5 Adapter interface

```go
type Agent interface {
    Name() string
    StartArgv(bin string, args []string, prompt string) []string
    ResumeArgv(bin string, args []string, sessionID string) []string
    ParseEvent(stdin []byte, argv []string) (Event, error)
    NeedsCaptureHeuristics() bool
}
```

If an agent cannot accept a prompt argument, omit it from argv and send the
prompt with `tmux send-keys -l` after the first idle event. Give up after 30 seconds
and log. The W6 dependency question for agents without idle hooks is in §14.

## 8. tmux integration

Target `motley.tmux.conf` (lines arrive with their respective work items):

```tmux
# schema = 1
bind h display-popup -E -w 90% -h 85% "motley"
set -as terminal-features "*:hyperlinks"
set -g status-interval 2
set -g status-left "#{?#{@motley_member},#{@motley_status} #{@motley_ticket} ,}"
set -ga status-right " #(motley status-line)"
set -g set-titles on
```

`motley status-line` emits aggregate attention counts such as `⚠1 ?1 ✓2` using
one `tmux list-sessions -F` call; budget < 20 ms. Introduced in W11.

## 9. TUI

With no arguments, motley opens a lazygit-style left list, right detail and bottom
key bar. Support full-screen and tmux popup use.

```text
 NEEDS YOU (3)                  │ ? 415 · bLink consent               waiting 4m
 ⚠ 433 migration       2m       │ Should consent refresh run in the scheduler or
 ? 415 bLink           4m       │ on first API call after expiry?
 ✓ 418 rate mig.      21m       │ 1) scheduler   2) lazy on request
 WORKING (2)                   │ crew FX & Banking ↗ · blink-svc @ feat/415-consent
 ● 412 FX cache       12s       │ blueprint feature-plan-first · codex · ttys012
 ● 431 Factur-X        1m       │ PR #88 draft ✗ checks (refreshed 1h ago)
 DEAD (1)                      │
 ✗ spike-pdfa          2h       │
───────────────────────────────┴───────────────────────────────────────────────
 ⏎ jump  i reply  b branch  c compare  p PR  R refresh gh  s spawn  x retire  ? help
```

### 9.1 List

Attention sections, in order: NEEDS YOU (permission/question/ready/idle), WORKING
(working/starting/stale), ENDED/DEAD. Within NEEDS YOU, oldest first.

`g` cycles attention → crew → repo. Crew grouping uses the collapsed crew view
in §9.7; crew headers show the title, color and an OSC 8 link when a URL exists.

Member rows show color bar, status icon, agent badge, ticket, truncated name,
short crew tag in crew color and time in current status.

| Status | Icon | Icon color |
| --- | --- | --- |
| permission | ⚠ | red |
| question | ? | yellow |
| ready | ✓ | green |
| idle | ◌ | grey |
| working | ● | blue |
| stale | ◐ | orange |
| ended | ■ | unspecified |
| dead | ✗ | red |

### 9.2 Detail

Permission: tool and command/file. Question: text and options. Ready: last
assistant message, branch, `git diff --stat <base>...HEAD`, short HEAD SHA.

Always show crew title/link, issue title, repo and branch, worktree path, agent,
blueprint, clients/tabs displaying the member from list-clients, and last-known PR
state and age when available in the delivered slice.

OSC 8 links: branch `…/tree/<branch>`, compare `…/compare/<base>...<branch>`, PR
and issue URLs.

### 9.3 Keys

Only expose keys once their capability is implemented.

| Key | Action |
| --- | --- |
| ↑↓ / j k | Navigate |
| / | Fuzzy filter id, name, ticket, repo, branch |
| Enter | Jump: switch inside tmux, exec attach outside; monitor rules below |
| t | Pick a tmux client and send selected member there |
| o | Pop out (§9.5) |
| i | One-line reply with send-keys -l followed by Enter; disabled for permission, jump instead |
| b / c / p | Open branch / compare / PR via macOS open or Linux xdg-open |
| P | PR menu: create with gh pr create --fill, mark ready, open |
| R | Refresh selected member's GitHub data |
| Shift+R | Refresh all GitHub data; terminal key conflict in §14 |
| s | Spawn form |
| a | Adopt a non-member tmux session |
| e | Edit name, ticket, crew and color |
| G | Crew manager: add, edit, delete, recolor |
| r | Revive dead member |
| x | Retire with confirmation showing pre-check results |
| W | Worktree clean view |
| T | Pin monitor work client |
| H | Toggle crews without live members |
| ? / q | Help / quit |

### 9.4 Refresh

Every second, make one `list-sessions -F` call containing all motley options and
one `list-clients -F '#{client_tty}\t#{client_session}'` call. Reload manifests
when mtime changes. Read event tails on selection change and when the selected
log mtime/size changes. Capture panes only for
Codex members. Monitor client-activity and selected-event freshness clarifications
are recorded in §14.

### 9.5 Pop into another tab

1. `motley pop <id>` writes id/timestamp to `pending-attach`, expiring in 60 s.
2. `motley shell-init <zsh|bash|fish>` prints an rc snippet. New shells outside
   tmux run `motley claim`, which atomically consumes pending attach. If present,
   the snippet execs tmux attach to the member. Thus opening a new Ghostty tab
   after `o` lands in that member.
3. Optional `pop_command` substitutes for writing pending attach, for example
   `ghostty +new-window -e tmux attach -t {id}` on Linux; verify this example before
   relying on it. No direct Ghostty API is introduced.

### 9.6 Monitor

`motley monitor` creates `_motley` if missing and attaches it, running the TUI in
monitor mode. This session is never a member and survives closing the terminal
window; rerunning monitor attaches again.

Enter switches the **work client**, leaving the monitor client in place. Choose
the most recently active other tmux client (`client_activity`) or a client pinned
with T. If none exists, tell the user to open a tab and run `motley attach`.
Normal TUI/popup jumps switch their current client.

Hook changes appear within about a second. Header totals such as `⚠2 ?1 ✓3 ●4 ✗1`
and an alert marker identify new NEEDS YOU entries. Optional `monitor_bell = true`
rings the terminal bell. Hide crews with no live members until H toggles them on.

### 9.7 Crew view

Collapsed by default, one left row per crew: `▌FX & Banking ↗   ⚠1 ●2 ✓1`.
Unassigned members appear in a final pseudo-crew, “No crew”.

Selecting a crew shows its title/link/count and a member table in the detail pane:

```text
 ST  MEMBER             AGENT TICKET REPO        BRANCH              SINCE PR
 ⚠   433 migration      CC    #433   aderis-api  feat/433-migration    2m   —
 ?   415 bLink consent  CX    #415   blink-svc   feat/415-consent      4m   #88 draft ✗
 ●   412 FX cache       CC    #412   aderis-api  feat/412-fx-cache    12s   —
 ✓   418 rate migration OC    #418   aderis-api  feat/418-rate-mig    21m   #231 ✔
```

Sort attention first, oldest waiting first. Shrink/drop columns from the right:
PR, SINCE, BRANCH. Scroll vertically. PR data is last-known only and empty before
W9.

Tab focuses the table; arrows select members; Enter jumps using monitor rules;
i/x/e act on the selected member; Esc returns to the list. Right/Space expands
a crew in the left list, Left collapses. An expanded member shows normal detail.

## 10. Blueprints

### 10.1 Discovery

Global `${XDG_CONFIG_HOME:-~/.config}/motley/blueprints/*.md` and repo
`<main-repo>/.motley/blueprints/*.md`. Repo wins a name clash. A non-empty `repos`
restriction limits which repos offer the blueprint.

### 10.2 Format and rendering

TOML front matter delimited by `+++`; Markdown body is Go `text/template`.

```markdown
+++
schema = 1
name = "feature-plan-first"
description = "Plan, wait for approval, then implement"
agent = "claude"
args = ["--permission-mode", "plan"]
repos = ["aderis-api", "blink-svc"]
fetch_issue = true
vars = ["constraints"]
+++
You are working on #{{.Ticket}}: {{.Issue.Title}}
Crew: {{.Crew.Title}} {{.Crew.URL}} · Branch: {{.Branch}} (base {{.Base}})

{{.Issue.Body}}

Constraints: {{.Vars.constraints}}
Start with a plan. Do not write code until I approve it.
```

Template data: Repo, Branch, Base, Ticket, Crew.Title/URL/Kind, Worktree, Name,
Issue.Title/Body/URL, Vars.&lt;name&gt;. Missing keys render empty strings. Unknown
template functions are validation errors. Commands: `blueprint list`,
`blueprint show <name>`, `blueprint validate`. Issue data arrives in W9.

### 10.3 Spawn form

Repo fuzzy picker → ticket/name with live branch preview → agent → blueprint
filtered by repo (including none) → variables → prompt preview → confirm.
In preview, e opens `$EDITOR` on the rendered prompt. Show fetch/worktree/copy/setup
progress inline as these capabilities arrive. On success, focus the new member.

## 11. GitHub: on demand only

- Convert `git@github.com:org/repo(.git)` and
  `https://github.com/org/repo(.git)` to web URLs.
- PR refresh:
  `gh pr list --repo <org/repo> --head <branch> --state all --limit 1 --json
  number,url,state,isDraft,reviewDecision,statusCheckRollup`. Compute check rollup.
- All-member refresh batches per repo with `--limit 200`, filtering member
  branches client-side.
- Issue lookup: `gh issue view <n> --repo <org/repo> --json title,body,url`, plus
  GraphQL for the parent issue or milestone according to `crew_suggest`.
- Crew title fetch, only when explicitly adding a GitHub URL without a title:
  `gh issue view` or `gh project view <n> --owner <org> --format json`.
- Missing/unauthenticated gh produces a one-line hint; other features keep working.

## 12. Delivery plan: thin vertical slices

### 12.1 Rules for every item

1. End with something usable in daily work. No hidden or half-built features.
2. Implement the smallest useful slice. Hard-code defaults; defer configurability,
   edge cases and polish until justified.
3. Use only earlier delivered capabilities. No forward dependencies or stubs.
4. Permit refactors and design changes; version written files with schema numbers,
   tolerate unknown/missing keys and migrate or document resets for breaking data
   changes before v1.0.
5. End green: build darwin/linux × amd64/arm64; clean `go vet` and
   `golangci-lint`; pass `go test ./...` and the item's new tests. Integration tests
   use `tmux -L motley-test-<rand>` and fixture repositories with local bare remotes,
   never the user's tmux server or real repositories. CI must be green; update
   README and `--help`; tag `v0.<n>.0` (numbering clarification in §14).
6. Stop after each item. Summarize built scope, deliberate omissions, validation
   and open questions. Wait for user feedback before the next item.

### W0 — Shell

**Scope:** Go module, cobra root, `motley version`, CI build matrix/vet/lint/test,
GoReleaser snapshot. Config has only schema, `repos_root` and `worktrees_root`,
defaulting to `~/projects` and `~/worktrees`. Precedence is file > default only.

**Done:** CI green; smoke test runs the binary and reads config.

### W1 — First member, end to end

**Scope:** `spawn --repo <r> --branch <b> [--agent claude|codex|opencode]
[--ticket <t>] [--name <n>]`. New branches only: main then master base, fetch,
worktree add from origin/base, push -u. Exact artifact copy from §5.2 using fixed
`.env`, `.env.*`, `graphify-out` defaults. tmux with MOTLEY_MEMBER and member,
ticket, agent options. Start agent without prompt; drop to shell on exit. Persist
known manifest fields. `ls` table: id/ticket/repo/branch/agent/alive-or-dead;
`attach <id>` and `switch <id>`.

**Done:** Integration test spawns, ls reports alive, killing the test session
reports dead. Artifact-copy golden test.

### W2 — Overview TUI v0

**Scope:** No-arg TUI with alive/dead list, manifest detail, 1 s refresh, Enter
jump, q. `init` writes popup config. Thin monitor: `_motley`, jump to the most
recently active other client, T to pin work client.

**Done:** List/selection model tests; jump inside/outside tmux; monitor jump
switches another test client and leaves monitor in place.

**Checkpoint focus:** Layout and density before adding more information.

### W3 — Attention states for Claude Code

**Scope:** report, Claude mapping/install, events without rotation, status/since/
seen options, NEEDS YOU/WORKING/DEAD sections with icons and status detail,
tmux status-left, monitor totals/alert/optional bell.

**Done:** Golden tests from recorded Claude payloads; report exits 0 on bad input
or unset MOTLEY_MEMBER; p95 < 15 ms benchmark; idempotent installer with backup.

### W4 — Finish and resume members

**Scope:** retire with dirty/unpushed checks, force, clean_one for one worktree
and archive; adopt; revive with Claude resume, fresh start for other agents;
TUI x confirmation and r.

**Done:** Dirty/unpushed retire refusal and force success; integration test with
a fake agent demonstrates Claude resume.

### W5 — Crews and colors

**Scope:** Crews with required title and no gh; palette/resolution; crew view,
member table, H; spawn/adopt crew/color flags; color bars, agent badges and crew
tags; grouping g; e for crew/color; manager G.

**Done:** Kind and color-resolution tests; crew color edits recolor live sessions.

### W6 — Blueprints

**Scope:** Discovery/parsing/rendering with Repo, Branch, Base, Ticket, Name,
Worktree, Crew.*, Vars.*. Spawn blueprint/var flags; list/show/validate; Claude
positional prompt. W6 uses the verified native prompt arguments for Codex and
OpenCode as well, resolving the no-forward-dependency question in §14.

**Done:** Rendering golden tests and fake-agent integration test receiving prompt.

### W7 — Spawn and steer from the TUI

**Scope:** Spawn form (repo → ticket/name/branch preview → agent → blueprint →
variables → editable prompt preview → progress/launch), reply i, tabs view and t,
filter /, `motley tabs`, `motley send`.

**Done:** Form validation tests; reply/send integration tests.

### W8 — Codex and OpenCode

**Scope:** Both integrations/installers, native prompt and resume support for both
adapters. Use Codex native lifecycle hooks when supported (§7.3); capture heuristics
are only needed for a legacy fallback.

**Done:** Recorded payload fixtures and native reporting/resume integration tests;
heuristic tests using captured screens if a capture fallback is used.

### W9 — GitHub on demand and links

**Scope:** Branch/compare/PR browser actions and OSC 8; issue lookup at spawn,
Issue.* template data, crew suggestions/title fetch; PR refresh/menu; open-PR
warning on retire.

**Done:** gh behind an interface with fake-gh tests; features tolerate absent gh.

### W10 — Full worktree tooling; retire wt and wt-clean

**Scope:** Remaining §5/§3 capabilities: existing-branch spawn;
`wt create|list|cd|rebase|rm|clean`; full config precedence, repo overrides, setup,
legacy env vars/import, protected-branch validation, W clean view.

**Done:** Parity tests against the reference scripts for behavior listed in §5.

### W11 — Pop out and hardening

**Scope:** pop/claim/shell-init/o, event rotation, stale detection, aggregate
status-line.

**Done:** Atomic claim and 60 s expiry; zsh/bash/fish snippet tests.

## 13. Verify during implementation; do not guess

These are verification tasks for their relevant slice, not prerequisites for
writing this document or implementing unrelated earlier slices.

- Claude: installed hook fields (`notification_type`, `tool_name`,
  `transcript_path`, `session_id`) and transcript records for last assistant text.
- Codex: installed notify payload, lifecycle-hook support, session-id field and
  resume command.
- OpenCode: plugin API, event names, initial-prompt flag and resume flag.
- gh: GraphQL fields for sub-issue parent lookup.
- Ghostty: optional Linux `+new-window -e …` behavior.

Record installed versions with fixtures so evidence has a reproducible context.

## 14. Clarifications to settle at the relevant checkpoint

The following inconsistencies or unspecified choices are retained for steering;
they do not authorize expanding an earlier work item.

| When | Question |
| --- | --- |
| W0 (resolved) | Use literal item numbering: W0 → v0.0.0, W1 → v0.1.0. GitHub destination supplied by the user: https://github.com/thomashartm/motley. |
| W3 (resolved for current writers) | Owned JSONL records carry schema: 1; generated config/snippets use schema fields/comments. Preserve third-party Claude settings format and exact backup bytes; the integration template has schema 1 and backup filenames carry motley-v1. Copied reference/artifact files remain unchanged. Prompt representation is deferred to W6. |
| W1 (resolved) | Use the branch-based default path, such as `aderis-api/feat-412-fx-cache`. Reserve normalized tmux session names as well as ids; prefix the repo on collision, then refuse if still occupied. |
| W3 (resolved) | Tail-read events on selection change and when the selected log mtime or size changes during polling. Ready commit/diff data refreshes with those events. This keeps selected details live without reading whole logs. |
| W2 (resolved) | Each poll reads client_name, client_tty, client_session and client_activity in one client-list call. Monitor jumps recheck clients before switching. T pins a client in memory; q detaches the monitor while keeping its TUI running. The popup binding uses run-shell to expand the originating client before display-popup runs. |
| W4 (resolved) | Adopt manages linked worktrees only, renames the session to its member id, and sets the session environment. Existing processes require the printed MOTLEY_MEMBER export and an agent restart. Detached worktrees use a detached-<sha8> id when no ticket is given. |
| W4 (resolved) | Refuse retiring the caller's own tmux session, since killing its pane would interrupt cleanup. Use the monitor, another session or an outside terminal. Force overrides dirty/unpushed checks, never worktree ownership checks. |
| W4 (resolved) | Revive is detached and applies only to active dead manifests with an existing linked worktree. Archived members stay retired. Archive filename collisions receive a UTC timestamp suffix, preserving previous history. Protected branches remain hard-coded main/master/develop until W10. |
| W5 (resolved) | Fixed palette and agent badges; tmux status-bar and emoji-title styling are enabled. Styling switches and badge customization are deferred until usage warrants them. Crew titles are manual, URLs are http/https, and no GitHub lookup occurs. Empty crew/color fields in the member editor clear assignment/override. Forced crew removal unassigns active/dead manifests; archives remain historical. |
| W6 (resolved) | Installed Claude 2.1.286 and Codex 0.159.2 accept positional prompts; OpenCode 1.18.21 accepts --prompt. Use these native arguments now, bringing initial prompt delivery forward from W8; no idle-hook or send-keys fallback is needed. Revive preserves blueprint args without replaying the prompt. CLI agent wins; conflicting agent-specific blueprint args are refused. |
| W6 (resolved) | Prompt files use a schema-1 Markdown comment, stripped on delivery. Render/validate known fields and missing Vars keys as empty strings; unknown struct fields are template errors. Vars are optional, with repeated CLI values taking the last value. Render before worktree creation, then save the prompt before the manifest/session; cap prompts at 64 KiB for portable argv delivery. |
| W7 (implementation) | Use a fixed feat/{ticket}-{slug} form branch with an editable override, and a case-insensitive subsequence matcher preserving attention order. The planned sahilm/fuzzy module is unavailable in this network-restricted environment; defer ranking/dependency changes. Preparing a spawn is read-only; launch uses the reviewed prompt snapshot. Manual prompts without a blueprint set prompt=true in the schema-1 manifest. Reply targets the active pane and refuses permission/ended/dead states. |
| W8 (implementation) | Codex 0.159.2 has native hooks enabled; use hooks.json and its /hooks trust review, preserving approval settings. No legacy notify/capture fallback in this slice. Launch Codex with --no-daemon so hooks inherit the member environment. OpenCode 1.18.21 uses its plugin event API, native --prompt and --session flags. Installer backups and recorded session ids support both agents. Synthetic contract fixtures are present; live payload recordings and the real tmux gate remain outstanding. |
| W9 | R and Shift+R are ordinarily the same uppercase terminal key. Choose distinguishable selected/all refresh bindings. |
| W10 | Legacy wt worktree_dir means a root directory; motley worktree_dir is a relative template. Specify import mapping, consistent with WT_WORKTREE_DIR → worktrees_root. |
| W10 | Env copying requires basename-only parity, while the example `config/*.local.yaml` contains a path. Choose whether to correct the example or explicitly change matching semantics. |
| W10 | Define precedence when both legacy WT_* and corresponding MOTLEY_* variables are set. |

All unimplemented work items remain pending. Delivery status and checkpoint
notes are in DELIVERY.md; no later slice starts without feedback on the preceding one.
