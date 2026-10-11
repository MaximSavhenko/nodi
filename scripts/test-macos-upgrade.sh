#!/bin/bash
set -euo pipefail
[[ "${GITHUB_ACTIONS:-}" == true && "${RUNNER_OS:-}" == macOS ]] || { echo 'Requires a disposable macOS CI runner' >&2; exit 1; }
version="$1"
bundle="$2"
root="$(mktemp -d "$RUNNER_TEMP/nodi-upgrade.XXXXXXXX")"
nodi_upgrade_home="$root/home"
export NODI_CONFIG_DIR="$root/config"
unset NODI_READ_TOKEN NODI_WRITE_TOKEN
mkdir -p "$nodi_upgrade_home" "$NODI_CONFIG_DIR" "$root/downloads"
printf '{"repository":"owner/private"}\n' > "$NODI_CONFIG_DIR/config.json"
printf 'fictional-upgrade-read' > "$NODI_CONFIG_DIR/read.credential"
printf 'fictional-upgrade-write' > "$NODI_CONFIG_DIR/write.credential"
chmod 700 "$NODI_CONFIG_DIR"
chmod 600 "$NODI_CONFIG_DIR"/*
printf 'preserved recipient edits\n' > "$root/downloads/user-edited.step"
printf 'existing user shell profile\n' > "$nodi_upgrade_home/.zprofile"
env HOME="$nodi_upgrade_home" ZDOTDIR="$nodi_upgrade_home" sh scripts/install-macos.sh --repo MaximSavhenko/nodi --release v0.3.0 --no-path --no-completion
[[ "$("$nodi_upgrade_home/.local/bin/nodi" version)" == 'nodi 0.3.0 darwin/'* ]]
shasum -a 256 "$NODI_CONFIG_DIR"/* "$root/downloads"/* "$nodi_upgrade_home/.zprofile" > "$root/before.sha256"
env HOME="$nodi_upgrade_home" ZDOTDIR="$nodi_upgrade_home" sh scripts/install-macos.sh --bundle "$bundle" --no-path --no-completion
[[ "$("$nodi_upgrade_home/.local/bin/nodi" version)" == "nodi $version darwin/"* ]]
shasum -a 256 --check "$root/before.sha256"
[[ "$(stat -f '%Lp' "$NODI_CONFIG_DIR/read.credential")" == 600 ]]
[[ "$(stat -f '%Lp' "$NODI_CONFIG_DIR/write.credential")" == 600 ]]
"$nodi_upgrade_home/.local/bin/nodi" help
echo "PASS: macOS installer upgrade 0.3.0 -> $version; config, credential bytes/permissions, downloads and profile preserved"
# Retain this runner's evidence; the real user's home is never used.
