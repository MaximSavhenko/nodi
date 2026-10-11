"""Test real bash completion and interactive zsh Tab widgets on macOS CI."""
import datetime
import hashlib
import json
import os
from pathlib import Path
import pty
import select
import shlex
import shutil
import subprocess
import sys
import tempfile
import time


def drain(fd, duration=0.1):
    end = time.monotonic() + duration
    chunks = []
    while time.monotonic() < end:
        ready, _, _ = select.select([fd], [], [], max(0, end - time.monotonic()))
        if ready:
            try:
                chunks.append(os.read(fd, 65536))
            except OSError:
                break
    return b"".join(chunks).decode("utf-8", errors="replace")


def main(binary):
    with tempfile.TemporaryDirectory(prefix="nodi-zsh-smoke-") as temp:
        root = Path(temp)
        (root / "bin").mkdir()
        program = root / "bin/nodi"
        shutil.copyfile(binary, program)
        program.chmod(0o755)
        (root / "fixture parts").mkdir()
        config = root / "config"
        (config / "completion").mkdir(parents=True)
        env = dict(os.environ, PATH=str(root / "bin") + os.pathsep + os.environ["PATH"],
                   NODI_CONFIG_DIR=str(config), NODI_REPO="owner/private",
                   NODI_READ_TOKEN="completion-test-read", NODI_WRITE_TOKEN="completion-test-write", TERM="xterm")
        for role in ("read", "write"):
            token = env[f"NODI_{role.upper()}_TOKEN"]
            key = hashlib.sha256(f"owner/private\0{str(role == 'write').lower()}\0{token}".encode()).hexdigest()
            cache = {"schema": 1, "updated": datetime.datetime.now(datetime.timezone.utc).isoformat(),
                     "items": [{"project": "nodi-demo", "version": v} for v in ("0.1.0", "0.1.1")]}
            (config / f"completion/{key}.json").write_text(json.dumps(cache))
        for shell in ("bash", "zsh"):
            script = subprocess.check_output([str(program), "completion", shell], env=env)
            (root / f"completion.{shell}").write_bytes(script)
        (root / "zsh-functions").mkdir()
        shutil.copyfile(root / "completion.zsh", root / "zsh-functions/_nodi")
        cases = [(["nodi", "pub"], ["publish"]), (["nodi", "auth", "lo"], ["login", "logout"]),
                 (["nodi", "get", "nod"], ["nodi-demo"]),
                 (["nodi", "get", "nodi-demo", "--version", "0.1."], ["0.1.0", "0.1.1"]),
                 (["nodi", "publish", "nodi-demo", "fi"], ["fixture parts"]),
                 (["nodi", "del"], ["delete"]), (["nodi", "upd"], ["update"]),
                 (["nodi", "send", "fixture", "--n"], ["--name"]),
                 (["nodi", "delete", "nod"], ["nodi-demo"]),
                 (["nodi", "delete", "nodi-demo", "--version", "0.1."], ["0.1.0", "0.1.1"]),
                 (["nodi", "delete", "nodi-demo", "--d"], ["--dry-run"]),
                 (["nodi", "update", "--c"], ["--check"])]
        for words, expected in cases:
            code = f"source {shlex.quote(str(root / 'completion.bash'))}; COMP_WORDS=({' '.join(map(shlex.quote, words))}); COMP_CWORD={len(words)-1}; _nodi_complete; printf '%s\\n' \"${{COMPREPLY[@]}}\""
            got = subprocess.check_output(["/bin/bash", "-c", code], cwd=root, env=env, text=True).splitlines()
            assert all(item in got for item in expected), (words, got)
        master, slave = pty.openpty()
        process = subprocess.Popen(["/bin/zsh", "-f"], stdin=slave, stdout=slave, stderr=slave,
                                   cwd=root, env=env, start_new_session=True)
        os.close(slave)
        capture = root / "buffer.txt"
        try:
            drain(master, 0.3)
            setup = (f"PROMPT='UX> '; unsetopt beep; fpath=({shlex.quote(str(root / 'zsh-functions'))} $fpath); "
                     f"autoload -Uz compinit; compinit -d {shlex.quote(str(root / 'zcompdump'))}; "
                     f"_nodi_capture() {{ print -r -- \"$BUFFER\" > {shlex.quote(str(capture))}; zle send-break; }}; "
                     "zle -N _nodi_capture; bindkey '^X^A' _nodi_capture\n")
            os.write(master, setup.encode())
            drain(master, 2)
            for line, expected in [("nodi pub", "nodi publish"), ("nodi del", "nodi delete"),
                                   ("nodi upd", "nodi update"), ("nodi send fixture --n", "nodi send fixture --name"),
                                   ("nodi delete nod", "nodi delete nodi-demo"), ("nodi auth logi", "nodi auth login"),
                                   ("nodi get nod", "nodi get nodi-demo"),
                                   ("nodi get nodi-demo --version 0.1.1", "nodi get nodi-demo --version 0.1.1"),
                                   ("nodi publish nodi-demo fi", "fixture\\ parts/")]:
                capture.unlink(missing_ok=True)
                os.write(master, (line + "\t\x18\x01").encode())
                deadline = time.monotonic() + 5
                while not capture.exists() and time.monotonic() < deadline:
                    drain(master, 0.1)
                assert capture.exists(), f"zsh widget did not capture {line!r}"
                got = capture.read_text().strip()
                assert expected in got, (line, got)
                drain(master, 0.1)
            print("PASS: real bash completion and interactive zsh Tab, commands/projects/versions/paths with spaces.")
        finally:
            # An interactive zsh ignores SIGTERM; this is our isolated test child.
            process.kill()
            process.wait(timeout=5)
            os.close(master)


if __name__ == "__main__":
    main(Path(sys.argv[1]).resolve())
