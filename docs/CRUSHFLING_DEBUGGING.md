# CrushFling Auto Register — debugging guide (বাংলা + English)

> **দ্রুত সারাংশ (বাংলা):** প্রতিটি run এখন `logs/run-*.log` ফাইলে লেখে।
> `--debug` দিলে প্রতিটি failed account-এর জন্য `debug/fail-*.json` (step,
> error code, HTTP trace, log tail) তৈরি হয়। `--trace-http` দিলে প্রতিটি
> request/response body (truncated + token redacted) `debug/trace-*.jsonl`-এ
> যায়। Dashboard-এ **Open log folder** আর **Debug report** বোতাম আছে।
> Cookie/session token কোথাও লেখা হয় না — সব লগ/ডাম্প লেখার আগে redact হয়।

---

## 1. What is written where

| File | When | Contents | Contains secrets? |
|------|------|----------|-------------------|
| `<data>/logs/run-<id>.log` | every run | level, timestamp, step timings, endpoint status lines | no — redacted |
| `<data>/debug/fail-<time>-<idx>.json` | `--debug`, per failed account | step, code, error, timings, profile, endpoints, HTTP records, 120 log lines | no — redacted, cookies excluded |
| `<data>/debug/trace-<id>.jsonl` | `--trace-http` | one JSON line per request hop: method, URL, status, duration, redirect, Set-Cookie **names**, bodies (32 KiB cap) | no — tokens/cookies/emails redacted |
| `<data>/debug/report-<id>.txt` | dashboard **Debug report** button | stats, error codes, last run config, endpoints, counts, last 80 log lines | no — redacted |
| `<data>/debug/verify-*.png` / `.html` | `--debug` + browser fallback fails | screenshot + DOM of the page the browser actually reached | no — HTML is redacted |

CLI data directory defaults to `crushfling/data/`; the Wails dashboard uses the
app data directory + `crushfling/` (shown as `data/crushfling` in the panel).

```text
crushfling/data/
  logs/run-20261020-141502-118.log
  debug/fail-20261020-141533-00.json
  debug/trace-20261020-141502-118.jsonl
  debug/report-20261020-142010-402.txt
  debug/verify-20261020-141533.png     <- only when the browser fallback failed
  debug/verify-20261020-141533.html
```

Both `logs/` and `debug/` live under the gitignored `data/` folder.

---

## 2. Running with diagnostics

```bat
:: offline pre-flight: folders, api.json, captions, pictures, logs (no network)
go run ./cmd/crushfling --check

:: normal run: info lines + log file
go run ./cmd/crushfling --count 1 --email mailtm

:: debug: debug lines, step timings, failure dumps, mail-poll diagnostics
go run ./cmd/crushfling --count 1 --email mailtm --debug

:: full: also record HTTP bodies (the endpoint-calibration tool)
go run ./cmd/crushfling --count 1 --email mailtm --debug --trace-http
```

`-v` is an alias for `--debug`. The startup banner prints the exact log path,
and the end of the run prints the trace path when enabled.

### Node diagnostic (no Go toolchain)

```bat
node crushfling/run.js --count 1 --email mailtm --verbose
```

It checks temp mail + the same-session magic-link click only. `--verbose`
prints each request/status, links are masked and only a verified account is
appended to `accounts.json`; there is no trace/dump file (the Go worker is the
full implementation).

### First: what does the site do today?

```bat
go run ./cmd/crushfling --probe
```

One GET, no registration. It prints the detected sign-up style, whether a CSRF
token was found, the form action / Livewire method, the upload and profile
paths the page references, the cookie names and the raw evidence lines. Most
"nothing works" reports are answered here before any account is created.

### Dashboard

If the sidebar button is missing (fresh checkout, `dist/` regenerated), restore
the overlay first:

```bat
npm run overlay:sync
npm run overlay:check
```

1. Sidebar → **💘 CrushFling Auto Register**
2. Leave **Debug logs + failure dumps** checked.
3. Enable **Trace HTTP bodies** only while calibrating — it writes more data.
4. **Open log folder** opens `logs/`; **Debug report** writes and shows a
   `report-*.txt` path.
5. The log filter (`All / Debug+ / Info+ / Warnings / Errors`) works on the
   level prefix inside each line, e.g. `[14:15:33] warn: ...`.

---

## 3. Log format and what to look for

```text
2026-10-20 14:15:02.118 [INFO ] run 20261020-141502-118: 1 account(s), concurrency=1, provider=mailtm, log=...
2026-10-20 14:15:02.902 [INFO ] [0] Temp mail created: a***@mail.tm via mail.tm
2026-10-20 14:15:03.355 [DEBUG] [0] CSRF token: 4f9a1c2b3d4e (cookie names: __Host-next-auth.csrf-token,cf_clearance)
2026-10-20 14:15:03.612 [DEBUG] magic request accepted: /api/auth/signin/email -> 200
2026-10-20 14:15:06.744 [DEBUG] [0] step magic_request took 3390ms
2026-10-20 14:15:09.100 [INFO ] [0] Magic link ready: https://crushfling.com/api/auth/callback/email?token=***...
2026-10-20 14:15:09.530 [INFO ] [0] Magic link clicked, cookies: 4 (cf_clearance,next-auth.csrf-token,next-auth.session-token,...), body len 4211
2026-10-20 14:15:09.900 [INFO ] [0] Profile created: sweet_emma42 age 22 US -> telegram
2026-10-20 14:15:09.901 [INFO ] [0] done in 7783ms verified=true
```

Every account logs `step <name> took <n>ms` at debug level with these names:

`inbox`, `session`, `csrf`, `magic_request`, `mail`, `magic_click`, `verify`,
`upload`, `profile`. The timings are also stored in `AccountResult.timings`
(with `total`), so a slow step is visible in `accounts.json` and the dumps.

### Status lines

| Log line | Meaning |
|----------|---------|
| `magic request accepted: <path> -> <status>` | the sign-in request worked; this is the real endpoint |
| `http POST <path> -> 404 (123ms, 0B)` | wrong path — calibrate `api.json` |
| `http GET <link> -> 200` + `cookies: ...session-token` | the same-session click worked |
| `cookies: 2 (cf_clearance,csrf-token)` | link clicked, but **no session cookie** → verify step / browser fallback |
| `FAILED step=... code=... error=...` | classification of an account that was not saved |
| `INCOMPLETE step=... code=... error=... (session valid, account saved)` | authenticated, but the profile step failed; check `PROFILE` |

---

## 4. Error codes

Codes are stable, short strings stored in `AccountResult.error_code`, counted in
`stats.by_code`, printed in `FAILED` lines and included in every dump.

| Code | Step | Typical cause | First thing to check |
|------|------|---------------|----------------------|
| `INBOX` | inbox | mail.tm/1secmail API down, rate-limited, no domain | `inbox poll error` lines, try `--email 1secmail` |
| `CSRF` | csrf | `/api/auth/csrf` missing/changed | trace status; edit `api.json` |
| `MAGIC_REQUEST` | magic_request | every candidate path rejected (404/400/error JSON) | `<data>/debug/trace-*.jsonl` statuses |
| `MAIL_POLL` | mail_poll | no email in 120 s | poll diagnostics, spam folder, provider |
| `BAD_LINK` | pasted/manual link | not `crushfling.com`, or nothing pasted | use the full `https://` link |
| `MAGIC_CLICK` | magic_click | link expired, Cloudflare challenge, different IP | keep one sticky proxy; enable `--browser-verify` |
| `VERIFY` | verify | click returned 200 but no session cookie and browser verify failed | trace the callback response; capture the real verify path |
| `UPLOAD` | upload | avatar endpoint path/shape wrong (non-fatal) | trace `/api/upload` response |
| `PROFILE` | profile | all `profile_paths` failed; account saved, caption **not** consumed | trace, then edit `profile_paths` |
| `CANCELED` | any | Stop pressed / context timeout | expected during stop |
| `PANIC` | any | internal bug; stack trace is in the log | keep the dump + report and open an issue |
| `CONFIG` | session/config | local setup (data folder, session) | log error text |

Rules that matter when reading results:

- **`VERIFY`** means no authenticated cookie: nothing was saved and the caption
  was released back to `caption.txt`.
- **`PROFILE`** means the session is valid but the onboarding call failed: the
  account **is** saved (so you can finish it by hand), the picture is in
  `pictures/used/`, and the caption is released for a later account.
- Verified + profile complete → caption line deleted, picture moved to
  `pictures/used/`, account appended to `accounts.json` + `accounts.csv`.

---

## 5. Trace file (`trace-*.jsonl`)

One JSON object per HTTP hop (redirects produce multiple lines):

```json
{"time":"2026-10-20T08:15:03.610Z","method":"POST","url":"https://crushfling.com/api/auth/signin/email","status":200,"duration_ms":257,"request_bytes":118,"response_bytes":84,"set_cookies":["next-auth.csrf-token"],"request_body":"csrfToken=***&email=a***%40mail.tm&callbackUrl=https%3A%2F%2Fcrushfling.com%2F&json=true","response_body":"{\"url\":\"https://crushfling.com/api/auth/verifyRequest\"}"}
```

Useful commands (Windows `jq` or WSL):

```bash
# status of every request, in order
jq -r '.status + " " + .method + " " + (.url|split("?")[0])' debug/trace-*.jsonl

# only failures with their bodies
jq -r 'select(.status==0 or .status>=400) | "\(.status) \(.url)\n\(.response_body)\n"' debug/trace-*.jsonl

# which cookies the verify endpoint set
jq -r 'select(.set_cookies != null) | .url + " -> " + (.set_cookies|join(","))' debug/trace-*.jsonl
```

Limits: 32 KiB per body (a `truncated: true` flag marks cuts), binary bodies
are replaced by `[binary response, N bytes captured]`, and `multipart/form-data`
uploads are never read into the trace.

---

## 6. Failure dump (`fail-*.json`)

```json
{
  "time": "2026-10-20T08:15:33Z",
  "run_id": "20261020-141502-118",
  "index": 0,
  "email": "a***@mail.tm",
  "step": "profile",
  "code": "PROFILE",
  "error": "profile (PROFILE): profile endpoints failed (tried: /api/profile: HTTP 404 ...)",
  "verified": true,
  "duration_ms": 8123,
  "timings": {"inbox": 812, "csrf": 210, "magic_request": 3390, "mail": 6100, "magic_click": 420, "verify": 8, "upload": 900, "profile": 120, "total": 8123},
  "profile": {"username": "sweet_emma42", "email": "a***@mail.tm", "...": "..."},
  "magic_link": "https://crushfling.com/api/auth/callback/email?token=***&email=a***%40mail.tm",
  "proxy_used": "http://user:redacted@1.2.3.4:8080",
  "endpoints": {"csrf_path": "/api/auth/csrf", "...": "..."},
  "http": [ ... same shape as trace records ... ],
  "log_tail": ["debug: http POST /api/auth/signin/email -> 200 (257ms, 84B)", "..."]
}
```

`session_cookies`, `local_storage` and the account store are **never** part of a
dump. The whole JSON is passed through `Redact()` before writing.

---

## 7. Redaction — what is masked and what is not

Masked automatically in logs, traces, dumps and reports:

- query parameters `token=`, `code=`, `key=`, `secret=`, `sig=`, `session=`, `auth=`, `password=`
- JSON/JS keys `csrfToken`, `sessionToken`, `access_token`, `refresh_token`, `token`, `password`, `apiKey`, `captchaToken`
- `Cookie:`, `Set-Cookie:`, `Authorization:`, `X-Api-Key:` header values
- `Bearer <token>` values
- email local parts (`alice@mail.tm` → `a***@mail.tm`)
- proxy passwords in URLs (`user:pass@host` → `user:redacted@host`)

Not covered (by design, on purpose):

- `accounts.json` / `accounts.csv` must contain the real cookies to be usable —
  they stay in the gitignored data folder. Do not share those files.
- A raw `host:port:user:pass` proxy line in a **config echo** is not a URL;
  keep proxy files out of shared reports.
- Bodies can contain personal data of the profile (username, bio). Dumps are
  local files with mode `0600`; delete `debug/` before sending a repo archive.

Redaction is defence-in-depth, not a guarantee: review a dump before sharing it.

---

## 8. Local (offline) verification

These Go tests do not touch crushfling.com and do not need the network:

| Test | What it proves |
|------|----------------|
| `TestMagicLinkFlowWithFakeServer` | CSRF GET → sign-in form POST → callback GET → session cookie, all against a fake NextAuth v4 server |
| `TestMagicLinkFallsBackToAltPath` | the primary path 404s, the alternate path is really tried |
| `TestTracerRecordsStatusAndRedactsURL` | trace records status/Set-Cookie names and masks `?token=` |
| `TestFailureDumpIsRedactedAndValidJSON` | dumps are valid JSON with codes and no secrets |
| `TestRedactHidesCredentials` | the redaction matcher itself |
| `TestRunLoggerLevelsAndFile` | level filtering, redaction and file output |
| `TestServiceDebugReportRedacts` | dashboard report generation |
| `TestLaravelFormSignUpFlow` | discovery → form POST `_token` → magic click → `/account` proof → multipart upload → profile → `/p/<user>/`, and a 404 for an unknown user |
| `TestProfileRetriesAfterCSRFExpiry` | a 419 triggers exactly one refresh + retry, and the session ends with the new token |
| `TestFetchCSRFFailsLoudlyWithoutPath`, `TestRequestMagicLinkFailsLoudlyWithoutEndpoint` | a missing endpoint fails before any request leaves the process |
| `TestGeneratedProfilesMatchTheBrief` | 500 profiles: always woman, always looking for guys, only US/GB/AU/CA/DE |
| `TestDiscoverLaravelForm`, `TestDiscoverLivewire`, `TestDiscoverRejects404Page` | page parsing and the refusal to treat a 404 page as a sign-in page |

```bat
go test ./internal/crushfling/... -run "Magic|Trace|Dump|Redact|Logger|DebugReport" -v
```

The fake-server tests are deliberately strict about the wire format: the
form content type and `_token`, the cookie name (`laravel_session`), the
multipart field name (`file`) and the two spellings of the profile payload.
Those are the things that silently break a Laravel sign-up.

### End-to-end without the live site

`--base-url` points the whole binary at a mock (or a mirror), and
`--manual-link` lets you paste the verify URL that the mock would have mailed:

```bat
go run ./cmd/crushfling --count 1 --email custom --custom-email me@test.local ^
  --manual-link --base-url http://127.0.0.1:8099 --data .\tmp-data --debug
```

Expected: `sign-up flow detected`, `CSRF token received`, `Magic link clicked`,
`Avatar uploaded`, `Profile created`, `Public profile page confirmed`,
`Success: 1`, exit code `0`. This is exactly how the flow was proven on
2026-10-20 without touching crushfling.com.

---

## 9. Troubleshooting recipes

### `no magic-link endpoint is configured` / `no csrf_path configured`
The worker refused to guess. Run `--probe` (it never registers anything):
- `Style: form` + a `Form action` means discovery worked and this error should
  not appear; if it does, `api.json` pins an empty path — delete it.
- `FAILED: GET /account ... EOF/403` means the network or Cloudflare, not the
  code: retry with `--browser` on a normal connection.

### "All magic-link endpoints failed" / `MAGIC_REQUEST`
1. Run once with `--trace-http`.
2. Open the newest `debug/trace-*.jsonl` and look at the `POST` lines.
3. If every path is `404`, run `--probe` and copy the discovered form action /
   Livewire method into `<data>/api.json` → rerun; no rebuild needed.
4. If the status is `403` with a Cloudflare body, keep `--browser` and a sticky
   proxy, or lower concurrency.

### Link clicked but `VERIFY` / no session cookie
- Inspect `set_cookies` on the callback record. Empty means the link was
  already used, expired, or the request did not carry the requesting session.
- Confirm the click used the same client (it does by design) and that no proxy
  rotation happened between request and click.
- Enable **Browser verify** (`browser_verify`, on by default): Rod opens the
  link, clicks the page's **Sign In**, and copies the cookies back.
- With `--debug`, a failed browser attempt leaves `debug/verify-*.png` and
  `debug/verify-*.html`: open the screenshot to see whether it was a Cloudflare
  page, an expired-link page or an unknown onboarding form.
- If the browser also lands without a session cookie, the verify URL in
  `api.json`/the capture is wrong — check the capture's callback request.

### `MAIL_POLL` timeout
- Debug lines show `inbox poll N: no messages yet` or the provider error.
- mail.tm sometimes has no domains: try `--email 1secmail`.
- Check the address in the log (local part masked) — if it looks empty, inbox
  creation silently failed.
- Poll interval/timeout are 3 s / 120 s today (code defaults; the dashboard
  does not expose them yet — roadmap R6 adds them to `api.json`).

### `PROFILE` after a valid session
- The account is usable; finish it by hand or fix `profile_paths`.
- Trace shows each attempted path with status and a response snippet.
- The caption is intentionally **not** deleted, so no caption is wasted.

### 429 / rate limits
- The worker backs off 10 s, then retries up to 3 times.
- Lower concurrency, add proxies, or wait — the dashboard report shows how many
  accounts hit each code under `error codes`.

### Need to send a report
1. Dashboard → **Debug report** (or `cat data/crushfling/debug/report-*.txt`).
2. Attach the newest `fail-*.json` **after** opening it once to confirm nothing
   personal is inside.
3. Never attach `accounts.json`, `accounts.csv` or the whole `data/` folder.

---

## 10. What this sandbox still cannot verify

- Real endpoint paths, response shapes and Cloudflare behaviour: raw TLS to
  crushfling.com is reset here. The trace layer exists precisely so the first
  Windows run produces the evidence.
- Go compilation, `go vet`, `go test` and the Wails build: no Go toolchain in
  this sandbox. The syntax gate is tree-sitter-go; the offline Go tests above
  run on your machine.
- Live registration: **NOT RUN** in this environment.

See `docs/CRUSHFLING_AUTO_REGISTER.md` §9 for the exact verification table and
`docs/CRUSHFLING_ROADMAP.md` for the next improvements.
