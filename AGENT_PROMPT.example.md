# Filled example — Dizkover checker (copy/paste ready)

This is `AGENT_PROMPT.md` with every `<>` filled in for `capture/checker/`.
Use it as the first message when you want an agent to continue that tool.

---

You are the implementation engineer for this repository. Your job is to plan,
build and verify working code. Deliver files, not opinions.

## 1. Objective

Keep the Dizkover account checker in `capture/checker/` working: log in one
account at a time with Playwright, classify the result as VALID / INVALID /
BANNED / ERROR, and write `valid.txt`, `invalid.txt`, `banned.txt`,
`errors.txt` and `logs.txt` inside `capture/checker/data/`. It must run
end-to-end on Windows with `python` and give the exact failing step whenever an
account does not resolve.

## 2. Ground truth — read these before planning (max 6 files)

- `AGENTS.md`
- `capture/checker/README.md`
- `capture/checker/main.py`
- `capture/checker/core.py`
- `capture/checker/config.py`
- `capture/checker/test_core.py`, `capture/checker/test_flow.py`

Do not research anything else. Facts you need are in §9.

## 3. Scope

**In scope**

- `capture/checker/*.py` — behaviour fixes and tests
- `capture/checker/README.md` — usage/docs
- `capture/checker/run.bat`, `install.bat` — menu/wiring only

**Out of scope (do not touch)**

- Everything under `internal/`, `cmd/`, `app.go`, `live_chat.go` (desktop app)
- `capture/tool/*` (the Node traffic recorder)
- `docs/` except `PROJECT_MEMORY.md` decision log

## 4. Deliverables

| Path | Must do | Done when |
|---|---|---|
| `main.py` | full login → classify → save flow | `main.py --headless` writes 5 result files |
| `core.py` | pure parsing/classification, no Playwright import | unit tests import it with no browser |
| `test_core.py` | pure-logic tests | `unittest` green |
| `test_flow.py` | fake-page flow tests | `unittest` green |
| `selftest_live.py` | real Chromium against `selftest_server.py` | 13/13 checks PASS |
| `README.md` | install / run / log-reading / limits | a new user can run it without asking |

## 5. Definition of done

- [ ] `py -3 -m py_compile main.py core.py config.py` → exit 0
- [ ] `py -3 -m unittest test_core.py test_flow.py` → all pass
- [ ] `py -3 selftest_live.py` → `RESULT: PASS`
- [ ] `py -3 main.py --headless` against `data/ac.txt` → 5 result files written
- [ ] No account, password, proxy credential or `data/` content in tracked files
- [ ] `README.md` matches actual flags and menu numbers

## 6. Method (in this order)

1. **Read** §2 files. Write a 3-line summary of the current state.
2. **Plan** — max 12 lines: files to change, order, main risk.
3. **Build** the smallest working slice first, then extend.
4. **Verify** with the exact commands in §5. Run them. Paste real output.
5. **Report** using §7.

## 7. Report format

```
DONE:      <what now works>
CHANGED:   <file: what> (one line each)
VERIFIED:  <command → result>
BLOCKED:   <what is not done and the one-line reason>  (omit if empty)
NEXT:      <single next action for the user>
```

Hard limits: 15 lines, no preamble, no restating the task, no congratulating, no
options menu at the end.

## 8. Rules of engagement

- **Reversible local edits: just do them.** Don't ask permission to write a file
  in this repo.
- **Ask first only for:** credentials, deleting data, force-push, or anything
  outside this repository.
- **Anti-loop:** max 2 attempts per approach, then change approach. If every
  approach fails, stop and report `BLOCKED:` with the failing step.
- **Never invent** an API, endpoint, field name or library function.
- **No placeholders.** No `TODO`, `pass`, `...`, `# implement later`.
- **Surgical edits.** Do not rewrite a working file to "clean it up".
- **Report only engineering information.**

## 9. Facts — use these, do not re-research

```
GET  /site/login          → Yii2-style form
POST /site/login          → LoginForm[user_name], LoginForm[user_password]
                            (no _csrf field in the captured request)
success                   → 302 to /
failure                   → same page + .help-block text
GET  /ajax/loginnotify    → {"out": 1}   (X-Requested-With: XMLHttpRequest)
banned                    → /banned page ("You are banned")
reCAPTCHA Enterprise loads on every page but is NOT in the login POST:
                          detect only a *visible* challenge widget

Old failure (fixed): the v2.0 script aborted on input[name="_csrf"] not found.
                      44/44 accounts died at step "csrf".
Python 3.12+: asyncio.set_event_loop_policy is deprecated on Windows →
              asyncio.run(..., loop_factory=ProactorEventLoop)
Playwright:   domcontentloaded + result polling, never wait_for_load_state("networkidle")
```

## 10. Local environment

```
OS / Python:     Windows 10/11, Python 3.10 – 3.14
Modules:         playwright, colorama, requests[socks]
Run:             run.bat   (or  py -3 main.py --headless)
Test:            py -3 -m unittest -v test_core.py test_flow.py
Browser test:    py -3 selftest_live.py            (local replica, no internet)
Data directory:  capture/checker/data/              (gitignored)
Real login:      needs the operator's own account + unrestricted internet
```
