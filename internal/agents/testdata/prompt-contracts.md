# Initial prompt contracts

Document schema: `1`

Verified from locally installed `--version` and `--help` on 2026-09-30.
No agent session or model request was started for this check.

- Claude Code **2.1.286**: `claude [options] [command] [prompt]`;
  interactive by default. Use a positional prompt after `--`.
- Codex CLI **0.159.2**: `codex [OPTIONS] [PROMPT]`;
  PROMPT is the optional user prompt to start a session. Use it after `--`.
- OpenCode **1.18.21**: `opencode [project]` starts the TUI;
  `--prompt` is a string option. Use `--prompt=<text>`.

W6 uses these native paths, bringing forward only initial prompt delivery from
W8. This avoids waiting for Codex/OpenCode idle hooks that are not installed yet.
Arguments and multiline prompts are passed directly to exec, never a shell.
Integration tests replace each executable with an argv recorder; they establish
handoff, not live model behaviour or compatibility with every older version.
