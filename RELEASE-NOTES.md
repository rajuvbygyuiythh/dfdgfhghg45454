# Release notes — 2026-10-20

## এই বিল্ডে কী আছে

- ড্যাশবোর্ড: `build\bin\chitchatui.exe` (Windows x64, WebView2, production tags)
- Bot CLI: `build\bin\chitchatbot.exe`
- CrushFling CLI: `build\bin\crushfling.exe`
- এক ক্লিকে চালু: `run.bat` · নিজে build করতে: `install.bat`

## যা পরীক্ষা করা হয়েছে (PASS)

| পরীক্ষা | ফল |
|---|---|
| `npm test` (settings, UI, contracts, package, overlay, launchers) | **83/83** |
| `go test -race ./internal/crushfling/...` | **52 tests, ok** |
| `go vet ./...` (Windows target, পুরো repo) | **PASS** |
| `GOOS=windows GOARCH=amd64 go build ./...` (পুরো repo, `app.go` সহ) | **PASS** |
| Linuxে mock Laravel সাইটের বিরুদ্ধে সম্পূর্ণ রেজিস্ট্রেশন E2E | **PASS** (bio লাইন reserved → profile verified → লাইন deleted) |
| ড্যাশবোর্ড UI চুক্তি (৬ ট্যাব, Start/Stop, Close all browsers, bio/proxy/picture bindings) | **PASS** |
| Release zip: SHA-256 manifest, source↔zip byte equality, PE header | **PASS** |
| `run.bat` / `install.bat`: CRLF, ASCII, সব `goto`/`call` লেবেল, build কমান্ড | **PASS** |

## যা এখনো চালানো হয়নি (NOT_RUN)

- Windows-এ exe চালিয়ে দেখা — বিল্ড environment Linux, Windows/Wine নেই।
  (তাই zip-এ prebuilt exe দেওয়া আছে; না চললে `install.bat` দিয়ে নিজের
  মেশিনে rebuild করুন।)
- আসল `crushfling.com`-এ লাইভ রেজিস্ট্রেশন — sandbox TLS ব্লক করে।
- `wails build` CLI নিজে — একই `desktop,production` tag দিয়ে `go build` করা
  হয়েছে, যা Wails CLI যেভাবে build করে সেই একই পথ।
- Windows ইনস্টলার পথ (WebView2 ইনস্টল, UAC) — বাস্তব Windows মেশিনে যাচাই বাকি।

## Prebuilt exe সম্পর্কে সৎ নোট

- exe তিনটি Linux sandbox-এ cross-compile করা; Windows-এ চালিয়ে দেখা হয়নি।
- sandbox থেকে sourcehut (`git.sr.ht`) পৌঁছায় না, তাই Wails-এর Windows
  notification নির্ভরতা `go-toast` একটি compile-only stub দিয়ে বানানো
  হয়েছে (notification no-op)। এই ড্যাশবোর্ড notification ব্যবহার করে না,
  তাই কার্যকারিতায় পার্থক্য নেই। সম্পূর্ণ আসল dependency সহ native build
  পেতে `install.bat` চালান।

## নিরাপত্তা

- `data\`, `crushfling\data\`, cookies, tokens, logs কোথাও নেই।
- build/release artefact git-ignored; zip-এ শুধু source + ৩টি exe + manifest।
- `.gitignore` runtime data ব্লক করে।

বিস্তারিত: `README.md`, `START-HERE.txt`, `docs\PROJECT_MEMORY.md`,
`docs\CRUSHFLING_AUTO_REGISTER.md`।
