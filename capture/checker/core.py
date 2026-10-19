"""Pure helpers for the Dizkover account checker.

No third-party imports here, so `python -m unittest` works before Playwright is
installed. Everything that can be tested without a browser lives in this file.
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Optional, Tuple
from urllib.parse import quote, urlsplit

LOGIN_PATH = "/site/login"

VALID = "VALID"
INVALID = "INVALID"
BANNED = "BANNED"
ERROR = "ERROR"

_ANSI_RE = re.compile(r"\x1b\[[0-9;]*m")
_SPACE_RE = re.compile(r"\s+")


@dataclass(frozen=True)
class LoginOutcome:
    status: str
    detail: str
    step: str


@dataclass(frozen=True)
class ProxyConfig:
    """A parsed proxy line.

    `server` is the value Playwright expects (scheme://host:port).
    `host`/`port`/credentials are kept for the requests-based health check.
    """

    server: str
    host: str
    port: int
    scheme: str = "http"
    username: Optional[str] = None
    password: Optional[str] = None

    def playwright(self) -> dict:
        data = {"server": self.server}
        if self.username:
            data["username"] = self.username
            data["password"] = self.password or ""
        return data

    def requests_url(self) -> str:
        """URL form for requests, with credentials percent-encoded."""
        auth = ""
        if self.username:
            auth = "{}:{}@".format(
                quote(self.username, safe=""),
                quote(self.password or "", safe=""),
            )
        return "{}://{}{}:{}".format(self.scheme, auth, self.host, self.port)


def strip_ansi(text: str) -> str:
    return _ANSI_RE.sub("", text)


def collapse_whitespace(text: str) -> str:
    return _SPACE_RE.sub(" ", (text or "").strip())


def safe_filename(text: str, limit: int = 40) -> str:
    cleaned = re.sub(r"[^A-Za-z0-9._-]+", "_", (text or "").strip())
    cleaned = cleaned.strip("._-")
    return (cleaned or "account")[:limit]


def parse_account_line(line: str) -> Optional[Tuple[str, str]]:
    """Parse `username:password` (password may contain `:`).

    Returns None for blank lines, comments and malformed rows. The password is
    never logged by callers; only the username is used in diagnostics.
    """
    raw = (line or "").strip()
    if not raw or raw.startswith("#"):
        return None
    if ":" not in raw:
        return None
    username, password = raw.split(":", 1)
    username = username.strip()
    password = password.strip()
    if not username or not password:
        return None
    return username, password


def parse_proxy(line: str) -> Optional[ProxyConfig]:
    """Accept all formats used in the old checker and in proxy lists.

        host:port
        user:pass@host:port
        http://host:port
        socks5://user:pass@host:port
    """
    raw = (line or "").strip()
    if not raw or raw.startswith("#"):
        return None

    scheme = "http"
    if "://" in raw:
        scheme, raw = raw.split("://", 1)
        scheme = scheme.strip().lower() or "http"

    username = None
    password = None
    if "@" in raw:
        auth, raw = raw.rsplit("@", 1)
        if ":" in auth:
            username, password = auth.split(":", 1)
            username = username.strip()
            password = password.strip()
        else:
            username = auth.strip()
            password = ""

    if raw.count(":") != 1:
        return None
    host, port_text = raw.rsplit(":", 1)
    host = host.strip().strip("[]")
    try:
        port = int(port_text.strip())
    except (TypeError, ValueError):
        return None
    if not host or not 1 <= port <= 65535:
        return None

    return ProxyConfig(
        server="{}://{}:{}".format(scheme, host, port),
        host=host,
        port=port,
        scheme=scheme,
        username=username,
        password=password,
    )


def is_login_url(url: str, login_path: str = LOGIN_PATH) -> bool:
    """True when the URL path is the login page (query/trailing slash safe)."""
    if not url:
        return False
    try:
        path = urlsplit(url).path or "/"
    except ValueError:
        return False
    path = path.rstrip("/") or "/"
    return path == (login_path.rstrip("/") or "/")


def classify_login_result(
    *,
    url_before: str,
    url_after: str,
    error_text: str = "",
    captcha: bool = False,
    banned: bool = False,
    api_out: Optional[int] = None,
    login_path: str = LOGIN_PATH,
) -> LoginOutcome:
    """Decide the account status from observable facts only.

    The ordering matters:
      1. banned page
      2. a visible CAPTCHA challenge (never a hidden reCAPTCHA script)
      3. an explicit site error message -> INVALID
      4. /ajax/loginnotify is authoritative: out=1 -> VALID, anything else -> INVALID
      5. the browser left /site/login (API unavailable) -> VALID
      6. still on the login page with no signal -> ERROR (needs a debug artifact),
         because claiming INVALID here would be a guess.
    """
    if banned:
        return LoginOutcome(BANNED, "account is banned", "banned")

    if captcha:
        return LoginOutcome(ERROR, "CAPTCHA challenge shown", "captcha")

    detail = collapse_whitespace(error_text)[:160]
    if detail:
        return LoginOutcome(INVALID, detail, "error_message")

    # The site's own post-login endpoint is the strongest signal. Treat it as
    # authoritative when it answers, so a redirect to an error page can never
    # be mistaken for a successful login.
    if api_out is not None:
        if api_out == 1:
            return LoginOutcome(VALID, "loginnotify out=1", "api")
        return LoginOutcome(INVALID, "loginnotify out={}".format(api_out), "api")

    if (
        url_after
        and url_after.lower().startswith(("http://", "https://"))
        and not is_login_url(url_after, login_path)
    ):
        return LoginOutcome(VALID, "redirected to {}".format(url_after), "redirect")

    return LoginOutcome(
        ERROR,
        "login not confirmed (still on login page, no error message)",
        "confirm",
    )


def format_duration(seconds: float) -> str:
    seconds = max(0, int(seconds))
    minutes, secs = divmod(seconds, 60)
    hours, minutes = divmod(minutes, 60)
    if hours:
        return "{}h {}m {}s".format(hours, minutes, secs)
    if minutes:
        return "{}m {}s".format(minutes, secs)
    return "{}s".format(secs)


def unique_preserving_order(items):
    seen = set()
    out = []
    for item in items:
        if item not in seen:
            seen.add(item)
            out.append(item)
    return out
