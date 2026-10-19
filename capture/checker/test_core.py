"""Browser-free tests for the checker's pure logic.

Run from this folder:

    python -m unittest -v test_core.py      (Windows: py -3 -m unittest -v test_core.py)

No Playwright, Chromium, network or credentials are used.
"""

import unittest

from core import (
    BANNED,
    ERROR,
    INVALID,
    VALID,
    classify_login_result,
    format_duration,
    is_login_url,
    parse_account_line,
    parse_proxy,
    safe_filename,
    strip_ansi,
)


class AccountParsingTests(unittest.TestCase):
    def test_normal(self):
        self.assertEqual(parse_account_line("alice:secret"), ("alice", "secret"))

    def test_password_may_contain_colon(self):
        self.assertEqual(
            parse_account_line("alice:pa:ss:word"), ("alice", "pa:ss:word")
        )

    def test_comment_and_blank_are_ignored(self):
        self.assertIsNone(parse_account_line("# note"))
        self.assertIsNone(parse_account_line("   "))
        self.assertIsNone(parse_account_line("no-password"))

    def test_missing_parts_are_ignored(self):
        self.assertIsNone(parse_account_line(":secret"))
        self.assertIsNone(parse_account_line("alice:"))

    def test_surrounding_whitespace_is_trimmed(self):
        self.assertEqual(parse_account_line("  alice:secret  "), ("alice", "secret"))


class ProxyParsingTests(unittest.TestCase):
    def test_host_port(self):
        proxy = parse_proxy("1.2.3.4:8080")
        self.assertIsNotNone(proxy)
        self.assertEqual(proxy.server, "http://1.2.3.4:8080")
        self.assertIsNone(proxy.username)

    def test_scheme_is_kept(self):
        proxy = parse_proxy("socks5://5.6.7.8:1080")
        self.assertEqual(proxy.server, "socks5://5.6.7.8:1080")
        self.assertEqual(proxy.requests_url(), "socks5://5.6.7.8:1080")

    def test_credentials(self):
        proxy = parse_proxy("user:pass@1.2.3.4:3128")
        self.assertEqual(proxy.server, "http://1.2.3.4:3128")
        self.assertEqual(proxy.username, "user")
        self.assertEqual(proxy.password, "pass")
        self.assertEqual(proxy.requests_url(), "http://user:pass@1.2.3.4:3128")

    def test_credentials_are_percent_encoded_for_requests(self):
        proxy = parse_proxy("u@ser:p@ss@1.2.3.4:3128")
        self.assertEqual(proxy.username, "u@ser")
        self.assertEqual(proxy.password, "p@ss")
        self.assertEqual(proxy.requests_url(), "http://u%40ser:p%40ss@1.2.3.4:3128")

    def test_playwright_shape(self):
        proxy = parse_proxy("user:pass@1.2.3.4:3128")
        self.assertEqual(
            proxy.playwright(),
            {
                "server": "http://1.2.3.4:3128",
                "username": "user",
                "password": "pass",
            },
        )

    def test_invalid_rows(self):
        for row in ("", "# comment", "1.2.3.4", "1.2.3.4:notaport", "1.2.3.4:0", ":8080"):
            self.assertIsNone(parse_proxy(row), row)


class LoginUrlTests(unittest.TestCase):
    def test_login_paths(self):
        self.assertTrue(is_login_url("https://www.dizkover.com/site/login"))
        self.assertTrue(is_login_url("https://www.dizkover.com/site/login/"))
        self.assertTrue(is_login_url("https://www.dizkover.com/site/login?next=/"))
        self.assertFalse(is_login_url("https://www.dizkover.com/"))
        self.assertFalse(is_login_url("https://www.dizkover.com/u/alice"))
        self.assertFalse(is_login_url(""))


class ClassificationTests(unittest.TestCase):
    def test_redirect_is_valid(self):
        result = classify_login_result(
            url_before="https://www.dizkover.com/site/login",
            url_after="https://www.dizkover.com/",
        )
        self.assertEqual(result.status, VALID)
        self.assertEqual(result.step, "redirect")

    def test_loginnotify_out_1_is_valid(self):
        result = classify_login_result(
            url_before="https://www.dizkover.com/site/login",
            url_after="https://www.dizkover.com/site/login",
            api_out=1,
        )
        self.assertEqual(result.status, VALID)
        self.assertEqual(result.step, "api")

    def test_error_message_is_invalid(self):
        result = classify_login_result(
            url_before="https://www.dizkover.com/site/login",
            url_after="https://www.dizkover.com/site/login",
            error_text="Incorrect username or password.",
        )
        self.assertEqual(result.status, INVALID)
        self.assertEqual(result.detail, "Incorrect username or password.")

    def test_loginnotify_out_0_is_invalid(self):
        result = classify_login_result(
            url_before="https://www.dizkover.com/site/login",
            url_after="https://www.dizkover.com/site/login",
            api_out=0,
        )
        self.assertEqual(result.status, INVALID)
        self.assertEqual(result.step, "api")

    def test_api_answer_overrides_a_redirect(self):
        """A redirect to /site/error must not look like a valid login."""
        result = classify_login_result(
            url_before="https://www.dizkover.com/site/login",
            url_after="https://www.dizkover.com/site/error",
            api_out=0,
        )
        self.assertEqual(result.status, INVALID)
        self.assertEqual(result.step, "api")

    def test_unconfirmed_is_error_not_invalid(self):
        """No redirect, no error text, no API answer: never guess."""
        result = classify_login_result(
            url_before="https://www.dizkover.com/site/login",
            url_after="https://www.dizkover.com/site/login",
        )
        self.assertEqual(result.status, ERROR)
        self.assertEqual(result.step, "confirm")

    def test_captcha_is_error(self):
        result = classify_login_result(
            url_before="https://www.dizkover.com/site/login",
            url_after="https://www.dizkover.com/site/login",
            captcha=True,
        )
        self.assertEqual(result.status, ERROR)
        self.assertEqual(result.detail, "CAPTCHA challenge shown")

    def test_captcha_wins_over_stale_error_text(self):
        result = classify_login_result(
            url_before="https://www.dizkover.com/site/login",
            url_after="https://www.dizkover.com/site/login",
            captcha=True,
            error_text="old error",
        )
        self.assertEqual(result.status, ERROR)

    def test_banned_wins(self):
        result = classify_login_result(
            url_before="https://www.dizkover.com/site/login",
            url_after="https://www.dizkover.com/site/login",
            banned=True,
            error_text="anything",
        )
        self.assertEqual(result.status, BANNED)

    def test_whitespace_in_error_is_collapsed(self):
        result = classify_login_result(
            url_before="https://www.dizkover.com/site/login",
            url_after="https://www.dizkover.com/site/login",
            error_text="Wrong\n   password   ",
        )
        self.assertEqual(result.detail, "Wrong password")


class MiscTests(unittest.TestCase):
    def test_strip_ansi(self):
        self.assertEqual(strip_ansi("\x1b[32mOK\x1b[0m"), "OK")

    def test_safe_filename(self):
        self.assertEqual(safe_filename("a/b c:?*"), "a_b_c")
        self.assertEqual(safe_filename(""), "account")
        self.assertLessEqual(len(safe_filename("x" * 100)), 40)

    def test_format_duration(self):
        self.assertEqual(format_duration(5), "5s")
        self.assertEqual(format_duration(65), "1m 5s")
        self.assertEqual(format_duration(3665), "1h 1m 5s")


if __name__ == "__main__":
    unittest.main()
