# Delivery checkpoints

Document schema: `1`

colony is built one usable slice at a time. Scope and acceptance criteria live in
[REQUIREMENTS.md](REQUIREMENTS.md#12-delivery-plan-thin-vertical-slices).

After each item, stop, summarize what was built and left out, report validation
and open questions, and wait for user feedback before starting the next item.
Feedback may change the scope or order of later items. Keep milestone status and
implementation-process notes here; the README describes how to use the tool.

## W0 — Shell: complete

Delivered the Go CLI shell, configuration loading for repository and worktree
roots, `colony version`, help, executable smoke tests, CI and GoReleaser snapshots.

- Commit: `e4492c8`; release tag: `v0.0.0`.
- All nine [release-tag CI jobs passed](https://github.com/thomashartm/colony/actions/runs/36705427765):
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

## W1 — First minion: complete

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
  All nine [implementation CI jobs passed](https://github.com/thomashartm/colony/actions/runs/36716731133),
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
- `colony init` scaffolds the current three-key config and popup binding, without
  overwriting either file. Generated tmux config carries a schema comment.
- `colony monitor` creates or reconnects to `_colony`. Enter targets another
  client; T pins a work tab or restores automatic selection. q detaches the
  monitor client, keeping the overview alive. A missing work tab gets a hint.
- `colony config` retains the former no-argument config display now that the root
  command opens the TUI. The overview needs at least a 60 × 10 terminal.
- Model tests cover selection, ordering, removals, dead-session refusal, monitor
  targeting and pinning, and narrow layouts. Real pseudoterminal tests exercise
  normal and popup jumps, outside-tmux attach, monitor reuse, pinning, detach,
  and alive-to-dead refresh on an isolated tmux server.
- Implementation: `9f5d5a6`, compatibility fix: `ec61c54`, final test adjustment:
  `ba91a73`; release tag: `v0.2.0`.
- Local tests, vet, lint, darwin/linux × amd64/arm64 builds, and GoReleaser
  snapshot packaging passed. All nine
  [implementation CI jobs passed](https://github.com/thomashartm/colony/actions/runs/36733809284),
  including macOS, Linux and Go 1.22. The terminal integration also passed locally
  on Linux arm64 with Go 1.22 and tmux 3.3a in a disposable container.
- Linux compatibility testing found that older tmux versions sanitize tab
  separators without UTF-8 mode. Session/client reads now explicitly use `-u`;
  the standalone terminal test also runs with the C locale. Popup tests wait
  for tmux to acknowledge the prefix key before sending h.

Deliberately left out: attention states/hooks, spawn forms, crews, permission-mode
selection and other later-item features. Native Claude and Codex permission-mode
requests remain tracked in [#1](https://github.com/thomashartm/colony/issues/1) and
[#2](https://github.com/thomashartm/colony/issues/2).

## W3 — Claude attention states: complete

Delivered Claude hook reporting, attention sections and details, monitor counts,
new-attention markers and optional terminal bell (`monitor_bell = true`).

- `colony hooks install claude` merges seven hooks idempotently and backs up an
  existing settings file. Global user hooks are installed only on explicit use
  of that command; implementation testing used temporary settings.
- Recorded real Claude Code 2.1.285 lifecycle, Read, Bash, question, permission
  and idle payloads. Fixtures include version and reproduction notes. Stop's
  direct assistant message avoids a transcript flush delay observed in testing;
  older payloads use a bounded transcript-tail fallback.
- `report` always exits 0 and emits no stdout/stderr. It ignores sessions without
  COLONY_MINION, caps payloads and event lines, bounds stdin/tmux waits at 250 ms,
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
  every host. Reproduce with `go test ./cmd/colony -run '^$' -bench
  '^BenchmarkReportCLI$' -benchtime=300x`.

Local tests, vet, lint, all four static builds and snapshot packaging passed.
Implementation commit: `a6dee0a`; release tag: `v0.3.0`. All nine
[implementation CI jobs passed](https://github.com/thomashartm/colony/actions/runs/36742146219),
including real terminal/report tests on macOS/Linux and Go 1.22 on Linux.

Deliberately left out: event rotation and stale detection (W11), Codex/OpenCode
reporting (W8), retire/revive (W4), crews, spawn forms and native permission modes
([#1](https://github.com/thomashartm/colony/issues/1),
[#2](https://github.com/thomashartm/colony/issues/2)).

## Next checkpoint

Stop after W3. Feedback focus: whether attention ordering, question/permission
context and monitor alerts fit daily Claude use. W4 waits for user feedback.

Continue using the original [wt 1.2.0](reference/wt) and
[wt-clean 1.0.0](reference/wt-clean) alongside colony until W10 delivers the full
worktree tooling.
