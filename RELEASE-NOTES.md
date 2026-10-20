# Release notes — 2026-10-20

**ডাউনলোড:** https://github.com/rajuvbygyuiythh/dfdgfhghg45454/releases/tag/windows-x64


## এই বিল্ডে কী আছে

- ড্যাশবোর্ড: `build\bin\chitchatui.exe` (Windows x64, Wails production build, GUI)
- Bot CLI: `build\bin\chitchatbot.exe` (console)
- CrushFling CLI: `build\bin\crushfling.exe` (console)
- এক ক্লিকে চালু: `run.bat` · নিজে build করতে: `install.bat`

এই তিনটি exe **আসল Windows মেশিনে** (GitHub Actions `windows-latest`) Go 1.25,
Wails CLI v2.12.0 এবং `wails build -clean -webview2 error` দিয়ে বানানো —
cross-compile নয়। workflow: `.github/workflows/windows-build.yml`।

## যা পরীক্ষা করা হয়েছে (PASS)

| পরীক্ষা | ফল |
|---|---|
| Windows runner-এ `go vet ./...` | **PASS** |
| Windows runner-এ `go test ./internal/crushfling/...` | **PASS** |
| Windows runner-এ `wails build -clean -webview2 error` | **PASS** |
| Windows runner-এ দুই CLI build | **PASS** |
| PE header: PE32+ x86-64, dashboard GUI (subsystem 2), CLI console (3) | **PASS** |
| `npm test` (Linux sandbox: settings, UI, contracts, package, overlay, launchers) | **83/83** |
| `go test -race ./internal/crushfling/...` (Linux sandbox) | **52 tests, ok** |
| `GOOS=windows go build ./...` + `go vet ./...` (Linux sandbox) | **PASS** |
| mock Laravel সাইটে সম্পূর্ণ রেজিস্ট্রেশন E2E | **PASS** |
| Release zip: SHA-256 manifest, source↔zip equality | **PASS** |
| `run.bat` / `install.bat`: CRLF, ASCII, সব লেবেল, build কমান্ড | **PASS** |

CI run: GitHub Actions → **Windows build** → commit `dd2b578`-এর ধারাবাহিক রান
(run id `39367503503`)।

## যা এখনো চালানো হয়নি (NOT_RUN)

- ডেস্কটপে ড্যাশবোর্ড 창 খুলে হাতে-কলমে চালানো — CI শুধু build ও PE যাচাই করে;
  WebView2 runtime থাকা Windows 10/11-এ এটি চলবে।
- আসল `crushfling.com`-এ লাইভ রেজিস্ট্রেশন — sandbox TLS ব্লক করে।
- `install.bat`-এর WebView2/UAC ইনস্টল পথ — বাস্তব Windows মেশিনে যাচাই বাকি।

## নিরাপত্তা

- `data\`, `crushfling\data\`, cookies, tokens, logs কোথাও নেই।
- build/release artefact git-ignored; zip-এ শুধু source + ৩টি exe + manifest।
- `.gitignore` runtime data ব্লক করে।

বিস্তারিত: `README.md`, `START-HERE.txt`, `docs\PROJECT_MEMORY.md`,
`docs\CRUSHFLING_AUTO_REGISTER.md`।
