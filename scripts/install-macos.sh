#!/bin/sh
set -eu
usage() { echo 'Usage: sh install-macos.sh --bundle DIRECTORY | --repo OWNER/REPO --release vX.Y.Z [--add-path|--no-path] [--no-completion]' >&2; exit 1; }
bundle='' repo='' release='' add_path=0 completion=1
while [ "$#" -gt 0 ]; do
  case "$1" in
    --bundle|--repo|--release) [ "$#" -ge 2 ] || usage; key=$1; value=$2; shift 2; case "$key" in --bundle) bundle=$value;; --repo) repo=$value;; --release) release=$value;; esac;;
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
mkdir -p "$HOME/.local/bin"
chmod 755 "$temporary/$asset"
mv "$temporary/$asset" "$HOME/.local/bin/nodi"
"$HOME/.local/bin/nodi" version
if [ "$add_path" -eq 1 ]; then
  path_line='export PATH="$HOME/.local/bin:$PATH"'
  touch "$HOME/.zprofile"
  grep -Fqx "$path_line" "$HOME/.zprofile" || printf '\n%s\n' "$path_line" >> "$HOME/.zprofile"
fi
echo "Installed: $HOME/.local/bin/nodi. Reopen Terminal or run: export PATH=\"\$HOME/.local/bin:\$PATH\""
if [ "$completion" -eq 1 ]; then
  if "$HOME/.local/bin/nodi" help | grep -q 'nodi setup'; then
    if [ -t 0 ]; then
      "$HOME/.local/bin/nodi" setup || echo 'Completion setup did not finish. Run nodi setup again; the binary is installed.' >&2
    else
      "$HOME/.local/bin/nodi" setup --check || true
      echo 'For persistent completion, run nodi setup interactively (profile changes require consent).'
    fi
  else
    echo 'This version requires manual completion setup. See nodi completion.'
  fi
fi
