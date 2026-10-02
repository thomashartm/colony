# W8 integration contracts

Interfaces verified locally on 2026-10-01; live acceptance on 2026-10-01/02.
Each agent has two fixtures: `events.json` holds synthetic edge cases derived
from the interfaces below (compaction start, unknown events, a transcript path
that must not be read); `live.json` holds payloads recorded from real agent turns
under Motley, with the run's temporary directory shortened to `/w8`. Every live
case's expected event, status and summary matched what Motley logged during the
run (41 of 41 logged events cross-checked; unlogged ones repeat a status).

## Codex

Installed `codex --version`: `codex-cli 0.159.2` (interfaces); live run 0.159.3.
`codex features list` reports `hooks stable true`.
`codex resume --help` reports `codex resume [OPTIONS] [SESSION_ID] [PROMPT]`.
Motley passes stored options, `--`, then the recorded session id; no initial
prompt is replayed. Startup prompt delivery remains the W6 positional argument.
Both paths use the installed CLI's `--no-daemon` option to keep the hook process
environment local to the member, including `MOTLEY_MEMBER`, rather than relying
on environment inheritance from a shared daemon.

[Official hook documentation](https://learn.chatgpt.com/docs/hooks) specifies
`hooks.json` beside the user config, stdin JSON, `hook_event_name`, `session_id`,
`tool_name`, `tool_input`, and `last_assistant_message`. PermissionRequest is the
approval event. Native hooks cover lifecycle and tool status, so this slice uses
them instead of legacy notify or capture-pane guessing. Hooks require review in
Codex's `/hooks` UI; Motley does not alter trust or approval policy.

`codex/testdata/events.json` covers those fields, unknown events, interruptions,
and the question tool's options; `codex/testdata/live.json` is the recorded
session. Codex transcripts are not Claude transcripts; the parser uses Stop's
supplied message and does not read transcript files.

## OpenCode

Installed `opencode --version`: `1.18.21`. Its help lists `--prompt` and
`--session` (`-s`). Motley passes both with `=` to preserve literal arguments.

[Official plugin documentation](https://opencode.ai/docs/plugins/) describes
the global `opencode/plugins` directory, event callback and permission/session
events. The installed executable's embedded JavaScript also contains
`permission.asked`, `question.asked`, `session.status`, `session.idle`, and
the `chat.message` hook. SDK declarations installed locally at version 1.14.48
corroborate `properties.sessionID`, `properties.info.id/parentID`, question
options, permission patterns, message parts and the plugin client's session API.
They are supporting evidence from an older SDK, not claimed to be 1.18.21 types.

`opencode/testdata/events.json` covers native events and the plugin's small
`chat.message` envelope and `lastAssistantMessage` enrichment; `live.json` holds
the recorded sessions, including the plugin's `interrupted` marking. The plugin
keeps only a bounded last text excerpt; no transcript or model API call is used.
Its Node test runs the actual plugin with an SDK fake and a subprocess reporter,
including resume binding, child exclusion, event ordering and reporter failure.

## Live acceptance

Run in a disposable home (`HOME`, `CODEX_HOME`, `XDG_*`) with its own tmux
server, a Motley build on PATH, and a wrapper recording each hook's stdin. Real
credentials were copied in, unchanged by the run (checksums compared) and deleted
afterwards. Models: Codex `gpt-6-astra` (low effort); OpenCode
`opencode-go/kimi-k3`.

Codex `codex-cli 0.159.3` (`hooks stable true`), with `approval_policy =
"on-request"` and `sandbox_mode = "workspace-write"`:

| Action | Hooks | Status |
| --- | --- | --- |
| First prompt | SessionStart, UserPromptSubmit | starting → idle → working |
| `ls` | PreToolUse, PostToolUse, Stop | working → ready |
| Write outside workspace | PreToolUse, PermissionRequest; approve: PostToolUse, Stop | permission → working → ready |
| Plan mode question | PreToolUse `request_user_input`; answer: PostToolUse | question → working |
| Esc during `sleep 30` | Interrupt; no late PostToolUse | idle |
| `/quit` | SessionEnd | ended |
| `motley revive` | `codex resume --no-daemon -- <id>`, SessionStart `source: resume` | context kept |
| `kill -9` the agent | none; Motley's `agent-exited` | ended |

OpenCode `1.18.21` with `permission.bash/edit = "ask"`:

| Action | Events | Status |
| --- | --- | --- |
| First prompt | session.created, chat.message, session.status busy | starting → idle → working |
| Read tool turn | session.status idle, session.idle (+ last text) | ready |
| Bash | permission.asked; allow: permission.replied | permission → working → ready |
| Question tool | question.asked; answer: question.replied | question → working → ready |
| Esc Esc | session.error `MessageAbortedError`, then idle twice | idle, "Turn interrupted" |
| `/exit` | none; Motley's `agent-exited` | ended |
| `motley revive` | `opencode --session=<id>`, plugin binds the resumed session | context kept |

Fixed after the first run: OpenCode emits no event on exit, so the member kept
`ready` with a shell in its pane, where a reply would have run as a command.
Agents also do not report crashes. The pane now runs `motley agent-exited`
after the agent returns, recording `AgentExit`/`ended` once (after Codex's
SessionEnd it adds nothing). OpenCode's idle events after an abort presented the
partial reply as finished; the plugin now marks them `interrupted`.

Known limitations, no native signal available:

- Codex fires SessionStart and OpenCode session.created only with the first
  prompt, so a bare spawn or revive shows `starting` until the user types.
- Codex 0.159.3 offers `request_user_input` only in Plan mode
  (`default_mode_request_user_input` is disabled), and its Plan-mode Stop has
  `last_assistant_message: null`, so the summary is empty.
- OpenCode sends both session.status idle and session.idle per turn; each is
  logged as a Stop.
- Codex hooks run only after `/hooks` trust; on first launch the trust prompt
  precedes any report.
