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

## W7 — Spawn and steer: implementation in progress

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
