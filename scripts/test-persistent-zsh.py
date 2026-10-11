"""Fresh interactive/login Zsh with automatic .zshrc loading, on native macOS.

Run once against the standalone binary, then against a locally installed Homebrew
formula using --homebrew. Uses isolated HOME/ZDOTDIR and fixture credentials/cache.
"""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import pty
import select
import shlex
import subprocess
import tempfile
import time


def drain(fd, duration=0.1):
    end = time.monotonic() + duration
    while time.monotonic() < end:
        if select.select([fd], [], [], max(0, end - time.monotonic()))[0]:
            try:
                os.read(fd, 65536)
            except OSError:
                break


def snapshot(directory):
    return {p.relative_to(directory): p.read_bytes() for p in directory.rglob("*") if p.is_file()}


def setup(program, env, answer="", check=False):
    result = subprocess.run([str(program), "setup"] + (["--check"] if check else []),
                            input=answer, text=True, capture_output=True, env=env, check=True)
    return result.stdout


def cache(config):
    key = hashlib.sha256(b"owner/private\0false\0completion-test-read").hexdigest()
    data = {"schema": 1, "updated": datetime.datetime.now(datetime.timezone.utc).isoformat(),
            "items": [{"project": "nodi-ux-1", "version": v} for v in ("0.1.0", "0.2.1")]}
    (config / "completion").mkdir(parents=True, exist_ok=True)
    (config / f"completion/{key}.json").write_text(json.dumps(data))


def fresh_tab(root, env):
    cache(Path(env["NODI_CONFIG_DIR"]))
    master, slave = pty.openpty()
    # No -f and no sourcing/compinit commands in the test session. -d only disables global RCs.
    child = subprocess.Popen(["/bin/zsh", "-d", "-i", "-l"], stdin=slave, stdout=slave,
                             stderr=slave, cwd=root, env=env, start_new_session=True)
    os.close(slave)
    capture = root / "buffer.txt"
    try:
        drain(master, 2)
        setting = root / "setting.txt"
        harness = (f"print -r -- $NODI_USER_SETTING > {shlex.quote(str(setting))}; PROMPT='TEST> '; "
                   f"_capture() {{ print -r -- \"$BUFFER\" > {shlex.quote(str(capture))}; zle send-break; }}; "
                   "zle -N _capture; bindkey '^X^A' _capture\n")
        os.write(master, harness.encode())
        drain(master, 0.5)
        assert setting.read_text().strip() == "preserved", "Existing .zshrc setting was lost"
        for line, expected in [("nodi pub", "nodi publish"), ("nodi se", "nodi setup"),
                               ("nodi get nod", "nodi get nodi-ux-1"),
                               ("nodi get nodi-ux-1 --version 0.2.", "nodi get nodi-ux-1 --version 0.2.1")]:
            capture.unlink(missing_ok=True)
            os.write(master, (line + "\t\x18\x01").encode())
            deadline = time.monotonic() + 5
            while not capture.exists() and time.monotonic() < deadline:
                drain(master)
            assert capture.exists(), f"New Zsh did not capture {line!r}"
            assert expected in capture.read_text(), (line, capture.read_text())
            drain(master)
    finally:
        child.kill()
        child.wait(timeout=5)
        os.close(master)


def main(binary, homebrew):
    with tempfile.TemporaryDirectory(prefix="nodi-persistent-zsh-") as temp:
        root = Path(temp)
        home = root / "home"
        home.mkdir()
        if homebrew:
            program = binary
        else:
            program = home / ".local/bin/nodi"
        env = dict(os.environ, HOME=str(home), ZDOTDIR=str(home), TERM="xterm",
                   PATH=str(program.parent) + os.pathsep + os.environ["PATH"],
                   NODI_CONFIG_DIR=str(root / "config"), NODI_REPO="owner/private",
                   NODI_READ_TOKEN="completion-test-read", NODI_WRITE_TOKEN="")
        rc = home / ".zshrc"
        original = "typeset -g NODI_USER_SETTING=preserved\n"
        rc.write_text(original)
        before = snapshot(home)
        if not homebrew:
            installer = Path(__file__).resolve().with_name("install-macos.sh")
            subprocess.run(["/bin/sh", str(installer), "--bundle", str(binary.parent)],
                           stdin=subprocess.DEVNULL, env=env, check=True)
            assert rc.read_text() == original, "Unattended installer changed .zshrc"
            before = snapshot(home)
        setup(program, env, "yes\n", check=True)
        assert snapshot(home) == before, "--check wrote files"
        setup(program, env, "no\n")
        assert snapshot(home) == before, "Refusal wrote files"
        setup(program, env, "yes\n")
        configured = snapshot(home)
        assert rc.read_text().startswith(original), "Profile prefix changed"
        assert rc.read_text().count("# >>> nodi completion >>>") == 1
        backups = list(home.glob(".zshrc.nodi-backup-*"))
        assert len(backups) == 1 and backups[0].read_text() == original
        if homebrew:
            assert "compdef _nodi nodi" in rc.read_text() and "source <(" not in rc.read_text()
        setup(program, env, "yes\n")
        setup(program, env, check=True)
        assert snapshot(home) == configured, "Repeat/check changed files"
        fresh_tab(root, env)
        # Replacement in the same installation directory simulates an upgrade.
        if not homebrew:
            subprocess.run(["/bin/sh", str(installer), "--bundle", str(binary.parent)],
                           stdin=subprocess.DEVNULL, env=env, check=True)
        before_repeat = snapshot(home)
        setup(program, env, "yes\n")
        assert snapshot(home) == before_repeat
        fresh_tab(root, env)
        # Existing standard Homebrew initialization should need no Nodi-specific block.
        if homebrew:
            rc.write_text(original + 'eval "$(brew shellenv)"\nautoload -Uz compinit\ncompinit\n')
            before_native = snapshot(home)
            output = setup(program, env, "yes\n")
            assert "HOMEBREW CONFIG DETECTED" in output
            assert snapshot(home) == before_native
            fresh_tab(root, env)
        print("PASS: new interactive/login Zsh, literal Tab commands/projects/versions, consent, repeat, upgrade, existing settings; "
              + ("Homebrew site-functions" if homebrew else "standalone"))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("binary", type=Path)
    parser.add_argument("--homebrew", action="store_true")
    args = parser.parse_args()
    main(args.binary.resolve(), args.homebrew)
