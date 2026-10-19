"""Real-browser self-test: runs main.py against a local Dizkover replica.

Why: the browser-free tests (test_core.py / test_flow.py) use a fake page, so
they cannot prove that Playwright, the selectors, the redirect handling and the
result files really work. This harness starts a local replica
(`selftest_server.py`), runs the unmodified `main.py` as a subprocess with a
throwaway data folder, then checks the produced files.

It never contacts the internet and never uses a real account.

    py -3 selftest_live.py
    py -3 selftest_live.py --visible        (watch the browser)
    set DIZKOVER_CHROMIUM=C:\\path\\chrome.exe & py -3 selftest_live.py

Exit code 0 = every expected result matched.
"""

from __future__ import annotations

import argparse
import os
import subprocess
import sys
import tempfile
from pathlib import Path

from selftest_server import Replica

HERE = Path(__file__).resolve().parent

ACCOUNTS = [
    ("gooduser", "goodpass"),
    ("baduser", "badpass"),
    ("banneduser", "bannedpass"),
    ("captchauser", "captchapass"),
]


def read(path: Path) -> str:
    try:
        return path.read_text(encoding="utf-8", errors="replace")
    except FileNotFoundError:
        return ""


def check(label: str, ok: bool, detail: str = "") -> bool:
    print("  [{}] {}{}".format("PASS" if ok else "FAIL", label, (" — " + detail) if detail else ""))
    return ok


def main() -> int:
    parser = argparse.ArgumentParser(description="Real-browser self-test")
    parser.add_argument("--visible", action="store_true", help="show the browser")
    args = parser.parse_args()

    failures = 0
    with tempfile.TemporaryDirectory(prefix="dizkover-selftest-") as tmp, Replica() as replica:
        data_dir = Path(tmp) / "data"
        data_dir.mkdir(parents=True, exist_ok=True)
        (data_dir / "ac.txt").write_text(
            "\n".join("{}:{}".format(u, p) for u, p in ACCOUNTS) + "\n",
            encoding="utf-8",
        )

        env = dict(os.environ)
        env.update(
            {
                "DIZKOVER_DATA_DIR": str(data_dir),
                "DIZKOVER_BASE_URL": replica.base_url,
                "DIZKOVER_LOGIN_PATH": "/site/login",
                "DIZKOVER_LOGIN_URL": replica.base_url + "/site/login",
                "DIZKOVER_LOGIN_NOTIFY_URL": replica.base_url + "/ajax/loginnotify",
                "DIZKOVER_USE_PROXY": "0",
                "DIZKOVER_TABS": "1",
                "PYTHONIOENCODING": "utf-8",
            }
        )

        command = [sys.executable, "main.py", "--tabs", "1", "--no-proxy"]
        if not args.visible:
            command.append("--headless")

        print("Replica: {}".format(replica.base_url))
        print("Data   : {}".format(data_dir))
        print("Command: {} {}".format(sys.executable, " ".join(command[1:])))
        print()
        result = subprocess.run(
            command, cwd=str(HERE), env=env, text=True, capture_output=True
        )
        tail = "\n".join((result.stdout or "").strip().splitlines()[-12:])
        print(tail)
        if result.returncode != 0:
            print((result.stderr or "").strip()[-2000:])
        print()

        logs = read(data_dir / "logs.txt")
        valid = read(data_dir / "valid.txt")
        invalid = read(data_dir / "invalid.txt")
        banned = read(data_dir / "banned.txt")
        errors = read(data_dir / "errors.txt")
        debug_files = list((data_dir / "debug").glob("*")) if (data_dir / "debug").exists() else []

        print("Expectations:")
        failures += not check("runner exit code is 0", result.returncode == 0, str(result.returncode))
        failures += not check("gooduser -> VALID", "gooduser" in valid and "goodpass" in valid)
        failures += not check("baduser -> INVALID", "baduser" in invalid)
        failures += not check(
            "invalid reason captured", "Incorrect username or password" in invalid
        )
        failures += not check("banneduser -> BANNED", "banneduser" in banned)
        failures += not check("captchauser -> ERROR", "captchauser" in errors)
        failures += not check("captcha step recorded", "captcha" in errors)
        failures += not check(
            "loginnotify was used for VALID", "loginnotify out=1" in logs
        )
        failures += not check(
            "no _csrf dead-end in the log", "CSRF not found" not in logs
        )
        failures += not check(
            "csrf state logged as informational",
            "csrf_hidden=absent" in logs,
        )
        failures += not check(
            "passwords never written to logs",
            all(password not in logs for _, password in ACCOUNTS),
        )
        failures += not check(
            "debug artifacts saved for errors", len(debug_files) >= 2, "{} file(s)".format(len(debug_files))
        )
        failures += not check(
            "final report written", "FINAL REPORT" in logs
        )

    print()
    if failures:
        print("RESULT: FAIL ({} unexpected result(s))".format(failures))
        return 1
    print("RESULT: PASS — all real-browser self-test expectations matched")
    return 0


if __name__ == "__main__":
    sys.exit(main())
