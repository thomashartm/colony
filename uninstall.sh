#!/usr/bin/env bash
# schema = 1
# Remove Motley's installation; preserve user work, settings and history.
set -euo pipefail
case "${1:-}" in
  '') ;;
  --help|-h)
    printf 'Usage: bash uninstall.sh\nRequires Python 3. Preserves worktrees, branches, configuration and saved state.\n'
    exit 0 ;;
  *) printf '• ❌ Unknown option: %s\n' "$1" >&2; exit 1 ;;
esac
[ "$#" -le 1 ] || { printf '• ❌ Too many arguments\n' >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { printf '• ❌ Python 3 is required to safely edit agent settings\n' >&2; exit 1; }
exec python3 - <<'PY'
import datetime
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shlex
import stat
import sys
import tempfile
import traceback

home = Path.home()
config = Path(os.environ.get('XDG_CONFIG_HOME') or home / '.config').absolute()
codex = Path(os.environ.get('CODEX_HOME') or home / '.codex')
history = home / '.motley' / 'uninstall-history'
console = sys.stdout
os.umask(0o077)
try:
    history.mkdir(parents=True, exist_ok=True)
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
    descriptor, log_path = tempfile.mkstemp(prefix=stamp+'-', suffix='.log', dir=history)
    log = os.fdopen(descriptor, 'w')
except OSError as error:
    print('• ❌ Cannot create uninstall history: '+str(error), file=sys.stderr)
    sys.exit(1)
sys.stdout = sys.stderr = log


def check(mark, message):
    line = '• '+mark+' '+message
    print(line, file=console, flush=True)
    print(line, flush=True)


def read_regular(path):
    if path.is_symlink():
        raise ValueError('Preserved symlink; review manually: '+str(path))
    if not path.exists():
        return None
    if not path.is_file():
        raise ValueError('Expected a regular file; preserved: '+str(path))
    return path.read_bytes()


# Preflight everything before removing commands or changing settings.
changes = []


def change(path, original, replacement, label):
    if original is not None and original != replacement:
        changes.append((path, original, replacement, label))


def hooks(path, agent):
    original = read_regular(path)
    if original is None:
        return
    root = json.loads(original)
    if not isinstance(root, dict):
        raise ValueError('Expected a settings object: '+str(path))
    table = root.get('hooks', {})
    if not isinstance(table, dict):
        raise ValueError('Expected a hooks object: '+str(path))
    changed = False
    for event, groups in list(table.items()):
        if not isinstance(groups, list):
            raise ValueError('Invalid hook event in '+str(path))
        kept_groups = []
        for group in groups:
            if not isinstance(group, dict) or not isinstance(group.get('hooks'), list):
                raise ValueError('Invalid hook group in '+str(path))
            handlers = group['hooks']
            kept = [h for h in handlers if not (
                isinstance(h, dict) and h.get('type') == 'command'
                and h.get('command') == 'motley report --agent '+agent)]
            if len(kept) == len(handlers):
                kept_groups.append(group)
                continue
            changed = True
            group['hooks'] = kept
            if kept or set(group) - {'hooks', 'matcher'}:
                kept_groups.append(group)
        if kept_groups:
            table[event] = kept_groups
        elif groups:
            del table[event]
    if not changed:
        return
    if not table:
        root.pop('hooks', None)
    if agent == 'codex' and isinstance(root.get('description'), str):
        lines = root['description'].splitlines()
        lines = [line for line in lines if line != 'Added motley schema = 1 status hooks.']
        if lines:
            root['description'] = '\n'.join(lines)
        else:
            root.pop('description', None)
    replacement = (json.dumps(root, indent=2, ensure_ascii=False)+'\n').encode()
    change(path, original, replacement, agent+' hooks')


def tmux_source(path):
    original = read_regular(path)
    if original is None:
        return
    lines = original.decode().splitlines(keepends=True)
    kept = []
    for line in lines:
        try:
            words = shlex.split(line, comments=True)
        except ValueError:
            words = []
        if len(words) == 2 and words[0] == 'source-file' and words[1] == str(config / 'motley/motley.tmux.conf'):
            if kept and kept[-1].strip() == '# motley installer — schema = 1':
                kept.pop()
            continue
        kept.append(line)
    change(path, original, ''.join(kept).encode(), 'tmux popup reference')


def binary(path):
    original = read_regular(path)
    if original is None:
        return
    if b'\xff Go buildinf:' not in original or b'path\tgithub.com/thomashartm/motley/cmd/motley\n' not in original:
        raise ValueError('Not a Motley executable; preserved: '+str(path))
    change(path, original, None, path.name+' command')


def write_backup(path, content):
    descriptor, name = tempfile.mkstemp(prefix=path.name+'.motley-uninstall-backup-', dir=path.parent)
    with os.fdopen(descriptor, 'wb') as backup:
        backup.write(content)
        backup.flush()
        os.fsync(backup.fileno())
    print('Backup: '+name, flush=True)


def apply(path, original, replacement):
    if read_regular(path) != original:
        raise ValueError('File changed during uninstall; preserved: '+str(path))
    if replacement is None:
        path.unlink()
        return
    mode = stat.S_IMODE(path.stat().st_mode)
    descriptor, name = tempfile.mkstemp(prefix='.motley-uninstall-', dir=path.parent)
    try:
        with os.fdopen(descriptor, 'wb') as output:
            output.write(replacement)
            output.flush()
            os.fsync(output.fileno())
            os.fchmod(output.fileno(), mode)
        os.replace(name, path)
    finally:
        if os.path.exists(name):
            os.unlink(name)


result = 0
lock = None
try:
    print('Motley uninstall — schema = 1\nStarted: '+stamp, flush=True)
    commands = home / '.local/bin'
    if commands.is_dir():
        lock = os.open(commands / '.motley-update.lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise ValueError('Another update or uninstall is running; retry when it finishes')
    hooks(home / '.claude/settings.json', 'claude')
    hooks(codex / 'hooks.json', 'codex')
    plugin = config / 'opencode/plugins/motley.ts'
    original = read_regular(plugin)
    if original is not None:
        # Only a shipped plugin is ours to remove. Keep edits or replacements.
        # List every released revision, so older installs are still recognised.
        if hashlib.sha256(original).hexdigest() not in {
            '159fa8c35b3d69b589b78275b5fb3e5219968228ee5c55b0d158d7bede018c48',
            'b6ecdd1dd0716fa34c9cc7b65191d69bba34b89a6dda762303ced45a82f55397',
        }:
            raise ValueError('Modified OpenCode plugin; preserved for manual review: '+str(plugin))
        change(plugin, original, None, 'opencode plugin')
    tmux_source(home / '.tmux.conf')
    tmux_source(config / 'tmux/tmux.conf')
    # Keep config files (including the popup snippet), shell PATH and fish PATH
    # setup. Disconnecting the tmux source removes future popup activation.
    alias = commands / 'mtly'
    alias_target = None
    if alias.is_symlink():
        alias_target = os.readlink(alias)
        if alias_target not in ('motley', str(commands / 'motley')):
            raise ValueError('Unrelated mtly symlink; preserved: '+str(alias))
    else:
        binary(alias)
    binary(commands / 'motley')
    check('✅', 'Installation checked')
    # Back up configuration before any edits or removals. Binaries can be
    # reinstalled; user settings and the plugin retain exact backup bytes.
    for path, original, replacement, label in changes:
        if path.parent != commands:
            write_backup(path, original)
    for path, original, replacement, label in changes:
        apply(path, original, replacement)
        print('Removed Motley integration or command: '+str(path), flush=True)
    if alias_target is not None:
        if not alias.is_symlink() or os.readlink(alias) != alias_target:
            raise ValueError('mtly symlink changed during uninstall; preserved')
        alias.unlink()
    check('✅', 'Motley commands and integrations removed' if changes or alias_target else 'Already uninstalled')
    check('✅', 'Worktrees, branches, settings and history preserved')
    print('Running tmux sessions were not changed. Shared shell PATH entries were kept.', flush=True)
    print('• Restart agents and tmux after current sessions finish', file=console, flush=True)
except KeyboardInterrupt:
    result = 130
    check('❌', 'Uninstall interrupted; check history before retrying')
except Exception as error:
    result = 1
    traceback.print_exc()
    check('❌', str(error))
finally:
    if lock is not None:
        os.close(lock)
    print('Finished; exit '+str(result), flush=True)
    print('• History: '+log_path, file=console, flush=True)
    log.close()
    sys.stdout, sys.stderr = console, console
sys.exit(result)
PY
