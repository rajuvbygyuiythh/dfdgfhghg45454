# ChitChat Bot + CrushFling Auto Register

Windows (10/11 x64) Wails ডেস্কটপ অ্যাপ — ChitChat Bot-এর সাথে যোগ করা
**CrushFling bulk auto-register** ফিচার। পুরোনো bot, EVA chat, license ও proxy
কোড অপরিবর্তিত; নতুন সব কোড `internal/crushfling/`, `cmd/crushfling/` এবং
ড্যাশবোর্ডের `overlay/` ফোল্ডারে আলাদা রাখা।

## শুধু CrushFling (আলাদা ছোট অ্যাপ) — একটি zip

ChitChat Bot ছাড়া **শুধু CrushFling** চাইলে:

**https://github.com/rajuvbygyuiythh/dfdgfhghg45454/releases/tag/crushfling**

- একটাই ফাইল: `CrushFling-Only-Windows-x64.zip`
- ভিতরে: `CrushFling.exe` (একই ছয়-ট্যাব ড্যাশবোর্ড), `crushfling-cli.exe`,
  `run.bat`, `install.bat`, সম্পূর্ণ source ও SHA-256 manifest
- extract → `run.bat` ডাবল-ক্লিক
- সোর্স: `standalone/` (অটো-জেনারেটেড; `npm run standalone:sync`)

## Windows ডাউনলোড (সবচেয়ে সহজ)

সরাসরি ডাউনলোড — exe সহ রেডি zip (Windows-এ build করা):

**https://github.com/rajuvbygyuiythh/dfdgfhghg45454/releases/tag/windows-x64**

- ফাইল: `CrushFling-Bot-Windows-x64.zip` (~21 MB)
- zip পুরোটা extract → `run.bat` ডাবল-ক্লিক
- আলাদা করে শুধু exe-ও ওই পেজ থেকে নামানো যায়
  (`chitchatui.exe`, `chitchatbot.exe`, `crushfling.exe`)

প্রতিবার Windows build সফল হলে GitHub Actions এই release-টি নিজে আপডেট করে।

## দ্রুত শুরু (Windows)

**উপায় ১ — রেডি zip:** zip পুরোটা extract করুন → `run.bat` ডাবল-ক্লিক।
`build\bin\chitchatui.exe` আগেই বানানো আছে, তাই সাথে সাথে ড্যাশবোর্ড খুলবে।

**উপায় ২ — নিজে build:** `install.bat` ডাবল-ক্লিক। এটি দরকার হলে Go 1.25+,
Wails CLI v2.12.0 ও WebView2 runtime ইনস্টল করে, তারপর ড্যাশবোর্ড + CLI
তৈরি করে এবং চালু করার প্রস্তাব দেয় (প্রথমবার ৫–১৫ মিনিট, ইন্টারনেট লাগবে)।

```bat
run.bat                  :: ড্যাশবোর্ড চালু
run.bat cli --check      :: CrushFling CLI (ডেটা ফোল্ডার ও সেটিংস যাচাই)
run.bat help             :: সাহায্য
install.bat              :: ইনস্টল + build + চালু
install.bat build        :: শুধু build
install.bat run          :: শুধু ড্যাশবোর্ড চালু
install.bat bundle       :: WebView2 + Chromium সহ portable bundle
```

লগ: `build\install.log`।

## ড্যাশবোর্ড

উপরে সব সময় **Start / Stop**। **Stop** রান বাতিল করে এবং worker-এর খোলা
**সব Chromium বন্ধ** করে দেয়। ছয়টি ট্যাব:

| ট্যাব | কী আছে |
|---|---|
| Pictures | ছবির ফোল্ডার Browse/Open, কত ছবি বাকি |
| Proxy | proxy ফাইল/লাইন, offline **Validate** |
| Bio list | `bio.txt` লোড, বাকি লাইন, পরের লাইনের প্রিভিউ |
| Browser | headless / visible, browser fallback, verify, concurrency, **Close all browsers** |
| Settings | account সংখ্যা, data folder, mail provider/hook, poll/timeout, retries, delay, captcha, debug/trace |
| Logs | লাইভ লগ, লেভেল ফিল্টার, log folder, debug report |

প্রতি account-এ: random নাম, বয়স **১৯–২৪**, দেশ **USA + ৩০টি ইউরোপীয় দেশ**
(মোট ৩১), লিঙ্গ female → looking for male, আর `bio.txt` থেকে random
**অব্যবহৃত** লাইন। লাইনটি কেবল profile verified হওয়ার পরেই ফাইল থেকে মুছে যায়।

## CLI

```bat
build\bin\crushfling.exe --probe
build\bin\crushfling.exe --count 1 --email mailtm -v
build\bin\crushfling.exe --count 10 --concurrency 2 --proxy proxies.txt
build\bin\crushfling.exe --bio my_bios.txt --preview
```

## ডেটা ও গোপনীয়তা

- `data\` — অ্যাপের settings, অ্যাকাউন্ট, session (কখনোই git-এ যাবে না)
- `crushfling\data\` (অথবা ড্যাশবোর্ডের data folder) — `bio.txt`, `pictures\`,
  `accounts.json`, `accounts.csv`, `logs\`
- কোনো অ্যাকাউন্ট, কুকি, টোকেন বা লগ রিপোজিটরিতে commit করা হয় না
  (`.gitignore` ব্লক করে)।

## ডকুমেন্টেশন

- [docs/START-HERE.md](docs/START-HERE.md) — ধাপে ধাপে সেটআপ ও ম্যানুয়াল টেস্ট
- [crushfling/README.md](crushfling/README.md) — বাংলা quick guide
- [docs/CRUSHFLING_AUTO_REGISTER.md](docs/CRUSHFLING_AUTO_REGISTER.md) — কীভাবে কাজ করে
- [docs/CRUSHFLING_SIGNUP_PLAN.md](docs/CRUSHFLING_SIGNUP_PLAN.md) — পরিকল্পনা ও যাচাইয়ের প্রমাণ
- [docs/PROJECT_MEMORY.md](docs/PROJECT_MEMORY.md) — সিদ্ধান্ত ও বর্তমান অবস্থা
