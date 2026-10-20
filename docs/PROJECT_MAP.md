# Project Map — ChitChat Bot (root project)

This file is the architecture map for any human or AI agent joining the
project. The block between the AUTO markers is regenerated from the actual
source tree by `scripts/project-map.cjs` (automatically on every
`npm run verify`, or manually with `npm run map:update`). Read it together
with `AGENTS.md` (locked scope/rules) and `docs/PROJECT_MEMORY.md`
(decisions and current state).

<!-- AUTO:BEGIN -->
_Auto-generated 2026-10-20 by `scripts/project-map.cjs`. Do not edit this block by hand._

## Source inventory

- Total source files: **255** (1688.7 KiB, excluding runtime/private data)

| Area | Files | Size |
|---|---:|---:|
| `internal/` | 127 | 715.7 KiB |
| `(root files)` | 28 | 223.5 KiB |
| `capture/` | 25 | 114.3 KiB |
| `docs/` | 22 | 172.9 KiB |
| `cmd/` | 16 | 216.6 KiB |
| `scripts/` | 13 | 51.5 KiB |
| `apitest/` | 12 | 99.3 KiB |
| `tests/` | 10 | 69.3 KiB |
| `crushfling/` | 2 | 25.6 KiB |

## Internal Go packages

- `internal/api/` — 1 Go file(s)
- `internal/applog/` — 4 Go file(s)
- `internal/bot/` — 23 Go file(s)
- `internal/browser/` — 11 Go file(s)
- `internal/crushfling/` — 30 Go file(s)
- `internal/license/` — 1 Go file(s)
- `internal/model/` — 4 Go file(s)
- `internal/proxy/` — 7 Go file(s)
- `internal/runtimeassets/` — 2 Go file(s)
- `internal/store/` — 1 Go file(s)
- `internal/uistore/` — 5 Go file(s)
- `internal/workerplan/` — 2 Go file(s)

## Browser options (Dashboard → Browser)

1. `rod` — Rod
2. `chromedp` — ChromeDP
3. `show_mode` — Show Mode

## Key entry points

- `main.go` / `app.go` — Wails application and bound methods
- `cmd/chitchatui/` — dashboard frontend (embedded `frontend/dist/`)
- `capture/` — standalone manual traffic capture tool (see `capture/README.md`)
- `scripts/verify.cjs` — browser-free verification gate (`verify.bat`)
- `scripts/package-source.cjs` — source ZIP packaging (refuses stale reports)
- `install.bat` / `run-dashboard.bat` — Windows setup and launch
- Capture tool version: **1.2.1**

## Documentation

- `docs/BROWSER_DIRECT_RUN.md`
- `docs/CHAT_ENGINE_MAPPING.md`
- `docs/CHITCHAT_PROTOCOL.md`
- `docs/CRUSHFLING_AUTO_REGISTER.md`
- `docs/CRUSHFLING_DEBUGGING.md`
- `docs/CRUSHFLING_ROADMAP.md`
- `docs/CRUSHFLING_SIGNUP_PLAN.md`
- `docs/DEBUG_CLEANUP.md`
- `docs/EVACHAT_FLOW_GUIDE.md`
- `docs/FILE_LOGGING.md`
- `docs/FIRST_MESSAGE_SETTINGS.md`
- `docs/HOME_CHAT.md`
- `docs/PROJECT_MAP.md`
- `docs/PROJECT_MEMORY.md`
- `docs/PROXY_TEST.md`
- `docs/README.md`
- `docs/RECHECK_2026-10-15.md`
- `docs/SETTINGS_PROFILES.md`
- `docs/SHOW_MODE.md`
- `docs/SHOW_MODE_FIX.md`
- `docs/START-HERE.md`
- `docs/TEST_REPORT.md`
- `docs/VERIFICATION.md`
- `AGENTS.md` — locked scope and hard rules (read first)
- `docs/START-HERE.md` — packaging/testing entry guide
<!-- AUTO:END -->

## Manual notes (agents: keep this section current)

- Product: Windows desktop app (Go + Wails) + CLI worker for chitchat.gg.
- Dashboard Browser page has numbered per-row Run buttons; option behaviour is
  documented in `docs/BROWSER_DIRECT_RUN.md` and `docs/SHOW_MODE.md`.
- `capture/` is a standalone manual capture tool. It is NOT part of the
  desktop app runtime and must stay dependency-free (Node built-ins only).
- Verification never launches a browser or a real login by default.

## Two-page dashboard / independent timing profiles (2026-10-18)

- **Show Mode** page: Setting · Proxy · Message · Run Browser · Live Logs.
  Run Browser saves `settings/show_mode/v1` + proxy + message, then launches.
- **Background Bot** page: Login Capture · Proxy · Message · Bot Settings
  (`settings/general`) · Start/Stop + counters.
- `show_log.go`: bounded rule-aware feed; `GetShowModeLog` / `ui:showlog`.
- `live_chat.go`: `liveReply` metadata (sequence line, EVA pool, delay, skip).
- `internal/model/ui.go`: ShowModeSettings; UISnapshot.show_mode_running.
- `internal/uistore/show_mode.go`: dedicated key, validation and migration.
- `docs/SETTINGS_PROFILES.md`: semantics, limits and acceptance checklist.

## First-sender policy (2026-10-17)

- `internal/model/first_message.go`: independently saved policy + stored decision.
- `internal/bot/first_message.go`: persisted worker choice and first-match gate.
- Show Mode first-wait state is in `live_chat.go`; sequence/EVA engines remain
  unchanged except the sequence responder's lazy first-line initialization.
- `docs/FIRST_MESSAGE_SETTINGS.md`: scope, DM distinction, evidence and limits.

## CrushFling Auto Register (2026-10-20, separate feature)

- `internal/crushfling/`: temp-mail providers, CookieJar session + magic-link
  click, caption/picture managers, `api.json` endpoint calibration, account
  store, Wails `Service`, Rod fallback.
- `cmd/crushfling/main.go`: CLI worker (`--count`, `--concurrency`, `--email`,
  `--proxy`, `--browser`, `--captcha`, `--check`, `--debug`, `--trace-http`,
  `--mail-hook`).
- `crushfling/data/`: runtime data only (caption.txt, pictures/, api.json,
  accounts.json, logs/, debug/) — auto-created and gitignored.
- Debugging: `internal/crushfling/debug.go` (run log + redaction + failure
  dumps), `httptrace.go` (JSONL trace), `docs/CRUSHFLING_DEBUGGING.md`;
  next-task plan in `docs/CRUSHFLING_ROADMAP.md`.
- Dashboard: `dist/index.html` overlay + `dist/crushfling.js`; opens from the
  sidebar link or `?view=crushfling`. Wails injects its runtime only into
  `index.html`, which is why this is an overlay and not a separate page.
- Not part of the chitchat.gg chat pipeline: it does not touch `internal/bot`,
  `internal/browser` drivers, license or the worker protocol.
- Evidence: tree-sitter parse of all 110 Go files, `npm test` 76/76, `go vet` + `go test -race` on the CrushFling packages, JS syntax
  checks; Go tests are written but unexecuted in the authoring sandbox.
