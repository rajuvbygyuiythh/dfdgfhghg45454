"""Local replica of the Dizkover login page for the browser self-test.

Behaviour mirrors the captured traffic in `capture/checker/README.md`:

    GET  /site/login           -> form with LoginForm[user_name] / [user_password]
                                  (deliberately NO _csrf input, like the real page)
    POST /site/login           -> 302 to / on success; the same page plus a
                                  `.help-block` error on bad credentials
    GET  /ajax/loginnotify     -> JSON {"out": 1}

Test accounts:

    gooduser:goodpass   -> VALID
    baduser:badpass     -> INVALID (error message)
    banneduser:bannedpass -> BANNED (redirect to /banned)
    captchauser:captchapass -> ERROR (visible reCAPTCHA challenge)

Nothing here talks to the internet and no real credential is used.
"""

from __future__ import annotations

import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs

LOGIN_PAGE = """<!DOCTYPE html>
<html><head><title>Dizkover - Login</title></head>
<body>
<div class="site-login">
  <form id="login-form" action="/site/login" method="post">
    <input type="text" name="LoginForm[user_name]" id="loginform-user_name">
    <input type="password" name="LoginForm[user_password]" id="loginform-user_password">
    <button type="submit" class="btn btn-primary">Log in</button>
  </form>
  {error}
</div>
</body></html>
"""

ERROR_BLOCK = '<div class="help-block">Incorrect username or password.</div>'

CAPTCHA_PAGE = """<!DOCTYPE html>
<html><head><title>Dizkover - Login</title></head>
<body>
<div class="site-login">
  <form action="/site/login" method="post">
    <input type="text" name="LoginForm[user_name]">
    <input type="password" name="LoginForm[user_password]">
    <button type="submit" class="btn btn-primary">Log in</button>
  </form>
  <div class="rc-imageselect" style="width:320px;height:120px">
    Select all images with traffic lights
  </div>
</div>
</body></html>
"""

HOME_PAGE = "<!DOCTYPE html><html><head><title>Dizkover</title></head><body><h1>Home feed</h1></body></html>"

BANNED_PAGE = (
    "<!DOCTYPE html><html><head><title>Banned</title></head><body>"
    "<h1>You are banned</h1><p>Reason: test</p></body></html>"
)

VALID = {"gooduser": "goodpass"}
BANNED = {"banneduser": "bannedpass"}
CAPTCHA = {"captchauser": "captchapass"}


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_args):  # keep the self-test output clean
        pass

    def _send(self, status, body, content_type="text/html; charset=utf-8", headers=None):
        data = body.encode("utf-8") if isinstance(body, str) else body
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(data)))
        for key, value in (headers or {}).items():
            self.send_header(key, value)
        self.end_headers()
        self.wfile.write(data)

    def _redirect(self, location):
        self.send_response(302)
        self.send_header("Location", location)
        self.send_header("Content-Length", "0")
        self.end_headers()

    def do_GET(self):
        path = self.path.split("?")[0].rstrip("/") or "/"
        if path == "/site/login":
            self._send(200, LOGIN_PAGE.format(error=""))
        elif path == "/ajax/loginnotify":
            self._send(
                200,
                json.dumps({"out": 1}),
                content_type="application/json",
            )
        elif path == "/banned":
            self._send(200, BANNED_PAGE)
        elif path == "/":
            self._send(200, HOME_PAGE)
        else:
            self._send(404, "<h1>Not found</h1>")

    def do_POST(self):
        path = self.path.split("?")[0].rstrip("/") or "/"
        length = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(length).decode("utf-8", "replace")
        form = parse_qs(raw)
        user = (form.get("LoginForm[user_name]") or [""])[0].strip()
        password = (form.get("LoginForm[user_password]") or [""])[0]

        if path != "/site/login":
            self._send(404, "<h1>Not found</h1>")
            return

        if VALID.get(user) == password:
            self._redirect("/")
        elif BANNED.get(user) == password:
            self._redirect("/banned")
        elif CAPTCHA.get(user) == password:
            self._send(200, CAPTCHA_PAGE)
        else:
            self._send(200, LOGIN_PAGE.format(error=ERROR_BLOCK))


class Replica:
    """Context manager that serves the replica on a free localhost port."""

    def __init__(self):
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.port = self.server.server_address[1]

    @property
    def base_url(self):
        return "http://127.0.0.1:{}".format(self.port)

    def __enter__(self):
        self.thread.start()
        return self

    def __exit__(self, *_exc):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)
        return False


if __name__ == "__main__":
    with Replica() as replica:
        print("Replica running on {}".format(replica.base_url))
        print("Press Ctrl+C to stop")
        try:
            replica.thread.join()
        except KeyboardInterrupt:
            pass
