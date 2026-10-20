# CrushFling Auto Register — standalone (Windows)

শুধু **CrushFling** — ChitChat Bot ছাড়া আলাদা ছোট অ্যাপ। একই ইঞ্জিন
(`internal/crushfling`), একই ছয়-ট্যাব ড্যাশবোর্ড, কিন্তু অ্যাপটি স্বাধীন:
`CrushFling.exe` + `data\` ফোল্ডার।

## চালাতে

1. এই ফোল্ডারটা যেকোনো জায়গায় রাখুন (Desktop / `D:\CrushFling`)।
2. **`run.bat`** ডাবল-ক্লিক → ড্যাশবোর্ড খুলবে।
3. `CrushFling.exe` আগেই বানানো থাকে; না থাকলে `install.bat` নিজে build করবে
   (Go 1.25+ এবং ইন্টারনেট লাগবে, প্রথমবার ৩–১০ মিনিট)।

## প্রথম সেটআপ (ড্যাশবোর্ডে)

| ট্যাব | কী করবেন |
|---|---|
| 1 · Pictures | **Browse…** দিয়ে ছবির ফোল্ডার বেছে নিন (jpg/png/webp) |
| 2 · Proxy | দরকার হলে proxy ফাইল/লাইন দিন → **Validate** |
| 3 · Bio list | **Load bio.txt…** — প্রতি লাইনে একটি বায়ো |
| 4 · Browser | headless / visible, browser verify, concurrency |
| 5 · Settings | account সংখ্যা, mail provider/hook, poll/timeout, retries, delay |
| 6 · Logs | লাইভ লগ, log folder, debug report |

উপরে সব সময় **Start / Stop**। **Stop** রান বাতিল করে এবং worker-এর খোলা
**সব Chromium বন্ধ** করে।

## প্রতিটি account-এ যা হয়

- random নাম + **বয়স ১৯–২৪**
- দেশ: **USA + ৩০টি ইউরোপীয় দেশ** (মোট ৩১), প্রতিটির নিজের শহর
- লিঙ্গ female → looking for male
- `bio.txt` থেকে random **অব্যবহৃত** লাইন; profile verified হলে লাইনটি
  ফাইল থেকে স্বয়ংক্রিয়ভাবে মুছে যায়
- ছবি `pictures\` থেকে, ব্যবহারের পর `pictures\used\`-এ চলে যায়

## ফাইল কোথায়

```
CrushFling.exe        ড্যাশবোর্ড
crushfling-cli.exe    কমান্ড-লাইন worker
run.bat / install.bat চালু ও build
data\
  crushfling\
    bio.txt           বায়ো লাইন (ব্যবহৃত লাইন auto-delete)
    pictures\         ছবি (+ used\)
    accounts.json     সেভ হওয়া অ্যাকাউন্ট
    accounts.csv      একই ডেটা CSV-তে
    api.json          সাইট endpoint ক্যালিব্রেশন
    logs\             প্রতি রানের লগ
```

`data\` ফোল্ডারে অ্যাকাউন্ট, কুকি, টোকেন ও লগ থাকে — কারও সাথে শেয়ার করবেন না।

## CLI

```bat
crushfling-cli.exe --check
crushfling-cli.exe --count 1 --email mailtm -v
crushfling-cli.exe --count 10 --concurrency 2 --proxy proxies.txt
crushfling-cli.exe --bio my_bios.txt --preview
```

## নিজে build করতে

```bat
install.bat
```

অথবা হাতে:

```bat
go mod tidy
go build -tags desktop,production -ldflags "-H windowsgui" -o CrushFling.exe .
go build -o crushfling-cli.exe .\cmd\crushfling
```

Windows 10/11-এ WebView2 runtime দরকার (Windows 11 ও আপডেটেড Windows 10-এ
আগে থেকেই থাকে)। না থাকলে: https://developer.microsoft.com/microsoft-edge/webview2/

## সীমা (সৎভাবে)

- এই অ্যাপটি `wails build`/`go build` দিয়ে **Windows-এ** কম্পাইল করা;
  খুলে হাতে-কলমে টেস্ট করা হয়নি।
- আসল crushfling.com-এ লাইভ রেজিস্ট্রেশন এখনো চালানো হয়নি।
