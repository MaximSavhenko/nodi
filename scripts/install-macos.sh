#!/bin/sh
set -eu
usage() { echo 'Usage: sh install-macos.sh --bundle DIRECTORY | --repo OWNER/REPO --release vX.Y.Z [--add-path|--no-path] [--no-completion]' >&2; exit 1; }
bundle='' repo='' release='' version='' add_path=0 completion=1
while [ "$#" -gt 0 ]; do
  case "$1" in
    --bundle|--repo|--release|--version) [ "$#" -ge 2 ] || usage; key=$1; value=$2; shift 2; case "$key" in --bundle) bundle=$value;; --repo) repo=$value;; --release) release=$value;; --version) version=$value;; esac;;
    --no-path) add_path=0; shift;;
    --add-path) add_path=1; shift;;
    --no-completion) completion=0; shift;;
    *) usage;;
  esac
done
[ "$(uname -s)" = Darwin ] || { echo 'macOS required' >&2; exit 1; }
case "$(uname -m)" in arm64) asset=nodi-darwin-arm64;; x86_64) asset=nodi-darwin-amd64;; *) echo 'Unsupported architecture' >&2; exit 1;; esac
temporary=$(mktemp -d "${TMPDIR:-/tmp}/nodi-install.XXXXXXXX")
trap 'rm -rf "$temporary"' EXIT HUP INT TERM
if [ -n "$bundle" ]; then
  [ -z "$repo$release" ] || usage
  cp "$bundle/$asset" "$temporary/$asset"
  cp "$bundle/SHA256SUMS" "$temporary/SHA256SUMS"
else
  [ -n "$repo" ] && [ -n "$release" ] || usage
  printf '%s\n' "$repo" | grep -Eq '^[A-Za-z0-9-]+/[A-Za-z0-9_.-]+$' || usage
  printf '%s\n' "$release" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$' || usage
  # Public trusted CLI release. Private manufacturing storage uses CLI auth;
  # private CLI distribution can use the local bundle installation mode.
  url="https://github.com/$repo/releases/download/$release"
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --max-filesize 33554432 "$url/$asset" -o "$temporary/$asset"
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --max-filesize 1048576 "$url/SHA256SUMS" -o "$temporary/SHA256SUMS"
fi
expected=$(awk -v name="$asset" '$2 == name && length($1) == 64 { print $1 }' "$temporary/SHA256SUMS")
[ "${#expected}" -eq 64 ] || { echo 'Missing or ambiguous checksum' >&2; exit 1; }
actual=$(shasum -a 256 "$temporary/$asset" | awk '{print $1}')
[ "$expected" = "$actual" ] || { echo 'SHA-256 mismatch; installation refused' >&2; exit 1; }
chmod 755 "$temporary/$asset"
if [ -n "$release" ]; then
  [ -z "$version" ] || [ "v$version" = "$release" ] || { echo 'Version and release disagree' >&2; exit 1; }
  version=${release#v}
fi
validate_candidate() {
  candidate_path=$1 expected_version=$2
  # Runtime alone can execute Intel binaries via Rosetta. Verify the Mach-O
  # header architecture too, using the platform's native file tool.
  case "$(/usr/bin/file -b "$candidate_path")" in
    "Mach-O 64-bit executable "*) ;;
    *) echo 'Expected native Mach-O executable' >&2; return 1;;
  esac
  case "$asset:$(/usr/bin/file -b "$candidate_path")" in
    nodi-darwin-arm64:*arm64*|nodi-darwin-amd64:*x86_64*) ;;
    *) echo 'Wrong binary architecture' >&2; return 1;;
  esac
  reported=$("$candidate_path" version) || return 1
  case "$asset" in *arm64) platform=darwin/arm64;; *) platform=darwin/amd64;; esac
  printf '%s\n' "$reported" | grep -Eq "^nodi (0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*) $platform$" || { echo 'Candidate version/platform check failed' >&2; return 1; }
  actual_version=$(printf '%s\n' "$reported" | awk '{print $2}')
  [ -z "$expected_version" ] || [ "$actual_version" = "$expected_version" ] || { echo 'Candidate version differs from requested version' >&2; return 1; }
  printf '%s\n' "$actual_version"
}
verified_version=$(validate_candidate "$temporary/$asset" "$version")
install_dir="$HOME/.local/bin"
mkdir -p "$install_dir"
lock="$install_dir/.nodi-install.lock"
mkdir "$lock" 2>/dev/null || { echo "Installer already active or interrupted; inspect $lock" >&2; exit 1; }
trap 'rmdir "$lock" 2>/dev/null || true; rm -rf "$temporary"' EXIT HUP INT TERM
transaction=$(mktemp -d "$install_dir/.nodi-update.XXXXXXXX")
candidate="$transaction/candidate"
backup="$transaction/previous"
cp "$temporary/$asset" "$candidate"
chmod 755 "$candidate"
[ "$(shasum -a 256 "$candidate" | awk '{print $1}')" = "$expected" ] || { echo 'Staged checksum differs' >&2; exit 1; }
validate_candidate "$candidate" "$verified_version" >/dev/null
had_previous=0
if [ -e "$install_dir/nodi" ]; then
  [ -f "$install_dir/nodi" ] && [ ! -L "$install_dir/nodi" ] || { echo 'Existing target is not a regular executable' >&2; exit 1; }
  # Keep the original inode as the crash/rollback backup before atomic replace.
  ln "$install_dir/nodi" "$backup"
  had_previous=1
fi
mv -f "$candidate" "$install_dir/nodi"
if ! validate_candidate "$install_dir/nodi" "$verified_version" >/dev/null; then
  if [ "$had_previous" -eq 1 ]; then
    # Retain the backup even if restoring the target fails.
    ln "$backup" "$transaction/restore" && mv -f "$transaction/restore" "$install_dir/nodi" || { echo "Rollback failed; previous executable retained at $backup" >&2; exit 1; }
  else
    mv "$install_dir/nodi" "$transaction/rejected" || { echo "Failed new install retained at $install_dir/nodi" >&2; exit 1; }
  fi
  echo 'Installed binary failed version check; rollback completed' >&2
  exit 1
fi
echo "Verified nodi $verified_version"
[ "$had_previous" -eq 0 ] || echo "Previous binary backup: $backup"
if [ "$add_path" -eq 1 ]; then
  path_line='export PATH="$HOME/.local/bin:$PATH"'
  touch "$HOME/.zprofile"
  grep -Fqx "$path_line" "$HOME/.zprofile" || printf '\n%s\n' "$path_line" >> "$HOME/.zprofile"
fi
echo "Installed: $HOME/.local/bin/nodi. Reopen Terminal or run: export PATH=\"\$HOME/.local/bin:\$PATH\""
if [ "$completion" -eq 1 ]; then
  if "$HOME/.local/bin/nodi" help | grep -q 'nodi setup'; then
    set -- setup
    if "$HOME/.local/bin/nodi" help | grep -q 'nodi setup --completion'; then
      set -- setup --completion
    fi
    if [ -t 0 ]; then
      "$HOME/.local/bin/nodi" "$@" || echo 'Completion setup did not finish. Run nodi setup --completion again; the binary is installed.' >&2
    else
      "$HOME/.local/bin/nodi" "$@" --check || true
      echo 'For persistent completion, run nodi setup --completion interactively (profile changes require consent).'
    fi
  else
    echo 'This version requires manual completion setup. See nodi completion.'
  fi
fi
