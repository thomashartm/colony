# Delivery checkpoints

Document schema: `1`

motley is built one usable slice at a time. Scope and acceptance criteria live in
[REQUIREMENTS.md](REQUIREMENTS.md#12-delivery-plan-thin-vertical-slices).

After each item, stop, summarize what was built and left out, report validation
and open questions, and wait for user feedback before starting the next item.
Feedback may change the scope or order of later items. Keep milestone status and
implementation-process notes here; the README describes how to use the tool.

## W0 — Shell: complete

Delivered the Go CLI shell, configuration loading for repository and worktree
roots, `motley version`, help, executable smoke tests, CI and GoReleaser snapshots.

- Commit: `e4492c8`; release tag: `v0.0.0`.
- All nine [release-tag CI jobs passed](https://github.com/thomashartm/motley/actions/runs/36705427765):
  tests and vet on macOS/Linux, the Go 1.22 minimum on Linux, lint, four platform
  builds and snapshot packaging.
- Local validation passed with Go 1.25.5. Go 1.22 test execution on the local
  macOS failed with `missing LC_UUID load command`; its minimum-version check
  passed on Linux in CI.
- Session management, tmux integration, hooks and the TUI were deliberately
  left out of this slice.

Local checks, hosted CI and tags are reported separately. A local pass does not
establish a hosted CI result. Release numbering follows work-item numbering:
W0 is `v0.0.0`, W1 is `v0.1.0`, and so on.

## W1 — First member: complete

Delivered new-branch worktree creation from main/master, immediate upstream push,
artifact copying, schema-1 manifests and interactive agent startup in tmux.
Commands: spawn, ls, attach and switch. Agent exit leaves a login shell.

- Fixed defaults; exact repository directory names. No additional config settings.
- Spawn attaches outside tmux and switches inside; `--detach` supports background
  creation and integration tests.
- Worktree paths follow the configured-root plus repository plus branch-slug rule.
  Ticket-based ids do not change the directory naming rule.
- Id allocation accounts for normalized tmux names. Existing identities get a
  repository prefix; a second collision is refused. Simultaneous spawn commands
  are serialized with an OS lock; a competing invocation asks the user to retry.
- Tests exercise real git and isolated tmux servers with fake Claude, Codex and
  OpenCode executables: startup, environment, no prompt arguments, upstream push,
  main/master bases, shell fallback, switching, alive/dead state and preflight
  refusals. Attach and outside-tmux switch argv are tested with a fake tmux.
- Artifact-copy tests compare an explicit golden inventory and native attributes
  against the unchanged wt function. Nested targets are excluded, and destination
  symlink parents are refused to prevent copying outside the worktree.
- Implementation commit: `46b45a6`; release tag: `v0.1.0`.
- Local tests, vet, lint, all four builds and snapshot packaging passed.
  All nine [implementation CI jobs passed](https://github.com/thomashartm/motley/actions/runs/36716731133),
  including the lifecycle tests on macOS/Linux and the Go 1.22 check on Linux.

Deliberately left out: fuzzy repo selection, existing-branch creation, prompt
passing, agent hooks/status, retire/revive, TUI, GitHub lookups and further config.
W1 supports branch slugs containing ASCII letters/digits, dots, underscores and
hyphens; broader name handling can follow actual usage.

## W2 — Overview TUI: complete

Delivered the Bubble Tea overview with alive/dead sections, a scrollable manifest
detail pane, stable selection across refreshes, and terminal-aware jumping.

- Polls sessions and clients once each per second. Manifest parsing is cached
  by modification time and size. Client activity supports automatic work-tab
  selection; action-time checks catch disconnected or repurposed clients.
- `motley init` scaffolds the current three-key config and popup binding, without
  overwriting either file. Generated tmux config carries a schema comment.
- `motley monitor` creates or reconnects to `_motley`. Enter targets another
  client; T pins a work tab or restores automatic selection. q detaches the
  monitor client, keeping the overview alive. A missing work tab gets a hint.
- `motley config` retains the former no-argument config display now that the root
  command opens the TUI. The overview needs at least a 60 × 10 terminal.
- Model tests cover selection, ordering, removals, dead-session refusal, monitor
  targeting and pinning, and narrow layouts. Real pseudoterminal tests exercise
  normal and popup jumps, outside-tmux attach, monitor reuse, pinning, detach,
  and alive-to-dead refresh on an isolated tmux server.
- Implementation: `9f5d5a6`, compatibility fix: `ec61c54`, final test adjustment:
  `ba91a73`; release tag: `v0.2.0`.
- Local tests, vet, lint, darwin/linux × amd64/arm64 builds, and GoReleaser
  snapshot packaging passed. All nine
  [implementation CI jobs passed](https://github.com/thomashartm/motley/actions/runs/36733809284),
  including macOS, Linux and Go 1.22. The terminal integration also passed locally
  on Linux arm64 with Go 1.22 and tmux 3.3a in a disposable container.
- Linux compatibility testing found that older tmux versions sanitize tab
  separators without UTF-8 mode. Session/client reads now explicitly use `-u`;
  the standalone terminal test also runs with the C locale. Popup tests wait
  for tmux to acknowledge the prefix key before sending h.

Deliberately left out: attention states/hooks, spawn forms, crews, permission-mode
selection and other later-item features. Native Claude and Codex permission-mode
requests remain tracked in [#1](https://github.com/thomashartm/motley/issues/1) and
[#2](https://github.com/thomashartm/motley/issues/2).

## W3 — Claude attention states: complete

Delivered Claude hook reporting, attention sections and details, monitor counts,
new-attention markers and optional terminal bell (`monitor_bell = true`).

- `motley hooks install claude` merges seven hooks idempotently and backs up an
  existing settings file. Global user hooks are installed only on explicit use
  of that command; implementation testing used temporary settings.
- Recorded real Claude Code 2.1.285 lifecycle, Read, Bash, question, permission
  and idle payloads. Fixtures include version and reproduction notes. Stop's
  direct assistant message avoids a transcript flush delay observed in testing;
  older payloads use a bounded transcript-tail fallback.
- `report` always exits 0 and emits no stdout/stderr. It ignores sessions without
  MOTLEY_MEMBER, caps payloads and event lines, bounds stdin/tmux waits at 250 ms,
  and uses two tmux invocations. Error logging gets at most another 10 ms and
  appends schema-1 report.log records when the state filesystem is available.
- tmux holds status, transition time, last-seen time and bounded latest event
  context. Generic permission notifications preserve a pending question, or
  retain the preceding tool/command. Hooks never write manifests. Events append
  only for transitions and the specified lifecycle/notification events.
- NEEDS YOU sorts oldest first; WORKING and ENDED / DEAD follow. Selected details
  tail-refresh on event mtime/size changes; ready detail includes commit and diff.
  `ls` includes STATUS alongside the existing alive/dead STATE column. Existing
  unreported sessions remain usable. New sessions get a tmux status display;
  init preserves existing config files, with manual upgrade steps in the README.
- Owned event records have schema 1. Claude settings preserve their native format
  and backups preserve exact bytes (versioned filenames); no unrelated schema
  key is inserted into third-party configuration.
- Tests cover recorded mappings, backup/idempotency, bounded records, timeout and
  error behavior, ordering/alerts, and real hook-to-terminal updates with an
  isolated tmux server and fixture Git repository. The terminal test covers the
  optional bell, fresh selected details and a ready commit/diff. It also verifies
  exactly two tmux calls and unchanged manifest bytes/mtime.
- Local full-process benchmark: **14.39 ms p95** across 300 invocations on macOS
  arm64 (Apple M4 Pro, Go 1.25.5, tmux 3.6a), using the shipped CGO_ENABLED=0 /
  trimpath build and real tmux. This is a local measurement, not a guarantee for
  every host. Reproduce with `go test ./cmd/motley -run '^$' -bench
  '^BenchmarkReportCLI$' -benchtime=300x`.

Local tests, vet, lint, all four static builds and snapshot packaging passed.
Implementation commit: `a6dee0a`; release tag: `v0.3.0`. All nine
[implementation CI jobs passed](https://github.com/thomashartm/motley/actions/runs/36742146219),
including real terminal/report tests on macOS/Linux and Go 1.22 on Linux.

Deliberately left out: event rotation and stale detection (W11), Codex/OpenCode
reporting (W8), retire/revive (W4), crews, spawn forms and native permission modes
([#1](https://github.com/thomashartm/motley/issues/1),
[#2](https://github.com/thomashartm/motley/issues/2)).

## W4 — Finish and resume members: complete

Delivered `retire`, `adopt`, `revive`, and the overview's x/r actions.

- Retirement refuses dirty/untracked or unpushed work, using the base branch
  when no upstream exists. Force explicitly skips these checks; ownership and
  main-worktree checks always apply. Normal retirement checks again after
  stopping the agent in case work changed during shutdown.
- Ports wt-clean's double-force removal, directory fallback, unlock/prune,
  registration verification and local branch deletion. Main/master/develop and
  all remote branches are retained; CLI/TUI can retain other local branches.
  Cleanup failures retain the active manifest and allow retry after a worktree
  has already been removed.
- Retirement archives all existing state files with retired_at. Archive copies
  are written before active files move; earlier retirements of a reused id get
  preserved through a timestamp suffix. Lifecycle changes share the existing
  OS lock with spawn.
- Adopt derives state from a linked worktree, including detached HEAD, and
  renames/registers its current session. It never adopts a main checkout or a
  worktree already managed by another member. Session environment changes apply
  to new processes; the CLI explains how to export the id and restart an existing
  agent for reporting.
- Revive recreates only a missing session in an existing worktree and leaves it
  detached. Claude uses its latest recorded session id; empty history and the
  other agents start fresh, without a prompt. The installed Claude Code 2.1.286
  help confirms --resume accepts a session id. The integration test verifies the
  argument through a fake agent, including history beyond a single tail window.
- x opens a retirement dialog with dirty/ahead counts, explicit force and
  keep-branch toggles, confirm and cancel. r revives dead members. Both operations
  run outside the UI update loop and report errors in the overview.
- Current-session retirement is refused because killing the caller's pane could
  interrupt cleanup. The monitor, another session and outside terminals support
  retirement. This is an explicit MVP boundary, documented in the README.
- Tests cover refusal and force, archive content/timestamps/collisions, local and
  remote branch retention, locked worktrees, fallback cleanup and retry, changed
  worktree ownership, main checkout protection, adoption, resume/fresh starts,
  and actual terminal confirmation/revive actions on isolated tmux servers.

Local tests, vet, lint, all four static builds and snapshot packaging passed.
Implementation commit: `ac568bc`; release tag: `v0.4.0`. All nine
[implementation CI jobs passed](https://github.com/thomashartm/motley/actions/runs/36774661364),
including lifecycle and terminal tests on macOS/Linux and Go 1.22 on Linux.

Deliberately left out: GitHub open-PR warnings (W9), configurable protected
branches and batch cleanup (W10), restore-from-archive, automatic environment
injection into running agents, native Codex/OpenCode resume (W8), crews and
permission-mode selection.

## W5 — Crews and colours: complete

Delivered crew add/list/edit/remove/assign commands, spawn/adopt crew and colour
flags, and the overview's crew browser and editor.

- Crews live in schema-1 TOML, written atomically under the shared lifecycle
  lock. IDs use title slugs with collision suffixes; new crews take the next
  unused palette colour. URL shape determines text/issue/project/link kind.
- Member colour resolves from its override, then crew, then a stable id hash.
  Live sessions receive crew title, colour and emoji options, contrasting tmux
  status bars and emoji titles. Crew edits update all live members immediately;
  revive restores the same styling. User titles are escaped as literal tmux text.
- Agent badges and the eight-colour palette are fixed. Status icon colours stay
  independent of identity bars. No configuration settings were added: tmux
  colour/title switches and badge customization remain deferred.
- g cycles attention/crew/repo grouping. Crew rows are collapsed, with status
  counts and clickable titles; Tab focuses the member table. Selection survives
  status changes and polling. Members support the existing jump/retire/revive
  actions and e editing. Right/Space expands, Left collapses, H shows inactive
  crews. Narrow tables drop SINCE and then BRANCH; the PR column awaits W9.
- G manages crews: add/edit, cycle colour, delete with explicit force-unassign.
  e edits member name/ticket/crew/override. Forms use Tab, Ctrl-s save and Esc.
  Removal counts live and dead references; force retains individual overrides.
  Archived manifests keep their historical crew ids.
- Tests cover URL kinds, palette resolution, schema/defaults, id collisions,
  real tmux recolouring, override retention, force-unassign, adopt/revive styling,
  grouped selection, responsive layouts, terminal editing and monitor table jumps.

Local tests, vet, lint, all four static builds and GoReleaser snapshot packaging
passed. Implementation commit: `18a2faf`; release tag: `v0.5.0`. All nine
[implementation CI jobs passed](https://github.com/thomashartm/motley/actions/runs/36778336786),
including crew and terminal tests on macOS/Linux and Go 1.22 on Linux.

Deliberately left out: GitHub title fetching and crew suggestions (W9), PR table
columns (W9), configurable styling, blueprints (W6) and native permission modes.

## W6 — Blueprints: complete

Delivered global/repository blueprint discovery, TOML front matter, Go template
rendering, spawn blueprint/variable flags, and list/show/validate commands.

- Repository definitions override global names, then repo restrictions filter
  the effective set. Duplicate names within one directory and malformed files
  are errors. Schema defaults to 1; unknown front-matter fields are ignored.
- Available data is Repo/Branch/Base/Ticket/Name/Worktree, Crew fields and Vars.
  Missing variable keys render empty; variables are optional and repeated CLI
  assignments use the last value. Template errors are caught before worktree
  creation. The authoring validator renders with empty data.
- CLI agent selection overrides the blueprint; otherwise use the blueprint's
  agent, defaulting to Claude. Arguments belonging to another blueprint agent
  cause a clear refusal. No permission-mode picker was added.
- Prompt files carry a schema-1 Markdown comment and mode 0600. The comment is
  stripped before delivery; the rendered body is passed byte-for-byte. A 64 KiB
  limit keeps the single argument below platform limits. Manifests record the
  blueprint and arguments; revive keeps arguments without replaying the prompt.
- Verified installed CLI help: Claude 2.1.286 and Codex 0.159.2 accept positional
  prompts, OpenCode 1.18.21 accepts --prompt. W6 brings only native initial prompt
  delivery forward from W8, avoiding a fallback dependent on future idle hooks.
  Version/contract evidence is in internal/agents/testdata/prompt-contracts.md.
- Added a ready-to-copy plan-first example and blueprint name in TUI details.
  Tests cover rendering goldens, missing variables, precedence/filtering, schema
  validation, invalid-input refusal before worktree creation, and real tmux with
  fake agents recording exact argv for all three agents. Multiline prompts,
  leading hyphens and shell metacharacters remain literal; revive/archive tests
  verify no replay and preserved prompt bytes. No live model calls were made.

Local tests, vet, lint, all four static builds and GoReleaser snapshot packaging
passed. The packaged example also passed CLI validation. Implementation commit:
`815e318`; release tag: `v0.6.0`. All nine
[implementation CI jobs passed](https://github.com/thomashartm/motley/actions/runs/36781525991),
including blueprint handoff and lifecycle tests on macOS/Linux and Go 1.22.

Deliberately left out: TUI spawn form and prompt editor (W7), Issue template data
and issue fetching (W9), agent status/resume integration for Codex/OpenCode (W8),
and native permission-mode selection ([#1](https://github.com/thomashartm/motley/issues/1),
[#2](https://github.com/thomashartm/motley/issues/2)).

## W7 — Spawn and steer: implemented (Done)

Current status, 2026-10-01: implemented in `513be9a` and pushed to main.
The full local gate and [hosted CI](https://github.com/thomashartm/motley/actions/runs/36834127856)
passed. The roadmap ticket is Done based on implementation, as requested;
the `v0.7.0` release tag remains a separate delivery step. Earlier implementation
and validation notes below record the original checkpoint history.

Implemented the TUI spawn form, reply input, work-tab picker, filtering, and
`motley tabs` / `motley send`.

- Spawn steps: repo filter → ticket/name/editable branch → agent → compatible
  blueprint or none → variables → prompt preview/editor → launch with progress.
  Preview preparation is read-only; launch uses its exact reviewed prompt and
  arguments, rechecking worktree availability and member identity. Success
  focuses the new member. Failed creation retains the existing cleanup behavior.
- `$EDITOR` opens a private schema-1 temporary prompt file; it is removed after
  the editor returns. Edited text also works without a blueprint. A new optional
  manifest `prompt` boolean records this case; older blueprint manifests still
  load their prompt. Revive does not replay either kind of initial prompt.
- `/` performs case-insensitive subsequence matching over id/name/ticket/repo/
  branch and preserves attention order. The matcher is local: fetching the
  planned sahilm/fuzzy dependency was blocked by this session's network policy.
- `i` sends literal text and Enter to the active pane, rejecting permission,
  ended/dead states and control characters. `t` chooses another work client;
  send rechecks both endpoints and protects the monitor client.
- New coverage includes form validation/navigation, preview editing and focus,
  narrow layouts, filter/refresh behavior, read-only preparation, and reply/send
  checks with a fake tmux. Real tmux integration tests were added for reply/send
  and the full terminal spawn/editor flow.

Local verification passed: all internal package tests, CLI smoke/non-terminal
checks, form/filter/reply/send checks, go vet, golangci-lint, four static builds,
and GoReleaser snapshot packaging. Prepared-launch tests use real local Git with
a fake tmux to verify the reviewed prompt snapshot and manual-prompt metadata.
The local binary is available as `./bin/motley` (`0.7.0-dev`).

Full integration and publication are pending. This session blocks tmux socket creation
(`Operation not permitted`), network access, writes to Git metadata, and writes
outside the workspace/temp roots. W7 must not be marked complete or released
until the real tmux tests and hosted CI pass.

Continuation check: `go test ./...` was rerun on October 1, 2026. All internal
packages passed; CLI integration tests, including the new reply/send and terminal
spawn tests, failed during isolated tmux fixture startup before exercising their
assertions. They remain unverified, not skipped or accepted as passing.

To finish in a session that permits tmux sockets, Git metadata writes and network
access, run the full gate from the repository root:

```sh
make check GOLANGCI_LINT=.tools/golangci-lint-2.14.0-darwin-arm64/golangci-lint
make snapshot GORELEASER=.tools/goreleaser/goreleaser
```

Then finish the W7 commit, hosted CI, release tag and local installation. Do not
start W8 until this checkpoint is complete and the user has given feedback.

Deliberately left out: configurable branch templates, ranked fuzzy matching,
setup commands (W10), GitHub lookup (W9) and native permission-mode selectors.

## Next checkpoint

### Installer and test walkthrough

Added `install.sh`: downloads `main` with Git (or builds the current checkout with
`--local`), builds using Go 1.25.5, atomically installs to `~/.local/bin`, and
configures bash/zsh/fish PATH, the tmux popup and installed Claude's hooks.
Missing prerequisites use an existing supported package manager. Existing
configuration is preserved; shell/tmux edits are backed up and idempotent.
The README now includes download commands and a first-member test walkthrough.
`AGENTS.md` records the user's instruction to use Git/gh, never the GitHub connector.

Installer checks passed with disposable bash/zsh/fish homes, repeat installs,
failed builds and old tmux refusal. ShellCheck passed. A real source build/install
in a temporary home verified config, `ls`, Claude hook installation and binary
discovery by a fresh zsh. Package-manager execution and network downloading were
mocked; live tmux reload remains subject to the W7 integration gate above.
Installer tests run via `make test` and CI. These changes remain local; the README
download URLs become available after publication. Use `bash install.sh --local`
from this checkout in the meantime.

Finish W7 validation and release before starting W8. Feedback focus: spawn-form
flow, prompt preview/editor handoff, filtering and steering across work tabs.

Continue using the original [wt 1.2.0](reference/wt) and
[wt-clean 1.0.0](reference/wt-clean) alongside motley until W10 delivers the full
worktree tooling.

## Motley naming checkpoint — 2026-10-01

Renamed the product and Go module to `motley`; origin is
`https://github.com/thomashartm/motley`, verified with Git and `gh`.
The full command is `motley`, with `mtly` available from local builds, the
installer, and all four release archives.

- A running agent session is a member; members form a crew. A gig describes the
  crew's package of work. Crews have an optional `gig` field, editable through
  `crew add/edit --gig` and the TUI crew editor, shown in text/JSON crew lists,
  crew details and member details. Empty input clears a gig.
- Renamed command/package paths, manifests, environment variables, tmux options,
  hooks, configuration, blueprints, test fixtures, docs and release artifacts.
  Removed the obsolete branded logo and replaced generated local artifacts.
- This is a pre-v1 naming reset: configuration lives under `motley`, member
  manifests under `motley/members`, hooks use `MOTLEY_MEMBER`, and tmux uses
  `@motley_*`. There are no compatibility aliases for previous naming. Existing
  user configuration and running sessions were not modified; the README
  documents setup and adoption into the new namespace.
- Preserved the existing W7 and installer work while applying the rename;
  these changes are included together in the user-requested local commit.
  Push and release publication remain pending.

Validation passed: `make check` (installer tests, all Go tests including real
tmux/terminal integration, vet, lint and four cross-builds), ShellCheck,
`make build`, and the GoReleaser snapshot. Both command names report the same
version; the short command uses the same configuration. Installer tests cover
the short command across bash/zsh/fish and repeat/failed installs. Gig coverage
includes CLI add/edit/clear/invalid input, terminal editing and TUI display.
All four archives contain both commands. Source, filenames, binary contents and
archive contents were checked for stale product and member terminology.

This successful local full gate supersedes the earlier local tmux blocker.
Hosted CI, W7 publication and the next release remain pending.

### README logo — 2026-10-01

Added the supplied Motley logo to the README and release archives, preserving
the original image. Checked that the repository asset matches the supplied file
and that all four snapshot archives contain the exact logo and README reference.
GoReleaser snapshot packaging passed. The logo and naming commits are ready
for the user-requested push to `origin/main`; hosted CI and release publication
remain separate checks.

### README cleanup — 2026-10-01

Shortened the README to installation, everyday commands, essential keyboard
controls, configuration and development checks. Removed repeated walkthroughs
and implementation detail while retaining launch/retirement side effects,
agent reporting limits, adoption and pre-v1 setup instructions. Documentation
only; checked local links, command examples and diff whitespace.

### #15 — Arrow-key navigation — 2026-10-01

- Added a visible List → Details → Actions focus path. Actions exposes the
  existing member editor, crew manager, spawn, reply, tab and lifecycle actions.
  Left/Esc returns focus; existing letter shortcuts remain available.
- Crew Right expands first, then enters the member table; another Right opens
  Actions. Crew management has an arrow-accessible action menu. Up/Down moves
  through editor fields and Save/Cancel; Left/Right remains caret movement.
- Retirement and crew deletion expose selectable confirmation, force and cancel
  controls. Spawn fields support Up/Down; prompt preview has selectable launch,
  edit and cancel actions. Polling preserves focus and member identity.
- Updated the README controls. Added model coverage at 60×10 and a real terminal
  edit/save/cancel test; overview integration exercises arrows in standalone,
  popup and monitor modes, including monitor work-client routing.
- Kept this ticket on `feat/15-arrow-navigation`, separate from the pending W8
  changes. No mouse support, configurable keymaps or new configuration subsystem.

Validation: `make check` passed (installer tests, all Go tests including real
tmux/terminal integration, vet, lint and all four cross-builds). Focused model
tests also passed after the final crew focus/footer adjustment.
[Hosted CI](https://github.com/thomashartm/motley/actions/runs/36838983647) passed
for code commit `d3fe63f`, including Linux/macOS tests and release snapshots.
[PR #17](https://github.com/thomashartm/motley/pull/17) is ready for review.
Stop after this ticket for navigation feedback.

## Roadmap tickets — 2026-10-01

Created and read back one GitHub issue for every W0–W11 item. Done means
implemented: W0–W7 are closed with the completed reason; W8–W11 remain open.
W7 is implemented and CI-green; its release tag is tracked separately.

| Item | Ticket | Status |
| --- | --- | --- |
| W0 — Shell | [#3](https://github.com/thomashartm/motley/issues/3) | Done |
| W1 — First member, end to end | [#4](https://github.com/thomashartm/motley/issues/4) | Done |
| W2 — Overview TUI v0 | [#5](https://github.com/thomashartm/motley/issues/5) | Done |
| W3 — Attention states for Claude Code | [#6](https://github.com/thomashartm/motley/issues/6) | Done |
| W4 — Finish and resume members | [#7](https://github.com/thomashartm/motley/issues/7) | Done |
| W5 — Crews and colors | [#8](https://github.com/thomashartm/motley/issues/8) | Done |
| W6 — Blueprints | [#9](https://github.com/thomashartm/motley/issues/9) | Done |
| W7 — Spawn and steer from the TUI | [#10](https://github.com/thomashartm/motley/issues/10) | Done |
| W8 — Codex and OpenCode | [#11](https://github.com/thomashartm/motley/issues/11) | Done |
| W9 — GitHub on demand and links | [#12](https://github.com/thomashartm/motley/issues/12) | Open |
| W10 — Full worktree tooling; retire wt and wt-clean | [#13](https://github.com/thomashartm/motley/issues/13) | Open |
| W11 — Pop out and hardening | [#14](https://github.com/thomashartm/motley/issues/14) | Open |

Existing permission-mode tickets #1 and #2 remain open and separate.
Each roadmap ticket includes scope, acceptance criteria, and either completion
evidence or the remaining implementation work.

## W8 — Codex and OpenCode: implementation in progress

Implemented locally for roadmap ticket #11:

- Codex native hook parsing and idempotent `hooks install codex`, with backups
  and preservation of existing hooks/config. Installed CLI 0.159.2 reports native
  hooks enabled; Motley uses those instead of legacy notify or screen heuristics.
  The user must review/trust the hooks in Codex's `/hooks` UI. Explicitly disabled
  hooks and approval policies are preserved. Codex runs with `--no-daemon` so each
  member's reporting environment belongs to its own process.
- OpenCode plugin installation and event mapping, verified against CLI 1.18.21,
  its embedded event definitions and official plugin documentation. Plugin
  callbacks return immediately; bounded background reporting is serialized,
  excludes child sessions, preserves pending approvals across busy events, and
  supplies the last assistant text excerpt at idle. Reporter errors are contained.
- Native resume for Codex and OpenCode using the latest recorded session id,
  preserving stored options and never replaying the initial prompt. Fresh start
  remains the fallback when no id was recorded.
- The installer configures all installed agents; CLI help and concise README
  usage cover reporting and resume. No new runtime dependency for Motley itself;
  Node 24 runs the OpenCode plugin test in development and CI.

Validation passed: internal Go package tests, CLI smoke/non-terminal checks,
real process replacement into fake Codex/OpenCode executables (prompt, fresh
start and resume), plugin subprocess/ordering/child-session tests, installer
tests, ShellCheck, vet, lint, four static platform builds and snapshot packaging.
The installed Codex binary's feature detection and both hook installers were
also exercised in a disposable home, without touching actual agent settings.

The full `go test ./...` run remains blocked at isolated tmux fixture startup
in this session, including the new native-reporting integration tests. Payload
fixtures are synthetic contract examples; live agent-turn recordings, real
tmux reporting/resume acceptance and hosted CI remain outstanding. No model
calls, hook-trust changes, commits, pushes or release tags were made in this run.
Git/gh network access is unavailable and Git metadata is read-only here.

See `internal/agents/testdata/w8-contracts.md` for version evidence, sources and
fixture provenance. Preview: `./bin/mtly` (`0.8.0-dev`). W8 is not yet released or
marked Done; finish its remaining validation and publication before W9.
Deferred: legacy Codex notify/capture compatibility and permission-mode selectors
(the existing separate tickets #1 and #2).

### Installer checklist and history — 2026-10-01

Installer terminal output now consists of concise bullets with green success
and red failure marks. Full command stdout/stderr, hook backup paths, restart
instructions and exit status are saved in a private timestamped log under
`~/.motley/install-history/`; the checklist prints the log path. Repeat runs
keep separate logs. Build failures retain their exit code and leave installed
files intact; individual hook failures remain nonfatal and show a red mark.
Codex still requires restart and explicit hook review/trust.

Installer tests passed for bash/zsh/fish, repeat installs, output separation,
history preservation, build failure, prerequisite refusal, hook failure and
download mode. ShellCheck and diff whitespace checks passed. Other in-progress
agent integration changes were preserved; no commit or push was performed.

Created and verified [#16 — Add an uninstall script](https://github.com/thomashartm/motley/issues/16)
as Open. It covers installation/integration removal, preserved user work and
settings, repeat runs, concise status output and detailed history. The uninstall
script itself is not implemented in this checkpoint.


### Uninstall and release updates — 2026-10-01

Implemented #16 as `uninstall.sh`, a standalone Bash/Python 3 script. It removes
installed Motley commands and owned agent hooks/plugins, disconnects tmux popup
includes, and preserves user settings, shared shell PATH, saved state, worktrees,
branches and running sessions. File identity is checked before any removal;
modified plugins, unrelated binaries/aliases and invalid settings are refused.
Changed settings and removed plugins receive exact backups. Checklist output and
private timestamped logs mirror installation, under `~/.motley/uninstall-history`.
Repeat/partial installs are handled without deleting unrelated files. The script
is included in all four release archives; uninstall tests run in Make and CI.

Added `motley update` / `mtly update`, with `--check` for discovery only. The
command uses `gh` against `thomashartm/motley`, selects the current OS/architecture
archive from the latest published release, verifies SHA-256 against checksums.txt,
and stages both commands in their installation directory. It verifies Motley
binary identity/platform, preserves an existing mtly symlink, refuses unrelated
files, skips current/newer stable versions and rolls back earlier replacements
if a later rename fails. Update and uninstall share an installation lock.
Configuration, worktrees and running sessions are not changed. Full update
details are logged under `~/.motley/update-history`.

Validation: full local `make check` passed (installer, uninstall, plugin, all Go
and real-tmux tests, vet, lint, four platform builds). After adding the shared
uninstall lock, all ten uninstall tests and focused update tests passed again.
ShellCheck, build, diff checks and GoReleaser snapshot passed; archive readback
verified both commands and the exact uninstall script on all four platforms.
Tests cover mixed user hooks, shell configuration preservation, invalid/replaced
files, repeat runs, concurrent operations, checksums and failed-update rollback.
A live read-only `mtly update --check` confirmed there are currently no published
GitHub releases; real release installation remains pending the first publication.
No actual user installation was removed or updated. #16 is Done based on
implementation. The user requested committing and pushing this checkpoint,
including the pending Codex/OpenCode integrations, installer checklist/history,
roadmap tracking, uninstall script and release updater. Hosted CI and release
publication are separate from the local validation above.

### Agent navigation guide — 2026-10-01

Added a short README guide for the tmux prefix, Motley popup/details, returning
to an agent, window/pane navigation, scrollback and detach. Documented opening a
Ghostty tab and attaching an existing member, including how to move the view and
the distinction between creating a terminal tab and sending to an attached tab.
Checked commands against the CLI, popup binding and TUI handlers; checked tmux
bindings and Ghostty's installed default keybindings. Documentation only.

### Persistent shortcuts and mouse navigation — 2026-10-01

Member sessions now show three help rows below their existing tmux status row,
using the configured prefix and the current member's attach command. Spawn,
revive, adopt and attachment install the footer. Prefix H toggles the originating
client between the persistent monitor and its previous session; prefix h retains
the popup. Underlined footer controls support mouse clicks for monitor, details,
detach, windows, previous/next window and scrollback. Clicks dispatch on release
to avoid tmux's rapid-click key translation. Existing key actions remain the
fallback outside Motley controls.

The overview supports clickable List/Details/Actions/Enter/q controls, member
and crew selection, action selection and wheel scrolling. Forms retain keyboard
input. Navigation callbacks target the originating client and its active session;
no navigation command is typed into the agent. README documents the controls.

Validation passed: installer, uninstall and plugin checks; full Go suite; vet;
lint; four platform builds; whitespace check. Real isolated tmux tests cover a
custom prefix, existing status row and key fallback, reattachment, keyboard and
mouse monitor round trips, rapid window/scroll clicks and non-destructive detach.
TUI tests cover navigation buttons, member/crew/action clicks, scrolling, modal
guards and minimum-size handling. Existing monitors must be restarted to load
the updated TUI; agent processes need not be restarted for the tmux footer.

### Local navigation rollout and tmux compatibility — 2026-10-01

The installed binary was still `local-a2ac75b`, explaining the popup-only shortcut
and missing arrow navigation. Hosted macOS CI also exposed tmux 3.7c's single-key
`list-keys` output going to the status line. Shortcut setup now reads the whole
key table and extracts the original binding. Focused parser, real-tmux footer and
overview/monitor tests passed before updating the local installation.
Installed `local-76fc7f4` with `install.sh --local` (all checklist steps passed),
closed the old popup and switched the existing client to `_motley`. Readback
confirmed the full monitor, clickable navigation, enabled mouse support and
preserved `feat-cleanup-tasks` session. The updated PR's hosted checks are pending.

### #15 follow-up — Grouped footer — 2026-10-01

- Grouped footer shortcuts by navigation, actions, view and session operations.
  Whole groups wrap into at most two reserved rows; small terminals use compact
  navigation and confirm/back controls. Extra actions stay in the Actions menu.
- Accounted for footer height in panels and prompt scrolling, retaining the
  60×10 minimum. Footer text leaves room before the terminal's right edge.
- Added resize checks for every footer context, from 60×10 to 160×30, including
  whole-view bounds and essential controls. Existing navigation bindings remain.

Validation results are recorded on the follow-up PR. This is a footer correction
only; the next delivery item waits for feedback.

### PR #18 merge with grouped footer — 2026-10-01

Preserved both delivery histories and combined clickable navigation with the
grouped footer. Navigation buttons occupy the last reserved footer row; short
terminals show compact hints or just the buttons. Mouse hit testing uses the
same content height as rendering, so footer clicks cannot select hidden rows.
Validation results are recorded on PR #18.

### Open agents directly from the monitor — 2026-10-01

Added a persistent Open agent button and o shortcut, plus Open agent as the first
Actions item. A monitor with no separate work client opens the selected member
in its own tab; another attached work tab remains preferred and a missing pin
still refuses rather than redirecting. The agent footer labels its clickable
return control Back to monitor. No shell command or tmux prefix is needed for
the normal open/return route.

Implemented in an isolated worktree, leaving the other session's checkout
untouched. Rebased onto its merged footer work, preserving the grouped layout
and bottom-row mouse controls.

Validation passed: full Go suite, vet, lint, four platform builds and whitespace
checks. Real tmux tests cover opening with Enter and clicking Open agent, then
clicking Back to monitor, with no separate work tab. Tests also retain the
separate/pinned work-tab behavior and guard ambiguous monitor clients.

### Selected member control and independent monitor — 2026-10-01

The Actions panel names its selected member. Terminate agent (X) confirms the
target and stops its owned tmux session while keeping the worktree, branch and
history for Revive. Confirmation supports mouse clicks and arrow keys. Retire's
confirmation, Force, Keep branch and Cancel controls also accept mouse clicks.
Lifecycle completion refreshes the member list immediately.

Running bare Motley inside an agent now switches to the independent monitor,
preventing the self-retirement refusal caused by an overview running inside its
target session. Ownership checks and explicit force for discarding work remain.

Validation passed: full Go suite, vet, lint, four cross-builds and whitespace
checks. A real tmux regression starts the overview inside its target agent,
terminates it without losing dirty files, revives it, then force-retires it
while the monitor stays alive. UI tests cover mouse targeting, confirmation,
cancellation, changed row order and resized footers. Other session's checkout
was left untouched.

### Visible panel shortcuts — 2026-10-01

The bottom bar now labels direct keys: [o Open agent] [1 List] [2 Details]
[3 Actions] [q Close]. Number keys focus their panel without a tmux prefix;
Actions says Enter runs the selected command. Text editors retain numeric input.

Full tests, vet, lint, cross-builds and whitespace checks passed. Real tmux tests
click all three panel buttons and exercise their number keys. The user's live
mouse failure is not yet reproduced: mouse reporting was enabled, but the client
detached before the requested click trace. Temporary tracing was removed and
the original MouseDown1Pane binding restored. Do not consider that report fixed
based only on automated mouse tests.

### Import running Claude sessions — 2026-10-01

Added `mtly import --list`, `mtly import <session-id>` and the monitor's Add
existing Claude action (a). Discovery uses Claude's supported `agents --json`
interface and rechecks the selected conversation before registration. Imported
sessions remain running in their original terminal, including sessions in main
checkouts or non-Git directories. Their live status appears in the monitor.

Open agent focused a unique matching Ghostty directory through AppleScript. That
broke the "No AppleScript" principle and was removed before landing on main (see
the #30 entry).
Terminate revalidates the Claude session/process, then waits for it to stop.
Revive resumes the saved conversation under Motley. Imported directories and
branches are always preserved on retirement, including after revival.

Validation: full Go suite and vet passed; focused import tests passed again
after error-message lint fixes; lint and four cross-builds passed. Tests use
isolated tmux servers and harmless fixture processes for main, linked and
non-Git directories, discovery failure, duplicates, resumed conversation IDs,
file/branch preservation and the real terminal picker. Read-only discovery
found six local running sessions. Ghostty scripting compiled and its directory
inventory was verified; no live Claude sessions were imported or interrupted.

### First-run configuration — 2026-10-01

Configuration now lives at ~/.motley/config.toml. The first config load or
`mtly init` creates it with defaults, or copies an existing XDG config verbatim.
Existing files, including invalid TOML, are never reset or overwritten. A
complete temporary file is linked into place atomically so concurrent first
launches cannot expose partial content or overwrite each other. Tmux snippets
and blueprint paths retain their current locations.

Validation passed: full Go suite, vet, lint, four cross-builds and whitespace
checks. Tests cover defaults, legacy settings and comments, concurrent first
loads, unchanged modification times, invalid-file preservation and CLI loading
from the new location.

### Monitor shortcut — 2026-10-01

Changed the monitor toggle to prefix m (Ctrl-b m with the default prefix).
Updated the member footer, monitor return hint and README. Prefix h still opens
Details. The real-tmux shortcut test now exercises m in both directions with
custom/default prefixes and preserves the user's m binding outside Motley.

Validation: binding parser and real-tmux shortcut integration tests passed.

### Claude permission modes (#1) — 2026-10-01

`motley spawn --mode <preset>` and a Permission mode step in the spawn form
(after Blueprint, Claude only) select manual, acceptEdits, plan, auto, dontAsk,
bypassPermissions or sandbox. Omitting the mode keeps today's bare `claude`, so
Claude's configured default still applies; there is no CLI menu or TTY
requirement. Presets are hard-coded argv in `internal/agents/modes.go`, resolved
in Prepare before any worktree, branch or manifest exists. Unknown presets, a
non-Claude agent, or a blueprint that already sets the same option (or
`--dangerously-skip-permissions`) are refused. The manifest records `mode` and the
resolved `agent_args`; revive reapplies them. Old manifests are unchanged.

Verified against Claude Code 2.1.286: `--permission-mode` lists manual (the
issue's "default" is now a hidden alias, so Motley does not offer it). The
script's sandbox block was not ported: `mode: autoAllow` is not a setting, and
`denyWrite ["~/"]` would override the worktree under `~/worktrees`. The sandbox
preset is `--permission-mode acceptEdits --settings
'{"sandbox":{"enabled":true,"failIfUnavailable":true,"autoAllowBashIfSandboxed":true}}'`,
so no file is written to the worktree.

macOS confinement was checked with real Claude (haiku, `-p`) in a linked
worktree under `$HOME`, and confirmed on disk: worktree writes and `git commit`
succeeded; writes to `$HOME`, the main checkout, `.git/hooks` and `.git/config`
were denied. Linux verification with real Claude is deferred to #26.

Validation: preset/conflict unit tests, TUI mode-step tests, real-tmux tests
with a fake Claude recording argv/CWD (spawn, revive, blueprint combination,
refusals leave no worktree/branch/manifest), the terminal spawn-form test,
full suite on macOS, cmd/motley ×2 on an Ubuntu 24.04/tmux 3.4 replica,
golangci-lint v2.14.0 (0 issues), vet, gofmt and four cross-builds.

### W8 live acceptance (#11) — 2026-10-02

Ran real Codex 0.159.3 and OpenCode 1.18.21 under Motley in a disposable home
with its own tmux server; credentials were copied in, unchanged by the run and
deleted afterwards. Both agents reported working, permission, question, ready,
interrupt and quit, and `motley revive` resumed the recorded session with its
context. Recorded payloads are now `internal/agents/{codex,opencode}/testdata/live.json`,
replayed by the parser tests and through real tmux; their expectations matched
every event Motley logged live.

Fixed from the run: OpenCode reports nothing on `/exit`, and no agent reports a
crash, so the member kept its last status (for example `ready`) with a shell in
the pane, where a reply would have run as a command. The pane now runs the
hidden `motley agent-exited` after the agent returns, recording `ended` once.
OpenCode's idle events after an abort showed the partial reply as finished; the
plugin marks them interrupted, matching Codex's `idle` / "Turn interrupted".
Uninstall recognises both shipped plugin revisions.

Known limitations, without a native signal: a bare or revived Codex/OpenCode
member shows `starting` until the first prompt; Codex asks questions only in Plan
mode, whose Stop has no message; OpenCode logs two Stop events per turn. Details:
`internal/agents/testdata/w8-contracts.md`. W8 is Done; the legacy Codex fallback
is #27.

Validation: `make check` stages (installer, uninstall, plugin, all Go and real
tmux tests, vet), golangci-lint v2.14.0 (0 issues), gofmt and four cross-builds.
The new exit test fails with the pane change reverted, and the plugin test fails
with the interrupt marking disabled.

### Imported sessions: no terminal control outside tmux (#30) — 2026-10-02

PRs #22 (import) and #23 (first-run config) had been merged into stacked branches
and never reached main; this change merges them. #22 focused the original Ghostty
tab through AppleScript and failed with "Cannot identify a unique Ghostty tab"
whenever several tabs shared the session's directory. AppleScript contradicts
REQUIREMENTS §0 ("No AppleScript or Ghostty API"), so the focus is removed rather
than repaired. A tty-title variant built in PR #32 was removed before merge for
the same reason.

Open agent, `attach` and `switch` on a running imported member now explain where
it runs: "<name> runs in its original terminal in <dir>; switch to it there, or
Terminate and Revive to run it in Motley." Reply and send messages no longer point
to Open agent. Revive remains the tmux-only way to take a session over.

Validation: `TestNoAppleScript` scans every Go, shell and TypeScript source and
fails on AppleScript use; it caught the removed package when restored. The import
lifecycle test checks the attach/switch explanation for main, linked and non-Git
sessions. The Claude discovery contract test now uses a generous bound through a
test seam; it had hit the 3 s production bound at a load average near 20.

### Wider member list — 2026-10-02

The list used a third of the window, capped at 42 columns, so details took about
80% of wide windows (200 columns: 42 | 153). The panels now split evenly and the
list stops at 100 columns: 80 → 40 | 35, 120 → 60 | 55, 200 → 100 | 95,
240 → 100 | 135. `TestPanelSplit` checks those widths and that no rendered line
exceeds the window; the full suite, including the real-terminal monitor tests,
passed unchanged.


The message line above the footer is cut to the window width and the monitor owns
the mouse, so messages such as errors could not be copied. **c**, a click on the
line, or **Copy message** (last in Actions, shown only while a message is present)
copies the full text: the poll error when there is one, otherwise the message.
Busy and filter text are not messages. The line shows **[c copy]**, then
**✓ copied** until the message changes; failures replace it with "Copy failed: …".

Inside tmux, `tmux load-buffer -w` stores a paste buffer and forwards it to the
client terminal as OSC 52. The monitor targets its most recently active tab, the
same rule as detach (now one helper). With `set-clipboard off` tmux cannot forward,
and Motley says so rather than reporting success. Outside tmux Motley writes OSC 52
itself. No AppleScript, `pbcopy` or terminal-specific API.

Validation: TUI tests cover the full untruncated text, poll-error preference,
busy/filter exclusion, no message, key, click, Actions entry, failure, monitor tab
choice and the exact OSC 52 bytes. A real-binary test runs the overview in an
isolated tmux pane with a pty client: the client receives OSC 52 with the payload,
`show-buffer` matches and `set-clipboard off` reports the paste-buffer-only result.
Four mutations (no `-w`, no off check, truncated copy, wrong monitor tab) each fail.

### Actions panel grouping and consequence help — 2026-10-02

Grouped actions under Member, Crews & members, Session & cleanup, and View,
with headings and blank lines between groups. Keyboard selection and mouse
hover update a highlighted explanation at the bottom of the panel. Terminate
and Retire use caution styling and explain confirmation, file/branch retention
or removal, and the effect of Force; Revive explains immediate restart and
its requirement for an existing worktree.

Enabled mouse motion without holding a button. Hover never executes an action;
click and Enter retain existing dispatch and confirmation behavior. Rendering
and mouse hit testing share the grouped rows, excluding headings, gaps and the
help box. Scrolling retains stable pointer targets. Small terminals use compact,
truncated help while keeping the selected action reachable.

Validation: TUI tests cover grouping, consequences, hover versus click, scrolling,
resize, empty selection, footer clicks and 60x10 bounds. The real-tmux overview
test verifies no-button hover and the resulting explanation. Installer,
uninstaller, plugin and all Go tests passed, as did vet, golangci-lint 2.14.0
(0 issues), formatting and four cross-builds. The linter was absent from PATH;
the pinned release was run from a temporary directory after make check reached
that missing-tool step. No manual visual acceptance is claimed.

Follow-up: removed the help heading. The box now fits only its explanation,
with no vertical padding and one character of horizontal padding. Reserved
space stays above the box so its bottom edge remains flush with the panel
interior without moving menu rows on hover. Updated layout assertions and
all TUI tests passed; rebuilt `bin/motley`.

### Panel dividers, Revive validation and lowercase shortcuts — 2026-10-02

Panel headings now have a dotted divider and a blank row beneath them. The
minimum-height layout remains compact to preserve controls. Mouse coordinates
account for the added rows, and the Actions explanation remains at the bottom.
Its enclosing box and heading are gone: a dotted divider above the highlighted
text matches the panel heading style, with no side or bottom borders.

Revive was using the pre-removal worktree validator, so a member recorded in the
main checkout failed with “refusing to remove the main worktree.” It now checks
registered checkout identity without applying deletion restrictions. Both main
and linked checkouts must match the recorded branch. Retirement still uses the
strict removal check and refuses the main checkout, including with Force.

Replaced case-only shortcut distinctions: d terminates, x retires, r revives,
c manages crews, g changes grouping, p pins a work tab, t sends to a tab, and
h toggles inactive crews. Menus, footer hints, README and terminal tests use the
new bindings.

Targeted validation passed: all three Revive input routes, real-tmux lifecycle,
crew and overview tests, main-checkout restart with saved session and preserved
files/HEAD/worktree registrations, and unchanged main-checkout retirement
refusal. Layout assertions cover dotted headings, spacing, borderless help,
bottom anchoring and compact terminal bounds.

Final validation: full `make check` passed with the pinned golangci-lint 2.14.0
binary: installer/uninstaller/plugin tests, all Go and real-tmux tests, vet,
lint (0 issues), and four cross-builds. Rebuilt the local `bin/motley` executable.

### Edit panel field and action styling — 2026-10-02

Editor labels are bold with trailing colons, above indented values; empty
fields show a muted placeholder. Fields have blank rows between them. Actions
use clearly bracketed, coloured buttons with a filled focus highlight and
spacing between controls. Save/Send/Delete and Cancel remain at the bottom,
while the fields scroll to keep the active label and value visible.

Clicking a label or value focuses that field. Buttons and the Force unassign
toggle are clickable; blank rows and hints do not activate actions, and a
pending save blocks further clicks. Crew deletion uses the same styling.

Validation: all TUI tests, real-terminal crew editing and overview tests, vet,
golangci-lint (0 issues) and build passed. The crew terminal test now saves
through an actual mouse event as well as Ctrl-s. Layout/mouse tests cover
field selection, Save/Cancel/Send/Delete, the force toggle, pending writes,
spacing, resize and 60x10 bounds. Rebuilt `bin/motley`.

### Colour selector in editors — 2026-10-02

Member and crew colour fields now render a swatch/name selector with visible
left/right arrows. Arrow keys and mouse clicks cycle through the shared palette
and wrap through Inherit (member) or Automatic (crew). The default choice stores
an empty override, preserving the existing inheritance behaviour. Existing
colours initialise the selector; text entry, paste and deletion do not mutate
it. Tab/up/down/Enter retain field navigation, and colours apply on Save.

Validation: all TUI tests, real-terminal crew/member saves using arrow-selected
blue/yellow colours, vet, lint (0 issues), formatting and build passed. Selector
tests cover every palette entry, wrapping, inherited/automatic defaults, saved
values, mouse arrows, rejection of text input and the minimum terminal size.
Rebuilt `bin/motley`.

### PR integration checkpoint — 2026-10-02

Rebased the UI and Revive changes onto current main, preserving Claude import,
clipboard copying and the wider panel split. Manage crews now uses m so c keeps
its existing Copy message action. Imported-member help explains that retirement
keeps files, and that Open agent reports the original terminal location. Import
picker mouse targets account for heading dividers. Compact Delete/Cancel spacing
fits the narrower detail panel, and terminal mouse tests use the new split.

The prior validation entries above describe the earlier branch snapshots;
validation of this integrated revision is recorded below.

Integrated revision validation: all Go tests passed, including the updated
terminal Save/Cancel assertions, real mouse/colour editing, clipboard and import
flows, and the main-checkout Revive regression. Installer and uninstaller tests,
vet, golangci-lint (0 issues), and four cross-builds passed. The unchanged
OpenCode plugin reporter test missed its first event on the initial make check
and a focused rerun; the baseline copy and a subsequent branch rerun passed.
No plugin implementation or test was changed for that intermittent failure.

### Crew selector in member editor — 2026-10-02

The Crew field now shares the colour selector's keyboard and mouse pattern.
Left/right arrows cycle through existing crew names with colour markers and
No crew, while saving the corresponding ID automatically. Selection stays tied
to its ID when the crew list refreshes. Empty lists, unavailable assignments,
long names and compact terminals are handled without accepting typed IDs.
Changes apply on Save; Cancel leaves the member unchanged.

Validation: all TUI tests, real-terminal crew/member editing and arrow navigation,
TUI vet, golangci-lint (0 issues), formatting and build passed. Tests cover
keyboard/mouse selection, wrapping, refreshed lists, empty/unavailable crews,
long Unicode names, cancellation and 60x10 bounds. Rebuilt `bin/motley`.

### Overview actions, grouped member table and details — 2026-10-02

Status sections now group members by crew and render aligned title, ticket and
crew columns with the existing status, agent and colour indicators. Overview
stays pinned above the list and is selectable by mouse, Home or Up from the first
entry. Main actions are separate from member actions; opening an agent from
Overview uses an explicit session picker, while Spawn, Add and Manage crews need
no member selection. Polling preserves Overview and picker targets by identity.
Rendering and mouse hit testing share the list layout, including decorative
headings, scrolling and compact terminals.

Ticket hyperlinks accept explicit web URLs and resolve numeric GitHub/GitLab.com
tickets using the member's remote. Details now have bold labels, aligned and
wrapped values, consistent field gaps and Workspace/Session sections with dotted
dividers. Narrow panels stack labels above indented values, matching the editor.

Validation: all Go tests passed, including real-terminal Home/Open picker,
editor navigation, monitor switching and popup operation. Model coverage includes
grouping, selection and polling, action scope, keyboard/mouse opening,
removed/stopped targets, ticket links, Unicode wrapping and 60x10 bounds.
Vet, golangci-lint (0 issues), build and diff checks passed. Rebuilt `bin/motley`.

### Ticket clicks and tmux mouse/prefix — 2026-10-02

Left-clicking a visible ticket or crew hyperlink now opens it through the system
browser, including in tmux panes and popups. Hit testing follows the rendered
cells, including scrolling and Unicode. Wrapped detail hyperlinks close at each
line boundary so they cannot extend into adjacent labels or the other panel.
Hover/release and background links behind editors do not open a browser; opener
failures appear in the message line. Web URLs are passed as individual process
arguments without shell interpolation.

New tmux configuration enables mouse support, uses Ctrl-a as its prefix and
binds Ctrl-a Ctrl-a to send-prefix. Native xterm-family hyperlinks are enabled
when tmux is 3.4 or newer; plain TUI clicks also work without native hyperlink
support. Existing user config remains preserved by init, with an upgrade snippet
in the README.

Validation: all Go tests passed, including real tmux pane/popup ticket clicks
with a stub browser, ordinary mouse navigation, Ctrl-a popup/detach and config
reload checks. Vet, golangci-lint (0 issues), build and diff checks passed.
Applied the interaction settings to the local tmux config after backing it up;
server readback confirms Ctrl-a and mouse on across existing sessions.

### Compact Details rows — 2026-10-02

Removed the empty row between each Details field. Labels and wrapped values keep
their alignment; dotted headings and spacing between sections remain intact.

Validation: TUI tests, targeted real-terminal overview/monitor and mouse ticket
link tests, TUI vet, build and diff checks passed.

### Crew wordmark — 2026-10-02

The TUI header pairs three crew bars in cyan, violet and amber with a bold
uppercase MOTLEY wordmark. Both overview and monitor keep a single header row.

Validation: TUI tests, real-terminal overview/monitor checks, TUI vet, build and
diff checks passed.
