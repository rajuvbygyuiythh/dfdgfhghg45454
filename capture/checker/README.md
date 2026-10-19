# Dizkover Checker 2.1

`capture/checker/` হলো একটা আলাদা Python tool — Dizkover login valid/invalid
check করার জন্য। এটা desktop app-এর runtime-এর অংশ না, আর `capture/`-এর Node
capture tool-এর সাথেও mixed না। Playwright দিয়ে আসল login form submit করে,
তারপর site-এর নিজের `/ajax/loginnotify` API দিয়ে result confirm করে।

## আগের version কেন প্রতিটা account-এ আটকে যাচ্ছিল

আপলোড করা `logs.txt`-এ দেখা গেছে ৪৪টা account-ই একই জায়গায় fail করছে:

```text
[ERROR] alice | CSRF not found | Step: csrf | Direct
...
Total: 44 | Valid: 0 | Invalid: 0 | Error: 44
```

পুরনো `main.py` login page-এ `input[name="_csrf"]` খুঁজে না পেলে সাথে সাথে
`ERROR` return করত। কিন্তু captured real traffic-এ দেখা যায় login POST-এ
শুধু এই দুটো field যায়:

```text
POST https://www.dizkover.com/site/login
Content-Type: application/x-www-form-urlencoded

LoginForm[user_name]=...
LoginForm[user_password]=...
```

অর্থাৎ এই login form-এ আলাদা `_csrf` hidden input নেই / লাগে না। তাছাড়া
Playwright তো আসল form-টাই submit করে — page যদি কোনো hidden token render
করে, browser নিজেই সেটা পাঠায়। তাই token হাতে পড়া বা gate বানানো দুটোই ভুল
ছিল।

**Fix:** CSRF hard-check সরানো হয়েছে। এখন token থাকলে শুধু debug log-এ লেখে,
আর form submit browser-এর হাতে। এতে ৪৪/৪৪ account আর `csrf` step-এ আটকাবে না।

## আরও যা fix হয়েছে

| সমস্যা | আগে | এখন |
|---|---|---|
| CSRF false gate | `_csrf` না থাকলে সব ERROR | token optional, form natural submit |
| False CAPTCHA | পুরো HTML-এ `"captcha"` শব্দ থাকলেই ERROR (login page-এ reCAPTCHA script সবসময় থাকে) | শুধু **visible challenge** দেখলে ERROR |
| `networkidle` timeout | analytics/reCAPTCHA-র কারণে ৩০s hang | `domcontentloaded` + result polling |
| Python 3.14 warning | deprecated `set_event_loop_policy` | Windows-এ `loop_factory=ProactorEventLoop` |
| Proxy visibility | শুধু `Proxies: Disabled` | প্রতিটা proxy requests + real Chromium দিয়ে test, IP + speed log |
| Stuck point | শুধু "CSRF not found" | প্রতি step (`page_load`, `find_form`, `submit`, `wait_result`, `api_verify`, `proxy_check`) log |
| Unconfirmed result | ভুল করে `INVALID` | `ERROR` + screenshot/HTML debug artifact |
| Password leak | console/log-এ `user:pass` | console/log-এ শুধু username; password শুধু `data/valid.txt`/`invalid.txt`-এ |

`INVALID` এখন শুধু তখনই হয় যখন site নিজে error message দেখায় (যেমন
`.help-block`) অথবা `/ajax/loginnotify` স্পষ্টভাবে `out != 1` বলে। কোনো
signal ছাড়া login page-এ আটকে থাকলে সেটা `ERROR` — কারণ তখন valid/invalid
guess করা ঠিক না।

## Install (একবার)

`install.bat` double-click করুন। এটা:

1. Python 3.9+ খুঁজে বের করে (`python`, না পেলে `py -3`)
2. `requirements.txt` থেকে `playwright`, `colorama`, `requests[socks]` install করে
3. Playwright Chromium download করে
4. `data\` folder আর `data\ac.txt`, `data\proxy.txt` তৈরি করে

## ব্যবহার

1. **`data\ac.txt`** — প্রতি লাইনে `username:password`
   ```
   alice:my-password
   bob:another-password
   ```
2. **`data\proxy.txt`** (ঐচ্ছিক) — প্রতি লাইনে একটা proxy
   ```
   1.2.3.4:8080
   user:pass@1.2.3.4:8080
   socks5://user:pass@1.2.3.4:1080
   ```
   `config.py`-তে `USE_PROXY = True` (default)। proxy না থাকলে Direct চলবে।
   সব proxy dead হলে `REQUIRE_PROXY = False` থাকলে Direct-এ fallback করবে।
3. **`run.bat`** — menu থেকে:
   - `1` headless check
   - `2` visible browser (কোনো challenge এলে দেখার জন্য)
   - `3` শুধু proxy test (requests + Chromium)
   - `4` browser-free self-test
   - `5-7` logs / valid / invalid দেখা
   - `8-9` account/proxy edit
   - `10` installer
   - `11` **real-browser self-test** — local replica site-এ আসল Chromium দিয়ে
     পুরো flow চালায় (ইন্টারনেট বা আসল account লাগে না)

Command line সরাসরি:

```bat
py -3 main.py --headless
py -3 main.py --visible --tabs 3
py -3 main.py --test-proxies
py -3 main.py --account alice
```

## Files (সব runtime data `data\`-এ)

| File | কী থাকে |
|---|---|
| `data\ac.txt` | আপনার input accounts (`user:pass`) |
| `data\proxy.txt` | আপনার input proxies |
| `data\valid.txt` | valid accounts + proxy + via + time (প্রথম `user:pass` অংশটুকু copy করলে আবার `ac.txt`-এ ব্যবহার করা যায়) |
| `data\invalid.txt` | invalid accounts + site-এর error message |
| `data\banned.txt` | banned ধরা পড়লে |
| `data\errors.txt` | network/CAPTCHA/unconfirmed error listings |
| `data\logs.txt` | পুরো step-by-step run log |
| `data\debug\` | ERROR/BANNED-এর screenshot + HTML |

`data/` Git-ignored এবং source package থেকেও বাদ (project inventory `data`
folder বাদ দেয়)। তাই credential বা session কখনো commit/package হবে না।

## `data\logs.txt` কীভাবে পড়বেন

প্রতিটা account-এর জন্য:

```text
[2026-10-20 03:16:52] [START] [alice] attempt=1/2 proxy=Direct
[2026-10-20 03:16:52] [DEBUG] [alice] GET /site/login -> 200 | title='Dizkover - Login'
[2026-10-20 03:16:52] [DEBUG] [alice] form user=... pass=... csrf_hidden=absent (expected; not required)
[2026-10-20 03:16:55] [SUCCESS] [alice] VALID | redirected to https://www.dizkover.com/ | cookies=...
[2026-10-20 03:16:55] [VALID]   alice | redirected to ... | proxy=Direct
```

আটকে গেলে `step=` দেখুন:

- `proxy_check` → proxy browser-এ চলছে না (একই proxy option `3` দিয়ে test করুন)
- `page_load` → site/page open হচ্ছে না, timeout বা HTTP error
- `find_form` → login form পাওয়া যায়নি (site layout বদলেছে — debug HTML দেখুন)
- `submit` → button click-এ exception
- `wait_result` → result-এর অপেক্ষায় timeout
- `api_verify` → `/ajax/loginnotify` call fail
- `confirm` → কোনো redirect/error/API signal নেই; screenshot দেখে সিদ্ধান্ত নিন
- `captcha` → visible reCAPTCHA challenge; এটা **bypass করা হয় না**

## Captured login API (আপনার request অনুযায়ী)

`03:16`-এর run-এর পরে যে capture/analysis হয়েছিল, তার সারমর্ম:

```text
1. GET  https://www.dizkover.com/site/login
         200 text/html (Yii2-style form)

2. POST https://www.dizkover.com/site/login
         Content-Type: application/x-www-form-urlencoded
         Origin: https://www.dizkover.com

         LoginForm[user_name]=<user>
         LoginForm[user_password]=<pass>

         (*) no _csrf field in the real POST

3. success -> 302/redirect -> https://www.dizkover.com/

4. GET  https://www.dizkover.com/ajax/loginnotify
         X-Requested-With: XMLHttpRequest
         Accept: application/json
         -> JSON {"out": 1}

5. GET  https://www.dizkover.com/?ajax=1&page=2
   GET  https://www.dizkover.com/u/<username>?page=2
   POST https://www.dizkover.com/ajax/agreetoterms
```

reCAPTCHA Enterprise-এর request (`/recaptcha/enterprise/reload`,
`replaceimage`, `userverify`) page-এ load হয়, কিন্তু captured login POST-এ
কোনো captcha token ছিল না। তাই script token bypass করার চেষ্টা করে না; কোনো
visible challenge এলে সেটা `ERROR: CAPTCHA challenge shown` হিসেবে report করে।

## Self-test levels

| Test | কী প্রমাণ করে | Browser | Internet |
|---|---|---|---|
| `test_core.py` + `test_flow.py` | parsing + classification + পুরো `check_account` flow (fake page) | ❌ | ❌ |
| `selftest_live.py` | আসল Chromium-এ form fill → POST → redirect → loginnotify → result files; VALID/INVALID/BANNED/CAPTCHA চারটাই | ✅ | ❌ (local replica) |
| `main.py` (real run) | আসল site-এ আসল account | ✅ | ✅ |

```bat
py -3 -m unittest -v test_core.py test_flow.py
py -3 selftest_live.py
py -3 selftest_live.py --visible
```

`selftest_live.py` একটা temporary data folder ব্যবহার করে, তাই আপনার আসল
`data\ac.txt` / `valid.txt` ছোঁয় না।

## Environment override (এক run-এর জন্য)

`config.py` না বদলে env var দিয়ে বদলানো যায়:

| Variable | অর্থ |
|---|---|
| `DIZKOVER_HEADLESS` | `0` = visible |
| `DIZKOVER_CHROMIUM` | `C:\Program Files\Google\Chrome\Application\chrome.exe` |
| `DIZKOVER_TABS` | একসাথে কত account |
| `DIZKOVER_USE_PROXY` | `0` = Direct |
| `DIZKOVER_BASE_URL` | অন্য mirror/host |
| `DIZKOVER_DATA_DIR` | data folder বদলানো (self-test এটাই ব্যবহার করে) |

```bat
set DIZKOVER_CHROMIUM=C:\Program Files\Google\Chrome\Application\chrome.exe
py -3 main.py --visible
```

## Limitations / নিরাপত্তা

- **reCAPTCHA solve করা হয় না।** Challenge এলে visible mode-এ চালিয়ে দেখুন,
  অথবা ওই account ম্যানুয়ালি login করুন। এটা কোনো captcha-solving tool না।
- `data\ac.txt`, `data\proxy.txt`, `data\valid.txt`, `data\logs.txt`,
  `data\debug\` — এগুলো secret। কাউকে share করবেন না। শুধু `logs.txt` share
  করলেও usernames দেখা যায়; password গুলো log-এ যায় না।
- এই tool শুধু নিজের account/authorized account-এর জন্য; site-এর terms
  মেনে চালানোর দায়িত্ব আপনার।
- Proxy credential `data\proxy.txt`-এ plain text-এ থাকে; ফাইলটা সুরক্ষিত
  জায়গায় রাখুন।
- Proxy health check `https://api.ipify.org` দিয়ে হয়। আপনার নেটওয়ার্কে এটা
  blocked হলে `config.py`-তে `PROXY_TEST_URL` বদলে অন্য একটা JSON IP echo
  endpoint দিন; নাহলে সব proxy ভুল করে DEAD দেখাতে পারে।

## Test

```bat
py -3 -m unittest -v test_core.py test_flow.py
```

মোট ৩২টা test:

- `test_core.py` (২৫) — account parsing, proxy parsing (auth/scheme/IP),
  login URL detection, VALID/INVALID/BANNED/ERROR classification।
- `test_flow.py` (৭) — fake Playwright page দিয়ে পুরো `check_account` flow:
  CSRF ছাড়া form ধরা → submit → redirect/loginnotify → result; CAPTCHA,
  wrong-password, anti-bot challenge আর unconfirmed case।

`run.bat` → option `4`-ও একই test চালায়। এগুলোতে কোনো network, browser বা
real credential লাগে না। একটা আসল Windows/Chromium run এখনও দরকার —
নিচের "Limitations" দেখুন।
