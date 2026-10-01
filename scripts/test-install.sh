#!/usr/bin/env bash
# schema = 1
# Exercise installer writes in disposable homes; never install real packages.
set -euo pipefail
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
fixture=$(mktemp -d "${TMPDIR:-/tmp}/motley-installer-test.XXXXXX")
trap 'rm -rf -- "$fixture"' EXIT
mkdir -p "$fixture/tools"
export INSTALL_TEST_LOG="$fixture/log"

cat > "$fixture/tools/git" <<'SH'
#!/usr/bin/env bash
printf 'git %s\n' "$*" >> "$INSTALL_TEST_LOG"
if [ "$1" = clone ]; then mkdir -p "${@: -1}"; else printf 'abcdef0\n'; fi
SH
cat > "$fixture/tools/tmux" <<'SH'
#!/usr/bin/env bash
printf 'tmux %s\n' "$*" >> "$INSTALL_TEST_LOG"
case "$1" in
  -V) printf 'tmux %s\n' "${INSTALL_TEST_TMUX_VERSION:-3.6a}" ;;
  list-sessions) exit 1 ;;
  *) exit 1 ;;
esac
SH
cat > "$fixture/tools/go" <<'SH'
#!/usr/bin/env bash
set -eu
if [ "$1" = version ]; then printf 'go version go1.25.5 linux/amd64\n'; exit; fi
printf 'go %s\n' "$*" >> "$INSTALL_TEST_LOG"
[ "${INSTALL_TEST_BUILD_FAIL:-0}" = 0 ] || exit 1
while [ "$1" != -o ]; do shift; done
cp "$INSTALL_TEST_BINARY" "$2"
SH
cat > "$fixture/motley" <<'SH'
#!/usr/bin/env bash
set -eu
printf 'motley %s\n' "$*" >> "$INSTALL_TEST_LOG"
case "$1" in
  init)
    directory="${XDG_CONFIG_HOME:-$HOME/.config}/motley"
    mkdir -p "$directory"
    [ -e "$directory/config.toml" ] || printf 'schema = 1\n' > "$directory/config.toml"
    [ -e "$directory/motley.tmux.conf" ] || printf '# schema = 1\n' > "$directory/motley.tmux.conf" ;;
  version) printf 'motley test\n' ;;
  hooks) ;;
  *) exit 1 ;;
esac
SH
printf '#!/bin/sh\nexit 0\n' > "$fixture/tools/claude"
chmod +x "$fixture/tools/"* "$fixture/motley"
export INSTALL_TEST_BINARY="$fixture/motley"
export PATH="$fixture/tools:$PATH"
unset ZDOTDIR

for login_shell in zsh bash fish; do
  export HOME="$fixture/$login_shell home"
  export XDG_CONFIG_HOME="$HOME/config with spaces"
  export SHELL="/bin/$login_shell"
  mkdir -p "$HOME" "$XDG_CONFIG_HOME/motley"
  printf 'schema = 1\nrepos_root = "~/custom"\n' > "$XDG_CONFIG_HOME/motley/config.toml"
  printf '# my settings\n' > "$HOME/.tmux.conf"
  case "$login_shell" in
    zsh) startup="$HOME/.zshrc" ;;
    bash) startup="$HOME/.bashrc" ;;
    fish) startup="$XDG_CONFIG_HOME/fish/conf.d/motley.fish"; mkdir -p "$(dirname "$startup")" ;;
  esac
  printf '# my shell settings\n' > "$startup"
  bash "$repo/install.sh" --local > "$fixture/output"
  [ -x "$HOME/.local/bin/motley" ]
  [ -L "$HOME/.local/bin/mtly" ]
  [ "$(readlink "$HOME/.local/bin/mtly")" = motley ]
  [ "$("$HOME/.local/bin/mtly" version)" = 'motley test' ]
  grep -q '^repos_root = "~/custom"' "$XDG_CONFIG_HOME/motley/config.toml"
  grep -q '^# my shell settings' "$startup"
  grep -q '^# my settings' "$HOME/.tmux.conf"
  grep -q '^source-file ' "$HOME/.tmux.conf"
  compgen -G "$startup.motley-backup.*" >/dev/null
  compgen -G "$HOME/.tmux.conf.motley-backup.*" >/dev/null
  cp "$startup" "$fixture/startup-before"
  cp "$HOME/.tmux.conf" "$fixture/tmux-before"
  bash "$repo/install.sh" --local > "$fixture/output"
  cmp "$startup" "$fixture/startup-before"
  cmp "$HOME/.tmux.conf" "$fixture/tmux-before"
done

# A failed build must preserve the installed binary and configuration.
cp "$HOME/.local/bin/motley" "$fixture/binary-before"
if INSTALL_TEST_BUILD_FAIL=1 bash "$repo/install.sh" --local > "$fixture/output" 2>&1; then
  printf 'Expected build failure\n' >&2; exit 1
fi
cmp "$HOME/.local/bin/motley" "$fixture/binary-before"
[ "$("$HOME/.local/bin/mtly" version)" = 'motley test' ]
cmp "$HOME/.tmux.conf" "$fixture/tmux-before"
if INSTALL_TEST_TMUX_VERSION=3.1 bash "$repo/install.sh" --local > "$fixture/output" 2>&1; then
  printf 'Expected old tmux refusal\n' >&2; exit 1
fi
grep -q 'tmux 3.2 or newer' "$fixture/output"

# Default installation uses the public Git URL; no connector is involved.
bash "$repo/install.sh" > "$fixture/output"
grep -q 'git clone --quiet --depth 1 https://github.com/thomashartm/motley.git' "$INSTALL_TEST_LOG"
grep -q 'motley hooks install claude' "$INSTALL_TEST_LOG"
printf 'Installer tests passed (bash, zsh, fish; repeat install; build failure; download).\n'
