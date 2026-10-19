"""Flow tests with a fake Playwright page (no browser, no network).

These cover the orchestration in main.check_account: form discovery, submit,
result polling, the loginnotify fallback, CAPTCHA/banned detection and the
"never guess INVALID" rule. They do not replace a real run on Windows.
"""

import asyncio
import tempfile
import unittest
from pathlib import Path

import config
import main


class FakeLocator:
    def __init__(self, page, selector):
        self.page = page
        self.selector = selector

    @property
    def first(self):
        return self

    def nth(self, _index):
        return self

    async def count(self):
        return 1 if self.page.has(self.selector) else 0

    async def is_visible(self):
        return self.page.has(self.selector)

    async def inner_text(self):
        return self.page.text_for(self.selector)

    async def click(self):
        await self.page.click(self.selector)

    async def fill(self, value):
        self.page.filled[self.selector] = value

    async def bounding_box(self):
        return self.page.boxes.get(self.selector)

    async def get_attribute(self, name):
        return self.page.attributes.get(self.selector, {}).get(name)


class FakeKeyboard:
    def __init__(self, page):
        self.page = page

    async def press(self, key):
        self.page.keys.append(key)


class FakeResponse:
    status = 200


class FakePage:
    """Minimal page that behaves like the Dizkover login page."""

    def __init__(self, mode="success"):
        self.mode = mode
        self.url = config.LOGIN_URL
        self.filled = {}
        self.keys = []
        self.boxes = {}
        self.attributes = {}
        self.api_response = {"out": 1}
        self.body = "home"

    def has(self, selector):
        if selector in main.USER_SELECTORS:
            return selector == main.USER_SELECTORS[0]
        if selector in main.PASS_SELECTORS:
            return selector == main.PASS_SELECTORS[0]
        if selector in main.SUBMIT_SELECTORS:
            return selector == main.SUBMIT_SELECTORS[0]
        if selector == ".help-block":
            return self.mode == "invalid"
        if selector == ".rc-imageselect":
            return self.mode == "captcha"
        return False

    def text_for(self, selector):
        if selector == ".help-block":
            return "Incorrect username or password."
        return ""

    async def click(self, selector):
        if selector == main.SUBMIT_SELECTORS[0]:
            if self.mode == "success":
                self.url = config.HOME_URL
            # invalid/captcha/api modes stay on the login page

    async def fill(self, selector, value):
        self.filled[selector] = value

    def locator(self, selector):
        return FakeLocator(self, selector)

    async def query_selector(self, selector):
        # The real current page has no _csrf input; the checker must not care.
        return None

    async def goto(self, url, **_kwargs):
        self.url = url
        return FakeResponse()

    async def title(self):
        return "Dizkover - Login"

    async def add_init_script(self, _script):
        return None

    def set_default_timeout(self, _timeout):
        return None

    async def inner_text(self, selector):
        if selector == "body":
            return self.body
        return self.text_for(selector)

    async def content(self):
        return "<html><body>{}</body></html>".format(self.body)

    async def screenshot(self, path, **_kwargs):
        Path(path).write_bytes(b"fake")

    async def evaluate(self, _script, _arg):
        if isinstance(self.api_response, Exception):
            raise self.api_response
        return self.api_response

    @property
    def keyboard(self):
        return FakeKeyboard(self)


class FakeContext:
    def __init__(self, page):
        self.page = page

    async def new_page(self):
        return self.page

    async def cookies(self):
        return [{"name": "PHPSESSID", "value": "test"}]

    async def close(self):
        return None


class CheckAccountFlowTests(unittest.IsolatedAsyncioTestCase):
    def setUp(self):
        self.tempdir = tempfile.TemporaryDirectory()
        self.addCleanup(self.tempdir.cleanup)
        self._old_data_dir = main.DATA_DIR
        self._old_save_debug = config.SAVE_DEBUG_ARTIFACTS
        self._old_timeout = config.TIMEOUT
        self._old_result_timeout = config.RESULT_TIMEOUT
        self._old_verify_proxy = config.VERIFY_PROXY_IN_BROWSER

        main.DATA_DIR = Path(self.tempdir.name)
        config.SAVE_DEBUG_ARTIFACTS = False
        config.TIMEOUT = 2000
        config.RESULT_TIMEOUT = 800
        config.VERIFY_PROXY_IN_BROWSER = False
        self.logger = main.Logger(Path(self.tempdir.name) / "logs.txt")

    def tearDown(self):
        main.DATA_DIR = self._old_data_dir
        config.SAVE_DEBUG_ARTIFACTS = self._old_save_debug
        config.TIMEOUT = self._old_timeout
        config.RESULT_TIMEOUT = self._old_result_timeout
        config.VERIFY_PROXY_IN_BROWSER = self._old_verify_proxy

    async def run_page(self, page):
        return await main.check_account(
            FakeContext(page), "tester", "secret", None, self.logger
        )

    async def test_success_without_csrf(self):
        page = FakePage("success")
        outcome = await self.run_page(page)
        self.assertEqual(outcome.status, "VALID")
        self.assertEqual(page.filled[main.USER_SELECTORS[0]], "tester")
        self.assertEqual(page.filled[main.PASS_SELECTORS[0]], "secret")
        # The password must never reach the log file.
        log_text = (Path(self.tempdir.name) / "logs.txt").read_text(encoding="utf-8")
        self.assertNotIn("secret", log_text)

    async def test_redirect_fallback_when_api_offline(self):
        page = FakePage("success")
        page.api_response = RuntimeError("offline")
        outcome = await self.run_page(page)
        self.assertEqual(outcome.status, "VALID")
        self.assertEqual(outcome.step, "redirect")

    async def test_invalid_error_message(self):
        page = FakePage("invalid")
        outcome = await self.run_page(page)
        self.assertEqual(outcome.status, "INVALID")
        self.assertEqual(outcome.detail, "Incorrect username or password.")

    async def test_api_zero_is_invalid(self):
        page = FakePage("api")
        page.api_response = {"out": 0}
        outcome = await self.run_page(page)
        self.assertEqual(outcome.status, "INVALID")
        self.assertEqual(outcome.step, "api")

    async def test_captcha_is_error_not_invalid(self):
        page = FakePage("captcha")
        outcome = await self.run_page(page)
        self.assertEqual(outcome.status, "ERROR")
        self.assertEqual(outcome.step, "captcha")

    async def test_anti_bot_challenge_is_error(self):
        page = FakePage("success")
        page.body = "Just a moment... checking your browser"
        outcome = await self.run_page(page)
        self.assertEqual(outcome.status, "ERROR")
        self.assertEqual(outcome.step, "challenge_check")

    async def test_unconfirmed_is_error(self):
        page = FakePage("api")
        page.api_response = {"raw": "<html>changed</html>"}
        outcome = await self.run_page(page)
        self.assertEqual(outcome.status, "ERROR")
        self.assertEqual(outcome.step, "confirm")


if __name__ == "__main__":
    unittest.main()
