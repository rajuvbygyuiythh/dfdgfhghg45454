# Dizkover account checker - configuration.
#
# Every value can be overridden for one run without editing this file, e.g.
#     set DIZKOVER_HEADLESS=0 & py -3 main.py
# Only the documented environment variables below are read.

import os


def _flag(name, default):
    value = os.environ.get(name)
    if value is None or value == "":
        return default
    return value.strip().lower() in ("1", "true", "yes", "on")


def _text(name, default):
    value = os.environ.get(name)
    return default if value is None or value == "" else value.strip()


def _number(name, default, cast=int):
    value = os.environ.get(name)
    if value is None or value == "":
        return default
    try:
        return cast(value)
    except (TypeError, ValueError):
        return default

# Paths below are relative to the checker's data/ folder, which is created by
# install.bat. Runtime files never live next to the source, so they cannot be
# committed or packed into a source archive by accident.

# ---- Files (inside data/) -------------------------------------------------
# Override the whole folder (used by selftest_live.py so a self-test never
# touches your real accounts/results).
DATA_DIR = _text("DIZKOVER_DATA_DIR", "")
ACCOUNTS_FILE = "ac.txt"
PROXY_FILE = "proxy.txt"
VALID_FILE = "valid.txt"
INVALID_FILE = "invalid.txt"
BANNED_FILE = "banned.txt"
ERRORS_FILE = "errors.txt"
LOG_FILE = "logs.txt"
DEBUG_DIR = "debug"

# ---- Target ---------------------------------------------------------------
# The URLs can be overridden for testing against a local replica; see
# selftest_live.py. Leave them as they are for real runs.
BASE_URL = _text("DIZKOVER_BASE_URL", "https://www.dizkover.com")
LOGIN_PATH = _text("DIZKOVER_LOGIN_PATH", "/site/login")
LOGIN_URL = _text("DIZKOVER_LOGIN_URL", BASE_URL + LOGIN_PATH)
HOME_URL = BASE_URL + "/"
LOGIN_NOTIFY_URL = _text("DIZKOVER_LOGIN_NOTIFY_URL", BASE_URL + "/ajax/loginnotify")

# ---- Browser --------------------------------------------------------------
HEADLESS = _flag("DIZKOVER_HEADLESS", True)
# Optional: full path to chrome.exe / chromium. Empty = Playwright's bundled
# Chromium (installed by install.bat). Useful if you already have Chrome/Edge.
CHROMIUM_EXECUTABLE = _text("DIZKOVER_CHROMIUM", "")
MAX_TABS = _number("DIZKOVER_TABS", 5)
TIMEOUT = 30000          # milliseconds, page navigation / selectors
RESULT_TIMEOUT = 15000   # milliseconds, wait for login result after submit
MAX_RETRY = 2            # only network/unknown errors are retried

# A real browser UA is safer than a random one: a UA that does not match the
# actual Chromium build is an easy bot signal. Enable only if you know why.
USE_RANDOM_UA = False
USER_AGENTS = [
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36",
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36",
]

VIEWPORTS = [
    {"width": 1920, "height": 1080},
    {"width": 1366, "height": 768},
    {"width": 1536, "height": 864},
]

# The old checker set a New York timezone from Bangladesh and randomised the
# UA; both are detectable. Keep the machine's real values by default.
SET_TIMEZONE = False
TIMEZONE_ID = "Asia/Dhaka"

# ---- Proxies --------------------------------------------------------------
# True  = use working rows from data/proxy.txt (falls back to Direct when the
#         file is empty or every proxy is dead).
# False = always connect directly.
USE_PROXY = _flag("DIZKOVER_USE_PROXY", True)
REQUIRE_PROXY = False    # True = do not fall back to Direct
PROXY_TIMEOUT = _number("DIZKOVER_PROXY_TIMEOUT", 10)
PROXY_TEST_URL = _text("DIZKOVER_PROXY_TEST_URL", "https://api.ipify.org?format=json")
VERIFY_PROXY_IN_BROWSER = True   # open PROXY_TEST_URL in the real context first

# ---- Timing ---------------------------------------------------------------
DELAY_MIN = 1.0
DELAY_MAX = 3.0

# ---- Diagnostics ----------------------------------------------------------
SAVE_DEBUG_ARTIFACTS = True      # screenshot + HTML for ERROR results
MAX_DEBUG_ARTIFACTS = 40         # keep the folder small
STEALTH_INIT_SCRIPT = True       # the behaviour already present in v2.0

# ---- Tests ----------------------------------------------------------------
RECORD_CPM_WINDOW = 60           # seconds
