# W8 integration contracts

Verified locally on 2026-10-01. These fixtures are synthetic examples derived
from the interfaces below, not recordings of live paid agent turns.

## Codex

Installed `codex --version`: `codex-cli 0.159.2`.
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
and the question tool's options. Codex transcripts are not Claude transcripts;
the parser uses Stop's supplied message and does not read transcript files.

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
`chat.message` envelope and `lastAssistantMessage` enrichment. The plugin
keeps only a bounded last text excerpt; no transcript or model API call is used.
Its Node test runs the actual plugin with an SDK fake and a subprocess reporter,
including resume binding, child exclusion, event ordering and reporter failure.

Remaining real-environment gate: observe status transitions and resume in the
installed agent UIs. This sandbox cannot start the isolated tmux servers used
by the CLI integration suite. No live agent-turn payloads or screenshots have
been collected in this run.
