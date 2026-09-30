<!-- schema = 1 -->
# Claude hook recordings

Captured locally with Claude Code **2.1.285**, Haiku, on 2026-09-30.
`recorded.json` contains real hook payloads with paths and session/prompt IDs
replaced by fixture values. Expected mappings are hand-written.
`transcript.jsonl` retains only the assistant text record fields used by colony;
model metadata and opaque signatures are omitted.

A temporary settings file registered a JSON-stdin recorder for SessionStart,
UserPromptSubmit, PreToolUse, PostToolUse, Notification, Stop and SessionEnd.
The recorder appended payloads without emitting stdout. No global settings were
changed. CLI options: `--setting-sources '' --settings <recorder-settings.json>
--strict-mcp-config --model haiku --effort low`.

1. In print mode, allow only Read, read a fixture file, then respond exactly
   `colony fixture complete`.
2. In interactive mode, enable AskUserQuestion and Bash. Ask “Which fixture
   option?” (Alpha/Beta), then request `touch` on a temporary marker. Select Alpha
   and approve that single command manually. Wait for the idle notification,
   then exit normally. This records question, permission and idle payloads.

Observed: AskUserQuestion also emits a generic `permission_prompt` notification.
The reporter retains the preceding question detail and state for that session.
A Bash permission notification contains only a generic message, so the reporter
retains preceding tool input in a bounded tmux option.

Stop supplies `last_assistant_message`. The copied transcript taken *during*
Stop did not yet contain that last record; the completed transcript did. Colony
prefers the direct field and reads only the final 64 KiB of a transcript when
that field is absent.

Reference: [official Claude Code hooks documentation](https://code.claude.com/docs/en/hooks).
