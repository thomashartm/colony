#!/usr/bin/env bash
# schema = 1
# Download/build motley and configure it for the current user.
set -euo pipefail

fail() { printf 'motley installer: %s\n' "$*" >&2; exit 1; }
say() { printf '\n%s\n' "$*"; }

local_source=false
case "${1:-}" in
  '') ;;
  --local) local_source=true ;;
  --help|-h)
    printf 'Usage: bash install.sh [--local]\n\nDownloads and builds main by default; --local builds the checkout containing this script.\n'
    exit 0 ;;
  *) fail "Unknown option: $1" ;;
esac
[ "$#" -le 1 ] || fail 'Too many arguments'

case "$(uname -s)/$(uname -m)" in
  Darwin/arm64|Darwin/x86_64|Linux/aarch64|Linux/arm64|Linux/x86_64) ;;
  *) fail 'Supported platforms: macOS/Linux on amd64/arm64' ;;
esac
SHELL=${SHELL:-/bin/bash}
case "${SHELL##*/}" in
  bash|zsh|fish) ;;
  *) fail 'Supported login shells: bash, zsh, fish (set SHELL to your login shell)' ;;
esac

as_root() {
  if [ "$(id -u)" -eq 0 ]; then "$@"; else sudo "$@"; fi
}

# Install only missing tools. Agent CLIs and their authentication stay user-owned.
missing=()
for tool in git go tmux; do
  command -v "$tool" >/dev/null 2>&1 || missing+=("$tool")
done
if [ "${#missing[@]}" -gt 0 ]; then
  say "Installing prerequisites: ${missing[*]}"
  if command -v brew >/dev/null 2>&1; then
    brew install "${missing[@]}"
  elif command -v apt-get >/dev/null 2>&1; then
    packages=()
    for tool in "${missing[@]}"; do
      if [ "$tool" = go ]; then packages+=(golang-go); else packages+=("$tool"); fi
    done
    as_root apt-get update
    as_root apt-get install -y "${packages[@]}"
  elif command -v dnf >/dev/null 2>&1; then
    packages=()
    for tool in "${missing[@]}"; do
      if [ "$tool" = go ]; then packages+=(golang); else packages+=("$tool"); fi
    done
    as_root dnf install -y "${packages[@]}"
  elif command -v pacman >/dev/null 2>&1; then
    as_root pacman -S --needed --noconfirm "${missing[@]}"
  else
    fail 'Install Homebrew (macOS), or git/go/tmux with your Linux package manager, then rerun.'
  fi
fi
for tool in git go tmux; do
  command -v "$tool" >/dev/null 2>&1 || fail "$tool is still unavailable on PATH"
done
tmux_version=$(tmux -V)
if [[ "$tmux_version" =~ ([0-9]+)\.([0-9]+) ]]; then
  (( BASH_REMATCH[1] > 3 || (BASH_REMATCH[1] == 3 && BASH_REMATCH[2] >= 2) )) || fail 'tmux 3.2 or newer is required; upgrade tmux and rerun.'
else
  fail "Cannot read tmux version: $tmux_version"
fi

# Go 1.21+ can obtain the tested toolchain automatically without replacing Go.
go_version=$(GOTOOLCHAIN=local go version)
if [[ "$go_version" =~ go([0-9]+)\.([0-9]+) ]]; then
  (( BASH_REMATCH[1] > 1 || BASH_REMATCH[2] >= 21 )) || fail 'Go 1.21+ is needed to download the build toolchain; upgrade Go and rerun.'
else
  fail "Cannot read Go version: $go_version"
fi

task_tmp=$(mktemp -d "${TMPDIR:-/tmp}/motley-install.XXXXXX")
staged_binary=''
cleanup() {
  rm -rf -- "$task_tmp"
  if [ -n "$staged_binary" ]; then rm -f -- "$staged_binary"; fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if "$local_source"; then
  source_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
  [ -f "$source_dir/go.mod" ] || fail '--local requires a motley checkout'
else
  say 'Downloading motley (main)…'
  source_dir="$task_tmp/source"
  git clone --quiet --depth 1 https://github.com/thomashartm/motley.git "$source_dir"
fi

revision=$(git -C "$source_dir" rev-parse --short HEAD)
version="main-$revision"
if "$local_source"; then version="local-$revision"; fi
say "Building motley ${version}…"
(
  cd -- "$source_dir"
  CGO_ENABLED=0 GOTOOLCHAIN=go1.25.5 GOWORK=off go build -buildvcs=false -trimpath \
    -ldflags "-s -w -X main.version=$version" -o "$task_tmp/motley" ./cmd/motley
)

bin_dir="$HOME/.local/bin"
mkdir -p -- "$bin_dir"
[ ! -d "$bin_dir/mtly" ] || fail "$bin_dir/mtly is a directory; cannot install the short command"
# Replace by rename so existing running binaries are safe to upgrade.
staged_binary=$(mktemp "$bin_dir/.motley.XXXXXX")
cp -- "$task_tmp/motley" "$staged_binary"
chmod 755 "$staged_binary"
mv -f -- "$staged_binary" "$bin_dir/motley"
staged_binary=''
ln -sfn motley "$bin_dir/mtly"
export PATH="$bin_dir:$PATH"

append_setting() {
  local file=$1 line=$2
  if [ -f "$file" ] && grep -Fqx -- "$line" "$file"; then return; fi
  mkdir -p -- "$(dirname -- "$file")"
  if [ -e "$file" ]; then
    local backup
    backup=$(mktemp "$file.motley-backup.XXXXXX")
    cp -p -- "$file" "$backup"
  fi
  printf '\n# motley installer — schema = 1\n%s\n' "$line" >> "$file"
}

say 'Configuring PATH and tmux…'
# Expand these variables when a future shell reads its startup file.
# shellcheck disable=SC2016
case "${SHELL##*/}" in
  zsh)
    append_setting "${ZDOTDIR:-$HOME}/.zshrc" 'export PATH="$HOME/.local/bin:$PATH"' ;;
  bash)
    append_setting "$HOME/.bashrc" 'export PATH="$HOME/.local/bin:$PATH"'
    if [ -f "$HOME/.bash_profile" ]; then login_file="$HOME/.bash_profile"
    elif [ -f "$HOME/.bash_login" ]; then login_file="$HOME/.bash_login"
    else login_file="$HOME/.profile"; fi
    append_setting "$login_file" 'export PATH="$HOME/.local/bin:$PATH"' ;;
  fish)
    append_setting "${XDG_CONFIG_HOME:-$HOME/.config}/fish/conf.d/motley.fish" \
      'fish_add_path --prepend --move "$HOME/.local/bin"' ;;
esac

"$bin_dir/motley" init >/dev/null
config_dir="${XDG_CONFIG_HOME:-$HOME/.config}/motley"
config_dir=$(cd -- "$config_dir" && pwd)
# Quote paths as tmux single-quoted strings, including spaces and apostrophes.
tmux_path=${config_dir//\'/\'\\\'\'}
tmux_file="$HOME/.tmux.conf"
if [ ! -e "$tmux_file" ] && [ -f "${XDG_CONFIG_HOME:-$HOME/.config}/tmux/tmux.conf" ]; then
  tmux_file="${XDG_CONFIG_HOME:-$HOME/.config}/tmux/tmux.conf"
fi
append_setting "$tmux_file" "source-file '$tmux_path/motley.tmux.conf'"
if tmux list-sessions >/dev/null 2>&1; then
  tmux set-environment -g PATH "$PATH"
  tmux source-file "$config_dir/motley.tmux.conf"
fi
if command -v claude >/dev/null 2>&1; then
  "$bin_dir/motley" hooks install claude
fi

"$bin_dir/motley" version
printf '\nInstalled: %s/motley (also available as mtly)\nOpen a new terminal and run motley or mtly. No manual PATH or config edits are needed.\n' "$bin_dir"
printf 'To run immediately in this terminal: "%s/motley"\n' "$bin_dir"
printf 'Restart existing Claude sessions to load hooks; rerun this installer after installing Claude.\n'
