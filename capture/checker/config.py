# Dizkover account checker - configuration.
#
# Paths below are relative to the checker's data/ folder, which is created by
# install.bat. Runtime files never live next to the source, so they cannot be
# committed or packed into a source archive by accident.

# ---- Files (inside data/) -------------------------------------------------
ACCOUNTS_FILE = "ac.txt"
PROXY_FILE = "proxy.txt"
VALID_FILE = "valid.txt"
INVALID_FILE = "invalid.txt"
BANNED_FILE = "banned.txt"
ERRORS_FILE = "errors.txt"
LOG_FILE = "logs.txt"
DEBUG_DIR = "debug"

# ---- Target ---------------------------------------------------------------
LOGIN_URL = "https://www.dizkover.com/site/login"
LOGIN_PATH = "/site/login"
HOME_URL = "https://www.dizkover.com/"
LOGIN_NOTIFY_URL = "https://www.dizkover.com/ajax/loginnotify"

# ---- Browser --------------------------------------------------------------
HEADLESS = True
MAX_TABS = 5
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
USE_PROXY = True
REQUIRE_PROXY = False    # True = do not fall back to Direct
PROXY_TIMEOUT = 10       # seconds per health check
PROXY_TEST_URL = "https://api.ipify.org?format=json"
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
