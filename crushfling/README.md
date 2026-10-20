# CrushFling Auto Register — quick guide

**কী করে:** temp-mail API → crushfling.com magic sign-in link → **একই HTTP
session-এ** link click → avatar upload → profile create (female, looking for
male, বয়স ১৯–২৪, random নাম, USA + সব ইউরোপীয় দেশ) → `accounts.json` এ save।

Site-টা Laravel — NextAuth নয়। তাই endpoint অনুমান করা হয় না: worker প্রথমে
`/account` পেজ পড়ে নিজেই বুঝে নেয় email form-টা plain form, Livewire নাকি
JSON — এবং সেই অনুযায়ী CSRF সহ request পাঠায়।

বিস্তারিত: [`docs/CRUSHFLING_AUTO_REGISTER.md`](../docs/CRUSHFLING_AUTO_REGISTER.md)
পরিকল্পনা ও ধাপে ধাপে যাচাই: [`docs/CRUSHFLING_SIGNUP_PLAN.md`](../docs/CRUSHFLING_SIGNUP_PLAN.md)

---

## প্রথমবার

```bat
:: 0) site আজ কী চায় দেখুন (কোনো account তৈরি হয় না)
go run ./cmd/crushfling --probe

:: 1) এক account দিয়ে test
go run ./cmd/crushfling --count 1 --email mailtm -v

:: 2) ছবি + bio দিয়ে bulk
::    ছবি রাখুন: crushfling\data\pictures\  (jpg/png/webp)
::    bio লিখুন: crushfling\data\bio.txt (প্রতি লাইনে একটি)
::    (অথবা dashboard → Bio list → Load bio.txt…)
go run ./cmd/crushfling --count 10 --concurrency 2 --proxy proxies.txt
```

প্রথম run-এ `crushfling\data\` নিজে থেকেই তৈরি হবে
(dashboard-এ চালালে app-এর data folder-এর ভিতরে `crushfling\`):
Data folder ঘরটি খালি রাখলে এই default-ই ব্যবহার হয়।

```
crushfling/data/
  bio.txt            ৩১টি ready bio line (ব্যবহৃত লাইন auto delete হয়)
  caption.txt        পুরোনো নাম — bio.txt না থাকলে এটাই পড়া হয়
  pictures/          এখানে ছবি রাখুন (dashboard → Pictures → Browse…)
  pictures/used/     ব্যবহৃত ছবি এখানে চলে যায়
  api.json           endpoint calibration (site বদলালে এখানে ঠিক করুন)
  accounts.json      verified account + cookies
  accounts.csv       spreadsheet export
  logs/              প্রতি run-এর লগ (run-*.log)
  debug/             fail-*.json, trace-*.jsonl, report-*.txt
```

> `crushfling/data/` gitignored — cookie/session কখনো commit হবে না।

## Dashboard (Wails)

`install.bat` → `run-dashboard.bat` (নতুন checkout-এ আগে `npm run overlay:sync`)
→ sidebar-এ **💘 CrushFling Auto Register** (অথবা `?view=crushfling`)।

উপরে সবসময় **▶ Start** আর **■ Stop**। ছয়টি ট্যাব:

| ট্যাব | কী আছে |
|---|---|
| 1 · Pictures | ছবির ফোল্ডার Browse/Open, কত ছবি বাকি |
| 2 · Proxy | proxy ফাইল Browse, লাইন লিখুন, **Validate** (offline format check) |
| 3 · Bio list | `bio.txt` লোড, কত লাইন বাকি, পরের লাইনগুলোর প্রিভিউ |
| 4 · Browser | headless / no-headless, browser fallback, browser verify, concurrency, **Close all browsers** |
| 5 · Settings | account সংখ্যা, data folder, mail provider/hook, poll/timeout, retries, delay, captcha, debug/trace, preview, export |
| 6 · Logs | live log + level filter, log folder, debug report, clear |

**Stop** চাপলে রান বাতিল হয় **এবং খোলা সব Chromium উইন্ডো সাথে সাথে বন্ধ হয়**
(`CloseAllBrowsers`) — counter ০-তে নেমে আসে।

Settings-এর **retries** এখন সত্যিই কাজ করে (default 3, সর্বোচ্চ 5; back-off
2/4/6 সেকেন্ড)। শুধু session প্রমাণ হওয়ার **আগে**র ব্যর্থতা retry হয় (inbox,
session start, magic-link request, inbox poll, magic-link click); `verify`,
upload বা profile ব্যর্থতা retry হয় না — অ্যাকাউন্ট ইতিমধ্যে তৈরি হয়ে থাকতে
পারে, দ্বিতীয় চেষ্টায় ডুপ্লিকেট হবে।

## CLI flags

| Flag | অর্থ |
|------|------|
| `--count` | কত account |
| `--concurrency` | একসাথে কত |
| `--email` | `mailtm` \| `1secmail` \| `guerrilla` \| `custom` |
| `--custom-email` | catch-all address (`@` সহ) |
| `--mail-hook` | custom provider-এর জন্য mail API URL (`?email=` যোগ হয়) |
| `--proxy` | proxy ফাইল path |
| `--proxy-list` | proxy লাইন সরাসরি |
| `--avatars` | ছবির ফোল্ডার |
| `--data` | data ফোল্ডার |
| `--browser` | API block হলে Rod browser fallback |
| `--headless` | fallback browser headless |
| `--manual-link` | request করার পর terminal-এ link paste করবেন (same session) |
| `--link` | আগেই পাওয়া link দিয়ে click (count=1) |
| `--browser-verify=false` | link click-এর পর cookie না এলে browser-এ "Sign In" click বন্ধ করে |
| `--check` | network ছাড়া data folder যাচাই (api.json, caption, pictures, logs) |
| `--probe` | live `/account` পেজ পড়ে দেখায় sign-up আসলে কীভাবে হয় (কিছু register করে না) |
| `--base-url` | অন্য origin-এ চালান (mock/mirror); default `https://crushfling.com` |
| `--bio` | একটি টেক্সট ফাইল `<data>/bio.txt` হিসেবে লোড করে (প্রতি লাইনে একটি bio) |
| `--preview` | পরের account-টা কেমন হবে (নাম, বয়স, দেশ, bio) দেখিয়ে বন্ধ হয় |
| `--captcha` | 2captcha key (Turnstile) |
| `-v` | বিস্তারিত log (`--debug`-এর সমান) |
| `--debug` | debug log + প্রতি failed account-এ `debug/fail-*.json` dump |
| `--trace-http` | request/response body সহ `debug/trace-*.jsonl` (token redacted) |

## Flow (সংক্ষেপে)

1. inbox তৈরি (mail.tm default)
2. একটাই `http.Client` + `CookieJar` — পুরো flow জুড়ে
3. `GET /account` → discovery: CSRF meta token, form action / Livewire method,
   upload/profile path (কোনো path অনুমান করা হয় না)
4. পেজ যেভাবে পাঠায় সেভাবেই email submit — form POST (`_token` + hidden
   fields) / `POST /livewire/update` (snapshot + method) / JSON fallback
5. inbox poll (3s, max 120s) → link regex
6. **same session**-এ link click → cookie; cookie না এলে `/account` আবার পড়ে
   প্রমাণ (email form নেই = signed in); তাও না হলে Rod browser-এ "Sign In"
7. avatar upload → profile create → `/p/<username>/` দেখে public verify
8. verified + profile complete হলে caption line delete + ছবি `used/`-এ move +
   account save (profile API fail করলে account save হয় কিন্তু caption ফেরত আসে)
9. সব account fail করলে CLI exit code `1`; অন্তত একটা সফল হলে `0`

## Debug (নতুন)

প্রতিটি run নিজে থেকেই `crushfling/data/logs/run-*.log` লেখে। প্রথমবার
live চালানোর সময়:

```bat
:: প্রথমে offline যাচাই — network লাগবে না
go run ./cmd/crushfling --check

:: তারপর এক account, full trace
go run ./cmd/crushfling --count 1 --email mailtm --debug --trace-http
```

- শুরুতে `Run log:` path প্রিন্ট হয়।
- fail করলে `crushfling/data/debug/fail-*.json` খুলুন — step, error code,
  timings, HTTP status, log tail সব এক জায়গায়।
- `debug/trace-*.jsonl`-এ প্রতিটি request: status, duration, কোন cookie set
  হলো, আর (enable করলে) body। Token/cookie/email সব redacted।
- Dashboard-এ **Open log folder** + **Debug report** বোতাম, আর log level
  filter আছে।

Error code: `INBOX`, `CSRF`, `MAGIC_REQUEST`, `MAIL_POLL`, `BAD_LINK`,
`MAGIC_CLICK`, `VERIFY`, `UPLOAD`, `PROFILE`, `CANCELED`, `PANIC`।
বিস্তারিত: [`docs/CRUSHFLING_DEBUGGING.md`](../docs/CRUSHFLING_DEBUGGING.md)

## Site বদলে গেলে (২ মিনিট)

`capture\run.bat --url https://crushfling.com/account` → নিজে একবার login করুন
→ `api-catalog.json` দেখে `crushfling\data\api.json`-এ আসল path বসান।
Rebuild লাগবে না। বিস্তারিত: docs section 4।

## Node diagnostic (Go ছাড়া)

```bat
node crushfling/run.js --count 1 --email mailtm --verbose
```

শুধু mail + same-session magic-link যাচাই করে; avatar/profile Go worker করে।
`--verbose` দিলে প্রতিটি request/status দেখায় (token masked), আর শুধু
**verified** account-ই `accounts.json`-এ save হয় — caption verified হলে তবেই
delete হয়।

## সতর্কতা

নিজের অনুমোদিত account-এর জন্য ব্যবহার করুন। Site-এর ToS/আইন মানা আপনার
দায়িত্ব। `--concurrency` ৩-এর বেশি করবেন না, নিজের proxy ব্যবহার করুন।
