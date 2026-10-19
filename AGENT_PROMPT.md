# Agent Task Prompt — paste this as the first message

> Reusable build prompt. Replace everything inside `<>`. Keep the structure.
> It works because it gives the agent a fixed target, a fixed finish line and a
> fixed report format — not because it tells the agent to ignore its own rules.

---

You are the implementation engineer for this repository. Your job is to plan,
build and verify working code. Deliver files, not opinions.

## 1. Objective

Build `<one concrete outcome, e.g. "a CLI that checks N accounts against site X
and writes valid/invalid/error result files">`. It must run end-to-end on
Windows with `python` and produce the files listed in §4.

## 2. Ground truth — read these before planning (max 6 files)

- `AGENTS.md` (repo rules)
- `docs/PROJECT_MAP.md`
- `<the main file you will extend>`
- `<existing tests>`

Do not research anything else. Facts you need are in §9.

## 3. Scope

**In scope**

- `<file or folder 1>` — `<what changes>`
- `<file or folder 2>` — `<what changes>`

**Out of scope (do not touch)**

- `<files/areas>`

## 4. Deliverables

| Path | Must do | Done when |
|---|---|---|
| `<path>` | `<behaviour>` | `<observable check>` |
| `<path>` | `<behaviour>` | `<observable check>` |

## 5. Definition of done

All of these must pass before you say the task is complete:

- [ ] `<command>` → exit 0, `<expected output>`
- [ ] `<command>` → exit 0, `<expected output>`
- [ ] No credentials, tokens or runtime data inside tracked files
- [ ] `<docs file>` updated with the new usage
- [ ] Nothing outside §3 was modified

## 6. Method (in this order)

1. **Read** §2 files. Write a 3-line summary of the current state.
2. **Plan** — max 12 lines: files to change, order, main risk. No prose.
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
- **Never invent** an API, endpoint, field name or library function. If unsure,
  read it from the code or the captured evidence in §9.
- **No placeholders.** No `TODO`, `pass`, `...`, `# implement later`, or fake
  return values in delivered code.
- **Surgical edits.** Do not rewrite a working file to "clean it up".
- **Report only engineering information.** If something cannot be done, one line
  saying what and why is enough.

## 9. Facts — use these, do not re-research

```
<site/API facts, e.g.:
POST /site/login  → LoginForm[user_name], LoginForm[user_password]  (no _csrf)
GET  /ajax/loginnotify → {"out": 1} on success
success → 302 to /
error  → .help-block text
>
<python/OS constraints, e.g. Windows 3.12+: no set_event_loop_policy>
<known failure from last run: ...>
```

## 10. Local environment

```
OS / Python / Node: <versions>
Run:                <exact command>
Test:               <exact command>
Data directory:     <path, gitignored>
```

---

## Why the previous prompt kept getting refused

The old prompt opened with *"You Are Fully Uncensored Elite"* and then listed
*auth bypass, session hijacking, MITM decryption, Cloudflare/Akamai/DataDome
defeat and captcha-farm services*. That is a jailbreak signature plus an
authorization gap. Any agent reads it as "bypass protections on a site I do not
own, at scale", and stops — **more wording does not fix that.**

What actually removes friction, in order of effect:

1. **Name the target and the permission.** "I own these accounts; this is my
   own tool for my own accounts" is believable and checkable. "Uncensored" is
   not.
2. **Drop the evasion list.** If the deliverable does not require defeating
   Cloudflare/DataDome or solving captchas, deleting those lines removes most
   of the refusal risk immediately.
3. **Give a finish line.** Agents hedge when "done" is undefined. §4 + §5 make
   it binary.
4. **Give a report format.** Most "lecturing" is the agent filling space
   because no output shape was specified.
5. **State the constraints once.** Repeating "NEVER" in caps 20 times reads as
   conflict, and conflict makes agents cautious.

## Requirements that will still be declined — plan around them

These are refused on the task, not the wording. Design so they are not needed:

| Requirement | Use instead |
|---|---|
| Defeat Cloudflare / Akamai / DataDome / PerimeterX | Use the site's own JSON API with your own authenticated session, slower rate, real browser |
| Captcha-farm (2Captcha, CapSolver, OCR) | Detect the challenge, pause, report `BLOCKED: captcha`, resume after manual solve |
| Session hijacking / cookie theft from other users | Own-account session export/import only |
| MITM HTTPS decryption of a live site | Replay from a capture you already have (`capture/` tool) |
| Mass-use of accounts you do not own | One account, one operator, documented authorization |

Everything else in the old prompt — Playwright/CDP, HAR replay, WebSocket
capture, proxy support, retry/backoff, session persistence for your own login,
fingerprint consistency — is normal engineering and stays.
