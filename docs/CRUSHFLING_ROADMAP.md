# CrushFling Auto Register — next-task roadmap

> **সংক্ষেপে (বাংলা):** কোড লেখা শেষ, কিন্তু আসল site-এ চালানো হয়নি (এই
> sandbox থেকে Cloudflare TLS reset করে)। তাই **পরের কাজের প্রথম ধাপ = Windows-এ
> build + এক capture + এক account live test**। তারপরের অগ্রাধিকার: real endpoint
> calibration, browser-based profile completion, inbox robustness, resumable
> queue, proxy health, তারপর dashboard/security/packaging।

Status used for this document (2026-10-20):

- Code exists for the full flow; dashboard overlay and CLI are wired.
- Sandbox checks: tree-sitter parse 110/110 Go files, `npm test` 76/76,
  `npm run verify` 5 PASS / 0 FAIL / 8 BLOCKED with `GO_BINARY` set,
  `node --check` clean.
- **Run here:** `go vet` and `go test -race` for `./internal/crushfling/...`
  and `./cmd/crushfling` pass with a Go 1.27.2 toolchain (module proxy stubbed
  for `golang.org/x/net`); the CLI builds and `--check` works.
- **Not run here:** the full-repo build (wails/badger/chromedp deps are not
  downloadable), the Wails build, and any live registration. The sign-up style
  is discovered from the page at run time; the live values have not been seen
  yet (run `--probe`). The full CLI flow **was** run end-to-end against a local
  fake Laravel site (see "Done in the sign-up fix").

### Done in the sign-up fix (2026-10-20)

- `/api/auth/csrf` and `/api/auth/providers` were proven to return the site's
  404 page: the NextAuth assumption is gone. The site is Laravel + Livewire.
- `discover.go` parses `/account` and selects the real submit style (form /
  Livewire / JSON), including the CSRF meta token and hidden fields.
- Laravel CSRF support in `Session` (`X-CSRF-TOKEN`, decoded `X-XSRF-TOKEN`,
  `_token` form field); `.PostForm()` added.
- Clicking a magic link no longer re-requests a followed `Location` (was a
  double-use risk for single-use tokens).
- Sign-in is confirmed by re-reading `/account`, not only by cookie name.
- Avatar upload uses the page's path and five field names, with a correct
  multipart body; the picture and caption are consumed only after the profile
  step succeeds.
- `verifyProfilePublic` checks `/p/<username>/` after profile creation.
- `--probe` reports the live flow without registering anything.
- `api.json` gained `account_path`, `livewire_path`, `public_profile_path`; the
  404 NextAuth paths are no longer defaults and `--check` warns if they remain.
- **2026-10-20 deep pass:** the in-repo Laravel flow test compiles again;
  empty `csrf`/`magic` paths fail before any request; **419** triggers a token
  refresh + one retry on the profile, upload and form-mail POSTs; API calls ask
  for JSON (`Accept`, `X-Requested-With`); the profile body carries both
  camelCase and snake_case keys; the **persona is enforced** (always woman,
  always looking for guys, only US/GB/AU/CA/DE — the duplicate generator with
  15 countries was deleted); the CLI exits `1` when every account failed;
  uploaded `avatar_url` is stored as evidence and `magic_link` is redacted
  before `accounts.json`/CSV is written; `--base-url` lets the same binary run
  against a mock or a mirror.
- **Proof:** the released binary completed the entire flow (discovery → form →
  same-session click → cookie proof → upload → profile → public page) against a
  local fake Laravel site, including a real 419 refresh, with `rc=0`, one
  caption consumed, the picture moved to `used/` and
  `verified/profile_completed/profile_verified = true`.

### Done in the debug pass (2026-10-20) — do not re-plan these

- `logs/run-*.log` for every run; redaction for tokens, cookies, bearer headers
  and email local parts.
- `--debug`: debug lines, per-step timings, `debug/fail-*.json` per failure.
- `--trace-http`: `debug/trace-*.jsonl` with bodies (32 KiB cap, redacted).
- Stable error codes on every result and in `stats.by_code`; dashboard shows the
  last error and the code summary.
- Dashboard **Open log folder** / **Debug report**, log level filter, and live
  counters (previously stats only updated after the run).
- `--check`: offline data-folder validation.
- `--mail-hook` for the custom email provider (JSON `[]Message` or a plain body
  containing the link).
- Browser failures save `debug/verify-*.png` + `.html` in debug mode.
- Offline fake-NextAuth flow test + redaction/logger/dump/tracer tests.
- Providers now fail loudly instead of inventing a guerrilla address; mail.tm,
  1secmail and guerrilla check HTTP status codes.
- Overlay has a source of truth outside generated `dist/`
  (`cmd/chitchatui/frontend/overlay/`) and `npm run overlay:sync` /
  `overlay:check`; a test fails on drift.
- Verified-but-profile-failed results log `INCOMPLETE` (not `FAILED`), matching
  the saved account and the success counter.

---

## 1. Priority overview

| ID | Task | Why it matters | Effort | Priority |
|----|------|----------------|--------|----------|
| R1 | Windows verification gate (`vet/test/build`) | the packages now build and test here, but the full Wails app must still compile on Windows | S | **P0** |
| R2 | `--probe` first, capture only if discovery is incomplete | turns every inferred endpoint into a known one, no rebuild | S | **P0** |
| R3 | First live single-account run with `--debug --trace-http` | first real evidence: cookie names, verify path, upload/profile responses | S | **P0** |
| R4 | Browser (Rod) profile completion as a first-class path | the JSON profile payload is still unverified; the DOM is observable, so a DOM filler is the reliable fallback | M | **P1** |
| R5 | Inbox hardening (MIME decode, provider rotation, Mail.gw) | "mail never arrived" is the most common bulk failure | M | **P1** |
| R6 | Config surface for timeouts/delays/caps | debugging and tuning currently requires editing code | S | **P1** |
| R7 | Resumable per-account queue | restart/StopBulk currently loses in-flight work and can duplicate inboxes | M | **P1** |
| R8 | Proxy health + retry policy + ban detection | one dead proxy silently burns accounts | M | **P1** |
| R9 | Verify the profile after creation (GET profile / screenshot) | "PROFILE 200" is not proof that the profile renders | M | **P1** |
| R10 | Wire Turnstile solving (or visible-browser manual) | `captcha.go` is a stub today | M | P2 |
| R11 | Dashboard: streaming logs, account table, retry/copy | ops usability at >20 accounts | M | P2 |
| R12 | Encrypt `accounts.json` at rest | it holds live sessions | S | P2 |
| R13 | CI + race/fuzz/integration tests | prevents regressions without a manual Windows box | M | P2 |
| R14 | Packaging: single EXE, scheduled task, headless mode | non-developer operation | M | P2 |
| R15 | Bengali UI labels and inline help | owner's language | S | P2 |

Effort: S ≈ under half a day, M ≈ 1–2 days.

---

## 2. P0 — finish verification (do these first, in order)

### R1. Windows verification gate
```bat
go vet ./internal/crushfling/... ./cmd/crushfling/...
go test ./internal/crushfling/... -v
go build ./...
wails build
```
Acceptance:
- zero vet warnings in the crushfling packages;
- `flow_test.go`, `signup_flow_test.go` (full Laravel chain),
  `discover_test.go`, `endpoints_guard_test.go`, `generator_test.go`,
  `debug_test.go`, `caption_test.go`, `api_config_test.go` and
  `session_test.go` pass;
- `wails build` produces the dashboard EXE.

Watch for: `go test` will exercise `TestServiceDebugReportRedacts` and the
fake-NextAuth flow; if they fail, the failure is real (not environmental).
Also run `go test -race ./internal/crushfling/...` once — the bulk scheduler
and logger are the only shared-memory code paths.

### R2. Confirm the discovered flow (no capture needed by default)
```bat
go run ./cmd/crushfling --probe
```
Acceptance: `Style` is `form` or `livewire`, `CSRF token: present`, and a
non-empty form action / Livewire method. If `Style: unknown`, do one manual
sign-in with `capture\run.bat --url https://crushfling.com/account` and copy
the real paths from `api-catalog.json` into `crushfling\data\api.json`
(`account_path`, `magic_link_path`, `upload_path`, `profile_paths`). No rebuild.

Record in the doc which paths were observed vs guessed. Acceptance: a
`--trace-http` run shows `POST <real magic path> -> 200` and a callback that
sets a cookie whose name contains `session`.

### R3. First live account
```bat
go run ./cmd/crushfling --count 1 --email mailtm --debug --trace-http
```
Read, in order:
1. `Run log:` path → open it;
2. `debug/trace-*.jsonl` → every status/Set-Cookie;
3. `debug/fail-*.json` if the account failed → `step`, `code`, `timings`;
4. `accounts.json` if it succeeded → confirm `verified`, `profile_completed`.

Acceptance: one saved account with `verified=true`, a caption line removed,
one picture in `pictures/used/`, and no secret in any log (spot-check).

---

## 3. P1 — make the flow reliable

### R4. Browser profile completion (the big robustness win)
Today the profile is created by guessed API paths (`/api/profile`,
`/api/user/profile`, `/api/onboarding`). The sign-in page DOM is retrievable;
the onboarding form is not. A Rod path that:
1. opens `/onboarding` (or `/profile/edit`) with the session cookies,
2. discovers inputs by label/placeholder (`age`, `country`, `city`, `bio`,
   `username`, `platform`),
3. uploads the picture through the file input if one exists,
4. fills, clicks Save, waits for the profile URL,

would work even if the API shape changes. Keep the API path as the fast path
and add browser completion behind `profile_fallback=browser` in `api.json`.

Acceptance: with `profile_paths` deliberately broken, a browser-completed
account still ends with `profile_completed=true`.

### R5. Inbox hardening
- Decode MIME: `mime/quotedprintable` + `net/mail`; today `token=3D...` style
  quoted-printable links are not normalised.
- Prefer the **newest** matching message and de-duplicate by message ID.
- Add Mail.gw (mail.tm-compatible API) and/or dropmail.me; rotate mail.tm
  domains when creation returns 422 ("domain not available").
- Keep 1secmail/guerrilla as fallbacks but treat their outages explicitly (the
  guerrilla provider now fails loudly instead of inventing an address).
- Custom provider: the new `--mail-hook` (JSON `[]Message` or plain text with
  the link) needs a documented contract and a 10-line example server.

Acceptance: `MAIL_POLL` rate below 10% over 20 accounts with mail.tm alone;
quoted-printable fixture test passes.

### R6. Config surface
Move `MailPollSec`, `MailTimeoutSec`, `MaxRetries`, `MinDelayMs`, `MaxDelayMs`,
`Concurrency` limits and `ProfileFallback` into `api.json` (or a new
`settings.json`) with defaults; expose them in the dashboard. Today they are
compile-time defaults except through the UI.

Acceptance: changing the timeout does not require a rebuild; invalid values
fall back to defaults and are logged.

### R7. Resumable queue
Add `queue.json` with one record per account: `{id, email, inbox provider,
step, magic_link, created_at, status}`. Persist after every step. On start,
resume unfinished records before creating new inboxes.

Acceptance: kill the process mid-run, restart → it continues the same inbox
(no duplicate mail request) and never loses a verified account.

### R8. Proxy health
- Pre-flight check each proxy (`GET https://api.ipify.org` or the site itself)
  and score latency.
- Keep a per-proxy failure counter; rotate on Cloudflare/403/429 and **retry the
  same account** with the next proxy (today the proxy is fixed per index).
- Detect "banned" pages and stop using that proxy for the session.

Acceptance: a run with one dead proxy in a two-proxy pool still completes.

### R9. Verify the profile after creation
After `createProfileAPI` returns, fetch the public profile (username URL or an
API lookup) and assert username, bio fragment, avatar URL and country are
present. If not, retry once via browser (R4) and record `profile_verified`.

Acceptance: a 200 response with a silently ignored payload no longer counts as
`profile_completed`.

---

## 4. P2 — operations, security, scale

### R10. Turnstile / captcha
`captcha.go` still returns "not yet wired". Choose one:
- implement 2captcha Turnstile (`in.php` → poll `res.php`) and feed
  `cf-turnstile-response` into the sign-in request; or
- make the fallback a **visible** browser and document that a human solves it.

Do not add silent solving without the operator's consent (see §7).

### R11. Dashboard
- Replace the 2 s poll with Wails events (`runtime.EventsEmit` on each log line,
  `GetLogsSince(offset)` as fallback) — smoother and no duplicate lines.
- Account table: username, email, country, code, verified, profile-complete,
  buttons for copy cookies / open profile / retry / delete.
- Error-code chips from `stats.by_code`, not only success/failed.

### R12. Security at rest
`accounts.json` contains session cookies. Encrypt it with AES-GCM using a
machine-bound key (DPAPI on Windows) or keep it in the OS keyring; keep the
`.gitignore` rule. Add a "reveal cookie" confirmation in the UI.

### R13. Tests / CI
- Windows GitHub Actions job: `go vet`, `go test ./... -race`, `npm test`,
  `npm run verify`, `wails build` (or `go build ./...`).
- Extract an inbox seam (`Register.inboxFactory`) and add an integration test of
  `RegisterOne` against the fake NextAuth server + a fake inbox + fake upload/
  profile endpoints (no network).
- Fuzz `Redact`, `ExtractMagicLink`, `extractNextURL`, `validateMagicLink`.
- Add a golden test that `crushfling.js`/`index.html` ids used by tests are all
  present (extend `tests/crushfling-ui.test.cjs`).

### R14. Packaging
Single Windows EXE (`go build -ldflags "-s -w"`), an installer, a scheduled-task
script, and a `--headless --count N` mode that logs to a rotating file. Document
where the app data folder is on Windows.

### R15. Language / docs
Bengali labels for dashboard fields, tooltips that explain `error_code`s, and a
one-page "what to do when it fails" card in `crushfling/README.md`.

---

## 5. Known limitations / bugs found during this debug pass

| # | Limitation | Impact | Fix |
|---|------------|--------|-----|
| 1 | `CaptchaSolver` is a stub (`TwoCaptchaSolver` returns "not wired") | Turnstile-protected IPs cannot proceed via API | R10 |
| 2 | `email.DebugLog` is a package-global hook | two concurrent Register instances share one log sink | pass a per-provider logger (R5/R7) |
| 3 | Profile API failure leaves the uploaded avatar orphaned | storage garbage, picture already moved to `used/` | delete uploaded asset or retry in browser (R4/R9) |
| 4 | `StopBulk` cancels the context, but an in-flight request can last up to the 30 s client timeout | stop feels slow | lower per-request timeout; check ctx between hops |
| 5 | Proxy is chosen per account index, not per attempt | one bad proxy burns one account | R8 |
| 6 | No username-availability check | profile creation can fail on a taken name | retry with a new username on 409 (R9) |
| 7 | Trace/dump bodies capped (32 KiB) and binary uploads skipped | large HTML error pages are cut | raise the cap while calibrating |
| 8 | Dashboard uses a relative default `data/crushfling` | resolves against the process working dir, not the EXE | resolve against the Wails data dir in Go (R14) |
| 9 | `accounts.csv` is only written by the CLI | dashboard users have no spreadsheet export | export both from the service |
| 10 | Error text is English only | owner reads Bengali | R15 |

---

## 6. Definition of done for "next task"

1. R1–R3 complete on Windows, with the verification table updated from
   "inferred" to "verified" where evidence exists.
2. A single-account run saved in `accounts.json` with `verified=true` and
   `profile_completed=true`, and its trace retained (redacted).
3. `docs/CRUSHFLING_AUTO_REGISTER.md` §9 updated with the real numbers and
   links to the trace/dump used as evidence.
4. No new runtime data committed (`data/`, `debug/`, `logs/` stay ignored).

---

## 7. Guardrails / do-not-do

- Do not run high concurrency or many accounts before R2/R3 produce a green
  single account.
- Do not bypass Cloudflare in a way that violates the site's terms; prefer a
  real browser and your own IP/proxy.
- Do not add silent captcha solving without an explicit opt-in flag and a
  documented consent note.
- Do not commit or paste cookies, session tokens, `accounts.json` or raw
  `data/` archives into chat or git.
- Do not keep retrying guessed endpoints in a loop: calibrate `api.json` once
  and fail fast with `MAGIC_REQUEST`.
